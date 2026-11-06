"""Recolector de evidencia AWS de sólo lectura para I-000 (revisión 2).

Ejecuta las consultas no facturables del catálogo y construye las evidencias que
consume ``viabilidad.py``:

- ``cuenta_<fecha>.json``: plan, vigencia, créditos y estado fiscal;
- ``servicios_<fecha>.json``: Redis OSS, VPC Origins, cuota de vCPU, zonas con
  ``t4g.micro`` y límites de DynamoDB;
- ``free_tier_<fecha>.json``: uso del plan Free.

Correcciones de esta revisión:

1. La lectura de respuestas usa el directorio de **evidencia sanitizada**
   (``evidencia/<id>.json``) y, si no existe, el volcado crudo
   (``<crudo>/<id>.raw.txt``). Antes se buscaba ``<id>.json`` dentro del
   directorio crudo, por lo que todos los campos llegaban vacíos.
2. Los nombres de campo son los que devuelve AWS de verdad:
   ``accountPlanType``, ``accountPlanStatus``, ``accountPlanExpirationDate``,
   ``accountPlanRemainingCredits.amount``, ``VpcOriginList.Items``,
   ``freeTierUsages``.
3. ``list-vpc-origins`` enumera orígenes existentes; **no** acredita que la
   cuenta pueda crear VPC Origins, así que la elegibilidad queda ``None``.

Reglas:

- Nunca ejecuta Cost Explorer ni usa ``--aprobar-facturable``.
- Sanitiza toda salida antes de escribir.
- No convierte un listado en prueba de disponibilidad.
- No inventa créditos: se informan los que AWS expone y se marca su origen.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import tempfile
from pathlib import Path

import consultas_readonly as cr
import sanitizar

RAIZ = Path(__file__).resolve().parents[2]
DIR_EVIDENCIA = Path(__file__).with_name("evidencia")
FECHA = "2026-11-06"
CREDITOS_DECLARADOS = "120"


def _leer(identificador: str, dir_evidencia: Path, dir_crudo: Path) -> dict | None:
    """Lee la evidencia sanitizada y, si falta, el volcado crudo."""

    saneada = Path(dir_evidencia) / f"{identificador}.json"
    if saneada.exists():
        texto = saneada.read_text(encoding="utf-8").strip()
        if texto:
            try:
                return json.loads(texto)
            except json.JSONDecodeError:
                pass
    crudo = Path(dir_crudo) / f"{identificador}.raw.txt"
    if crudo.exists():
        texto = crudo.read_text(encoding="utf-8").strip()
        if texto:
            try:
                return json.loads(texto)
            except json.JSONDecodeError:
                return None
    return None


def _ejecutar_todas(profile: str, dir_crudo: Path, ids: tuple[str, ...]) -> dict[str, dict]:
    catalogo = cr.catalogo_por_id()
    resultados: dict[str, dict] = {}
    for identificador in ids:
        consulta = catalogo[identificador]
        resultados[identificador] = cr.ejecutar(
            consulta,
            profile,
            cr.REGION_COMERCIAL_DATOS,
            dir_crudo,
            DIR_EVIDENCIA,
            account_id=None,
            aprobar_facturable=False,
        )
    return resultados


def _version_tupla(texto: str) -> tuple[int, ...]:
    partes: list[int] = []
    for trozo in str(texto).replace("-", ".").split("."):
        try:
            partes.append(int(trozo))
        except ValueError:
            partes.append(0)
    return tuple(partes)


def construir_cuenta(
    dir_crudo: Path,
    resultados: dict[str, dict],
    dir_evidencia: Path = DIR_EVIDENCIA,
    hoy: str = FECHA,
) -> dict:
    identidad = _leer("sts_identidad", dir_evidencia, dir_crudo)
    plan = _leer("plan_free", dir_evidencia, dir_crudo)
    sts_ok = resultados.get("sts_identidad", {}).get("estado") == "PASS"
    plan_ok = resultados.get("plan_free", {}).get("estado") == "PASS"

    creditos = None
    if isinstance(plan, dict):
        resto = plan.get("accountPlanRemainingCredits") or {}
        if isinstance(resto, dict):
            creditos = resto.get("amount")
            unidad = resto.get("unit")
        else:
            unidad = None
    else:
        unidad = None

    estado = "VERIFIED_AWS" if (sts_ok and plan_ok and plan) else "NOT_VERIFIED"
    motivo = None
    if not sts_ok:
        motivo = "sin identidad AWS valida"
    elif not plan_ok:
        motivo = "sin respuesta de plan Free Tier"

    datos = {
        "plan": (plan or {}).get("accountPlanType"),
        "estado_plan": (plan or {}).get("accountPlanStatus"),
        "fecha_alta": None,
        "fecha_expiracion": (plan or {}).get("accountPlanExpirationDate"),
        "cuenta_enmascarada": "<ACCOUNT_ID>" if identidad else None,
        "creditos_declarados_usd": CREDITOS_DECLARADOS,
        "creditos_verificados_usd": "0" if creditos is None else str(creditos),
        "creditos_unidad": unidad,
        "fuente_creditos": (
            "aws freetier get-account-plan-state (accountPlanRemainingCredits)"
            if creditos is not None
            else "no expuesto por la API consultada"
        ),
        "impuestos": "UNKNOWN",
    }
    return {
        "id": "cuenta",
        "fecha": hoy,
        "estado": estado,
        "fuente": "aws sts get-caller-identity; aws freetier get-account-plan-state",
        "consulta_ejecutada": True,
        "motivo": motivo,
        "datos": datos,
        "sanitizado": True,
        "nota": (
            "creditos_declarados_usd es el dato historico del operador; "
            "creditos_verificados_usd procede de accountPlanRemainingCredits cuando la "
            "respuesta de plan fue valida. El saldo liquidado sigue en Billing"
        ),
    }


def construir_servicios(
    dir_crudo: Path,
    resultados: dict[str, dict],
    dir_evidencia: Path = DIR_EVIDENCIA,
    hoy: str = FECHA,
) -> dict:
    redis = _leer("redis_versiones", dir_evidencia, dir_crudo)
    zonas_ec2 = _leer("tipos_instancia", dir_evidencia, dir_crudo)
    quotas = _leer("cuota_ec2_vcpu", dir_evidencia, dir_crudo)
    origins = _leer("cloudfront_vpc_origins", dir_evidencia, dir_crudo)
    ddb = _leer("ddb_limites", dir_evidencia, dir_crudo)

    versiones = (redis or {}).get("CacheEngineVersions") or []
    elegibles = [
        v
        for v in versiones
        if str(v.get("Engine", "")).lower() == "redis"
        and str(v.get("EngineVersion"))
    ]
    elegibles.sort(key=lambda v: _version_tupla(v.get("EngineVersion", "")), reverse=True)
    sku_redis = None
    if elegibles:
        ultima = elegibles[0]
        sku_redis = (
            f"cache.t4g.micro redis {ultima.get('EngineVersion')} "
            f"({ultima.get('CacheParameterGroupFamily')})"
        )

    zonas: list[str] = []
    for oferta in (zonas_ec2 or {}).get("InstanceTypeOfferings") or []:
        if oferta.get("InstanceType") == "t4g.micro" and oferta.get("Location"):
            zona = str(oferta["Location"])
            if zona not in zonas:
                zonas.append(zona)
    zonas.sort()

    cuota = None
    quota = (quotas or {}).get("Quota") or {}
    if quota.get("Value") is not None:
        cuota = int(float(quota["Value"]))

    origenes = ((origins or {}).get("VpcOriginList") or {}).get("Items") or []

    limites_ddb = None
    if ddb:
        limites_ddb = {
            "lecturas_max_cuenta": ddb.get("AccountMaxReadCapacityUnits"),
            "escrituras_max_cuenta": ddb.get("AccountMaxWriteCapacityUnits"),
            "lecturas_max_tabla": ddb.get("TableMaxReadCapacityUnits"),
            "escrituras_max_tabla": ddb.get("TableMaxWriteCapacityUnits"),
        }

    estados = {
        "redis": resultados.get("redis_versiones", {}).get("estado"),
        "zonas": resultados.get("tipos_instancia", {}).get("estado"),
        "cuota": resultados.get("cuota_ec2_vcpu", {}).get("estado"),
        "origins": resultados.get("cloudfront_vpc_origins", {}).get("estado"),
        "ddb": resultados.get("ddb_limites", {}).get("estado"),
    }
    verificados = bool(sku_redis) and len(zonas) >= 2 and cuota is not None
    return {
        "id": "servicios",
        "fecha": hoy,
        "estado": "VERIFIED_AWS" if verificados else "NOT_VERIFIED",
        "fuente": (
            "aws elasticache describe-cache-engine-versions; aws ec2 "
            "describe-instance-type-offerings; aws service-quotas get-service-quota; "
            "aws cloudfront list-vpc-origins; aws dynamodb describe-limits"
        ),
        "datos": {
            "redis_oss_sku": sku_redis,
            "redis_versiones_observadas": len(versiones),
            "cloudfront_vpc_origin_habilitado": None,
            "cloudfront_vpc_origin_nota": (
                "list-vpc-origins enumera origenes existentes; no demuestra que la cuenta "
                "pueda crear VPC Origins, por lo que NO se marca como habilitado"
            ),
            "vpc_origins_observados": len(origenes),
            "cuota_vcpu_on_demand": cuota,
            "az_con_t4g_micro": zonas,
            "dynamodb_limites": limites_ddb,
        },
        "estados_consulta": estados,
        "sanitizado": True,
    }


def construir_free_tier(
    dir_crudo: Path,
    resultados: dict[str, dict],
    dir_evidencia: Path = DIR_EVIDENCIA,
    hoy: str = FECHA,
) -> dict:
    datos = _leer("free_tier_uso", dir_evidencia, dir_crudo)
    estado_consulta = resultados.get("free_tier_uso", {}).get("estado")
    usos = (datos or {}).get("freeTierUsages") or []
    return {
        "id": "free_tier",
        "fecha": hoy,
        "estado": "VERIFIED_AWS" if estado_consulta == "PASS" else "NOT_RUN",
        "fuente": "aws freetier get-free-tier-usage",
        "datos": {
            "servicios_consultados": len(usos),
            "servicios": sorted(
                {str(u.get("serviceCode")) for u in usos if u.get("serviceCode")}
            )[:20],
            "franquicias_consumidas": None,
            "cobertura_60_dias": None,
        },
        "motivo": None if estado_consulta == "PASS" else "consulta no ejecutada o fallida",
        "nota": (
            "La lista vacia indica que la cuenta no reporta consumo de Free Tier en este "
            "momento; no prueba que no existan cargos"
        ),
        "sanitizado": True,
    }


def recolectar(
    profile: str,
    dir_crudo: Path | None = None,
    aplicar: bool = True,
    dir_evidencia: Path = DIR_EVIDENCIA,
) -> dict:
    dir_crudo = Path(dir_crudo) if dir_crudo else directorio_crudo_temporal()
    ids = (
        "sts_identidad",
        "plan_free",
        "free_tier_uso",
        "cuota_ec2_vcpu",
        "tipos_instancia",
        "redis_versiones",
        "cloudfront_vpc_origins",
        "ddb_limites",
    )
    resultados = _ejecutar_todas(profile, dir_crudo, ids)
    cuenta = construir_cuenta(dir_crudo, resultados, dir_evidencia)
    servicios = construir_servicios(dir_crudo, resultados, dir_evidencia)
    free_tier = construir_free_tier(dir_crudo, resultados, dir_evidencia)

    if aplicar:
        Path(dir_evidencia).mkdir(parents=True, exist_ok=True)
        for nombre, contenido in (
            (f"cuenta_{FECHA}.json", cuenta),
            (f"servicios_{FECHA}.json", servicios),
            (f"free_tier_{FECHA}.json", free_tier),
        ):
            (Path(dir_evidencia) / nombre).write_text(
                json.dumps(sanitizar.sanitizar(contenido), indent=2, ensure_ascii=False) + "\n",
                encoding="utf-8",
            )

    return {
        "consultas": {k: v.get("estado") for k, v in resultados.items()},
        "cuenta": cuenta,
        "servicios": servicios,
        "free_tier": free_tier,
    }


def directorio_crudo_temporal() -> Path:
    """Ruta temporal nativa; nunca la cadena literal ``$env:TEMP`` ni el repo."""

    base = os.environ.get("TEMP") or os.environ.get("TMP") or tempfile.gettempdir()
    candidato = Path(base) / "nexum_finops_crudo"
    try:
        candidato.mkdir(parents=True, exist_ok=True)
        return candidato
    except OSError:
        return Path(tempfile.gettempdir()) / "nexum_finops_crudo"


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Evidencia AWS read-only I-000")
    parser.add_argument("--profile", default=os.environ.get("AWS_PROFILE", "nexum"))
    parser.add_argument("--dir-crudo", type=Path, default=None)
    parser.add_argument("--no-aplicar", action="store_true")
    args = parser.parse_args(argv)

    resumen = recolectar(args.profile, args.dir_crudo, aplicar=not args.no_aplicar)
    print(json.dumps(sanitizar.sanitizar(resumen), indent=2, ensure_ascii=False, sort_keys=True))
    return 0


if __name__ == "__main__":
    sys.exit(main())
