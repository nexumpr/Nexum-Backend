"""Evaluación de viabilidad FinOps de I-000 (INF-000-05).

Regla central: sólo se declara ``VIABLE`` cuando la evidencia AWS y todos los
precios requeridos están verificados. Cualquier vacío produce
``BLOCKED_FINOPS`` con el motivo y la evidencia que falta. Nunca se eleva un
estado por disponibilidad de créditos ni por ausencia de errores.
"""

from __future__ import annotations

import argparse
import json
import sys
from datetime import date
from pathlib import Path

import dinero
import modelo_costos
import requisitos

DIR_EVIDENCIA = Path(__file__).with_name("evidencia")
DIR_RESULTADOS = Path(__file__).with_name("resultados")
ESTADO_VERIFICADO = "VERIFIED_AWS"


def _cargar_evidencia(dir_evidencia: Path, identificador: str) -> dict | None:
    archivos = sorted(dir_evidencia.glob(f"{identificador}_*.json"))
    if not archivos:
        return None
    return json.loads(archivos[-1].read_text(encoding="utf-8"))


def _motivo(codigo: str, detalle: str, evidencia_requerida: str) -> dict:
    return {"codigo": codigo, "detalle": detalle, "evidencia_requerida": evidencia_requerida}


def _evaluar_cuenta(cuenta: dict | None, hoy: date) -> tuple[list[dict], list[str]]:
    motivos: list[dict] = []
    condiciones: list[str] = []
    if cuenta is None:
        return [
            _motivo(
                "EVIDENCIA_CUENTA_AUSENTE",
                "no existe evidencia de cuenta AWS",
                "salida sanitizada de aws freetier get-account-plan-state",
            )
        ], condiciones

    if cuenta.get("estado") != ESTADO_VERIFICADO:
        motivos.append(
            _motivo(
                "EVIDENCIA_CUENTA_NO_VERIFICADA",
                f"estado de evidencia de cuenta: {cuenta.get('estado')} ({cuenta.get('motivo', 'sin motivo')})",
                "reejecutar consultas read-only con sesion AWS valida",
            )
        )

    datos = cuenta.get("datos") or {}
    plan = datos.get("plan")
    if plan is None:
        motivos.append(
            _motivo(
                "PLAN_NO_DETERMINADO",
                "tipo de plan (FREE/PAID) desconocido",
                "aws freetier get-account-plan-state",
            )
        )
    elif str(plan).upper() != "FREE":
        motivos.append(
            _motivo("PLAN_NO_FREE", f"plan observado {plan}; I-000 exige plan Free", "confirmar plan Free activo")
        )

    expiracion = datos.get("fecha_expiracion")
    if expiracion is None:
        motivos.append(
            _motivo(
                "VIGENCIA_NO_DETERMINADA",
                "fecha de expiracion del plan desconocida",
                "aws freetier get-account-plan-state",
            )
        )
    else:
        try:
            fecha_exp = date.fromisoformat(str(expiracion)[:10])
        except ValueError:
            fecha_exp = None
        if fecha_exp is None:
            motivos.append(
                _motivo("VIGENCIA_ILEGIBLE", f"fecha de expiracion no parseable: {expiracion}", "formato ISO")
            )
        elif fecha_exp <= hoy:
            motivos.append(
                _motivo("VIGENCIA_VENCIDA", f"plan vence {fecha_exp.isoformat()}", "plan Free vigente")
            )
        else:
            condiciones.append(
                f"Vigencia declarada hasta {fecha_exp.isoformat()}; no garantiza 45 dias si los creditos se agotan"
            )

    creditos = datos.get("creditos_verificados_usd")
    if creditos is None or dinero.dec(creditos) <= 0:
        motivos.append(
            _motivo(
                "CREDITOS_NO_VERIFICADOS",
                f"creditos verificados: {creditos}; los creditos declarados no son evidencia",
                "saldo real de creditos y fecha de expiracion",
            )
        )
    return motivos, condiciones


def _evaluar_servicios(servicios: dict | None) -> tuple[list[dict], list[str]]:
    motivos: list[dict] = []
    condiciones: list[str] = []
    if servicios is None:
        return [
            _motivo(
                "EVIDENCIA_SERVICIOS_AUSENTE",
                "no existe evidencia de servicios, cuotas ni SKU",
                "consultas describe/list read-only de ElastiCache, CloudFront, Service Quotas y EC2",
            )
        ], condiciones

    if servicios.get("estado") != ESTADO_VERIFICADO:
        motivos.append(
            _motivo(
                "EVIDENCIA_SERVICIOS_NO_VERIFICADA",
                f"estado de evidencia de servicios: {servicios.get('estado')}",
                "reejecutar consultas read-only con sesion AWS valida",
            )
        )
    datos = servicios.get("datos") or {}
    if not datos.get("redis_oss_sku"):
        motivos.append(
            _motivo(
                "REDIS_OSS_NO_CONFIRMADO",
                "SKU/version de ElastiCache Redis OSS no confirmado en la cuenta",
                "aws elasticache describe-cache-engine-versions y Price List API",
            )
        )
    if datos.get("cloudfront_vpc_origin_habilitado") is not True:
        motivos.append(
            _motivo(
                "VPC_ORIGIN_NO_CONFIRMADO",
                "disponibilidad de CloudFront VPC Origins no confirmada",
                "aws cloudfront list-vpc-origins y prueba en cuenta",
            )
        )
    cuota = datos.get("cuota_vcpu_on_demand")
    if cuota is None or int(cuota) < 2:
        motivos.append(
            _motivo(
                "CUOTA_EC2_INSUFICIENTE",
                f"cuota de vCPU On-Demand observada: {cuota}",
                "aws servicequotas get-service-quota (L-1216C47A)",
            )
        )
    if datos.get("t4g_micro_disponible_az") is not True:
        motivos.append(
            _motivo(
                "TIPO_INSTANCIA_NO_CONFIRMADO",
                "disponibilidad de t4g.micro por zona no confirmada",
                "aws ec2 describe-instance-type-offerings",
            )
        )
    if datos.get("dynamodb_limites") is None:
        condiciones.append("Limites de DynamoDB no verificados; no impiden el diseño actual")
    return motivos, condiciones


def _evaluar_costos(costos: dict | None) -> tuple[list[dict], list[str]]:
    motivos: list[dict] = []
    condiciones: list[str] = []
    if costos is None:
        return [
            _motivo(
                "EVIDENCIA_COSTOS_AUSENTE",
                "no existe evidencia de costo observado",
                "revision de facturacion (consola) o Cost Explorer con aprobacion especifica",
            )
        ], condiciones
    if costos.get("estado") != ESTADO_VERIFICADO:
        motivos.append(
            _motivo(
                "COSTO_OBSERVADO_NO_VERIFICADO",
                f"costo observado no verificado: {costos.get('motivo', costos.get('estado'))}",
                "consola de facturacion (accion humana) o autorizacion especifica de Cost Explorer",
            )
        )
    condiciones.append(
        "Cost Explorer tiene costo por solicitud (USD 0.01); su uso requiere aprobacion humana concreta"
    )
    return motivos, condiciones


def _evaluar_precios(resumen_costos: dict) -> tuple[list[dict], list[str]]:
    motivos: list[dict] = []
    condiciones: list[str] = []
    escenario = resumen_costos["escenarios"]["C_CLOUDFRONT_VPC_ORIGIN_PRIVADO"]
    no_verificadas = [
        p["clave"]
        for p in escenario["partidas"]
        if p["requerida"] and dinero.dec(p["cantidad"]) > 0 and p["estado_evidencia"] != ESTADO_VERIFICADO
    ]
    if no_verificadas:
        motivos.append(
            _motivo(
                "PRECIOS_NO_VERIFICADOS",
                f"{len(no_verificadas)} partidas requeridas sin precio verificado en AWS: {', '.join(no_verificadas)}",
                "aws pricing get-products por SKU en us-east-2 y actualizacion del catalogo",
            )
        )
    if escenario["total_bruto_45d_usd"] is None:
        motivos.append(
            _motivo(
                "COSTO_BRUTO_DESCONOCIDO",
                "el total bruto a 45 dias no puede calcularse sin precios verificados",
                "catalogo de precios completo para todas las partidas requeridas",
            )
        )
    supuestos = [p["clave"] for p in escenario["partidas"] if p["estado_evidencia"] == "SUPUESTO"]
    if supuestos:
        condiciones.append(
            f"Partidas de reserva/supuesto no tarifarias incluidas: {', '.join(supuestos)}"
        )
    return motivos, condiciones


def _evaluar_financiero(resumen_costos: dict, creditos_verificados: str) -> tuple[list[dict], list[str]]:
    motivos: list[dict] = []
    condiciones: list[str] = []
    bruto = resumen_costos["escenarios"]["C_CLOUDFRONT_VPC_ORIGIN_PRIVADO"]["total_bruto_45d_usd"]
    if bruto is None:
        return motivos, condiciones
    if dinero.dec(bruto) > dinero.dec(creditos_verificados):
        motivos.append(
            _motivo(
                "CREDITOS_INSUFICIENTES",
                f"bruto {bruto} USD supera creditos verificados {creditos_verificados} USD",
                "verificar saldo real de creditos y reoptimizar topologia",
            )
        )
    fin = resumen_costos["financiero"]
    if fin["impuestos"] == "UNKNOWN":
        condiciones.append("IVA/impuestos no determinados: no se imputa 0% ni un monto sin evidencia")
    condiciones.append(
        "Cero desembolso personal: el modelo no contempla pago con fondos propios ni cambio a plan Paid"
    )
    return motivos, condiciones


def evaluar(
    dir_evidencia: Path = DIR_EVIDENCIA,
    ruta_precios: Path | None = None,
    ruta_topologia: Path | None = None,
    hoy: date | None = None,
) -> dict:
    """Produce la decisión de viabilidad con motivos y condiciones."""

    hoy = hoy or date.today()
    cuenta = _cargar_evidencia(dir_evidencia, "cuenta")
    servicios = _cargar_evidencia(dir_evidencia, "servicios")
    costos = _cargar_evidencia(dir_evidencia, "costos_observados")
    free_tier = _cargar_evidencia(dir_evidencia, "free_tier")

    catalogo = modelo_costos.cargar_precios(ruta_precios)
    creditos_verificados = "0"
    if cuenta and (cuenta.get("datos") or {}).get("creditos_verificados_usd") is not None:
        creditos_verificados = str(cuenta["datos"]["creditos_verificados_usd"])
        estado_cuenta = cuenta.get("estado")
        if estado_cuenta != ESTADO_VERIFICADO:
            creditos_verificados = "0"

    resumen_costos = modelo_costos.calcular_todo(catalogo, creditos_verificados)

    motivos: list[dict] = []
    condiciones: list[str] = []

    m, c = _evaluar_cuenta(cuenta, hoy)
    motivos += m
    condiciones += c
    m, c = _evaluar_servicios(servicios)
    motivos += m
    condiciones += c
    m, c = _evaluar_costos(costos)
    motivos += m
    condiciones += c
    m, c = _evaluar_precios(resumen_costos)
    motivos += m
    condiciones += c
    m, c = _evaluar_financiero(resumen_costos, creditos_verificados)
    motivos += m
    condiciones += c

    if free_tier is None or free_tier.get("estado") != ESTADO_VERIFICADO:
        condiciones.append("Uso del plan Free Tier no verificado (aws freetier get-free-tier-usage)")

    ruta_topo = ruta_topologia or (Path(__file__).with_name("evidencia") / "topologia_c_2026-11-06.json")
    topologia = json.loads(Path(ruta_topo).read_text(encoding="utf-8"))
    revision = requisitos.validar(topologia)
    if not revision["diseno_valido"]:
        motivos.append(
            _motivo(
                "TOPOLOGIA_INCUMPLE",
                "; ".join(revision["dos_instancias"] + revision["conectividad_privada"]),
                "corregir el diseno de dos instancias y conectividad privada",
            )
        )
    condiciones.extend(revision["tls"])
    condiciones.append("Instancias y balanceo reales: verificacion runtime NOT_RUN hasta H08-I/H08-E autorizado")

    estado = "VIABLE" if not motivos else "BLOCKED_FINOPS"
    escenario = resumen_costos["escenarios"]["C_CLOUDFRONT_VPC_ORIGIN_PRIVADO"]
    return {
        "version": "1.0.0",
        "spec": "I-000-cuenta-finops",
        "fecha_evaluacion": hoy.isoformat(),
        "estado": estado,
        "escenario_evaluado": escenario["escenario"],
        "motivos": motivos,
        "condiciones": condiciones,
        "resumen_financiero": {
            "total_bruto_45d_usd": escenario["total_bruto_45d_usd"],
            "total_conocido_usd": escenario["total_conocido_usd"],
            "creditos_verificados_usd": dinero.formatear(creditos_verificados),
            "cobro_neto_usd": resumen_costos["financiero"]["cobro_neto_usd"],
            "impuestos": "UNKNOWN",
            "desembolso_personal_usd": "0.0000",
        },
        "revision_topologia": revision,
        "nota": (
            "VIABLE exige evidencia AWS verificada y precios VERIFIED_AWS; "
            "la ausencia de errores o la existencia de creditos no eleva el estado"
        ),
    }


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Viabilidad FinOps I-000")
    parser.add_argument("--evidencia", type=Path, default=DIR_EVIDENCIA)
    parser.add_argument("--precios", type=Path, default=None)
    parser.add_argument("--topologia", type=Path, default=None)
    parser.add_argument("--salida", type=Path, default=None)
    args = parser.parse_args(argv)

    decision = evaluar(args.evidencia, args.precios, args.topologia)
    texto = json.dumps(decision, indent=2, ensure_ascii=False, sort_keys=True)
    destino = args.salida or (DIR_RESULTADOS / "viabilidad_i000.json")
    destino.parent.mkdir(parents=True, exist_ok=True)
    destino.write_text(texto + "\n", encoding="utf-8")
    print(texto)
    return 0


if __name__ == "__main__":
    sys.exit(main())
