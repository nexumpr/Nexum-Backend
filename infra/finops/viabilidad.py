"""Evaluación de viabilidad FinOps de I-000 (INF-000-05), revisión 2.

Corrección C: ``VIABLE`` exige evidencia completa y riesgos resueltos. Antes
bastaba con que no hubiera errores. Ahora se bloquea si falta cualquiera de:

1. cuenta AWS verificada, plan ``FREE`` y vigencia que cubra los 45 días;
2. créditos verificados suficientes para el **bruto verificado** más un margen;
3. precios ``VERIFIED_AWS`` de todas las partidas requeridas (incluido Redis OSS);
4. cuota de vCPU y disponibilidad de ``t4g.micro`` en dos zonas;
5. CloudFront VPC Origins confirmado y ALB/CloudFront cotizados;
6. impuestos determinados (no se asume 0 %);
7. TLS conforme a la decisión humana (tramo CloudFront→ALB resuelto);
8. topología con diseño validado y runtime explícitamente ``NOT_RUN``.

Ninguna condición se relaja para facilitar la aprobación.
"""

from __future__ import annotations

import argparse
import json
import sys
from datetime import date, timedelta
from pathlib import Path

import dinero
import modelo_costos
import requisitos

DIR_EVIDENCIA = Path(__file__).with_name("evidencia")
DIR_RESULTADOS = Path(__file__).with_name("resultados")
ESTADO_VERIFICADO = "VERIFIED_AWS"
ESTADO_IMPUESTOS_DETERMINADOS = "DETERMINADO"

# Márgenes exigidos por el Gate H08-I: el bruto verificado no se compara con el
# saldo "a pelo". 10 % cubre variabilidad de LCU, egreso y solicitudes.
MARGEN_SEGURIDAD = "0.10"
DIAS_REQUERIDOS = 45


def _cargar_evidencia(dir_evidencia: Path, identificador: str) -> dict | None:
    archivos = sorted(Path(dir_evidencia).glob(f"{identificador}_*.json"))
    if not archivos:
        return None
    return json.loads(archivos[-1].read_text(encoding="utf-8"))


def _motivo(codigo: str, detalle: str, evidencia_requerida: str) -> dict:
    return {"codigo": codigo, "detalle": detalle, "evidencia_requerida": evidencia_requerida}


def _fecha(texto: object) -> date | None:
    if texto is None:
        return None
    try:
        return date.fromisoformat(str(texto)[:10])
    except ValueError:
        return None


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
                f"estado de evidencia de cuenta: {cuenta.get('estado')} "
                f"({cuenta.get('motivo', 'sin motivo')})",
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
            _motivo(
                "PLAN_NO_FREE",
                f"plan observado {plan}; I-000 exige plan Free",
                "confirmar plan Free activo",
            )
        )

    expiracion = _fecha(datos.get("fecha_expiracion"))
    if datos.get("fecha_expiracion") is None:
        motivos.append(
            _motivo(
                "VIGENCIA_NO_DETERMINADA",
                "fecha de expiracion del plan desconocida",
                "aws freetier get-account-plan-state",
            )
        )
    elif expiracion is None:
        motivos.append(
            _motivo(
                "VIGENCIA_ILEGIBLE",
                f"fecha de expiracion no parseable: {datos.get('fecha_expiracion')}",
                "formato ISO",
            )
        )
    else:
        minimo = hoy + timedelta(days=DIAS_REQUERIDOS)
        if expiracion < minimo:
            motivos.append(
                _motivo(
                    "VIGENCIA_INSUFICIENTE_45_DIAS",
                    f"el plan vence {expiracion.isoformat()} y se requieren "
                    f"{DIAS_REQUERIDOS} dias (hasta {minimo.isoformat()})",
                    "plan Free con vigencia que cubra la ventana completa",
                )
            )
        else:
            condiciones.append(
                f"Vigencia declarada hasta {expiracion.isoformat()}: cubre la ventana de "
                f"{DIAS_REQUERIDOS} dias; no garantiza que los creditos no se agoten antes"
            )

    impuestos = str(datos.get("impuestos") or "UNKNOWN").upper()
    if impuestos != ESTADO_IMPUESTOS_DETERMINADOS:
        motivos.append(
            _motivo(
                "IMPUESTOS_NO_DETERMINADOS",
                f"estado fiscal observado: {impuestos}; no se asume IVA 0 %",
                "determinacion fiscal (IVA) responsable",
            )
        )

    creditos = datos.get("creditos_verificados_usd")
    if creditos is None or dinero.dec(creditos) <= 0:
        motivos.append(
            _motivo(
                "CREDITOS_NO_VERIFICADOS",
                f"creditos verificados: {creditos}; los declarados no son evidencia",
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
                "consultas describe/list read-only de ElastiCache, CloudFront, "
                "Service Quotas y EC2",
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
                "disponibilidad de CloudFront VPC Origins no confirmada "
                "(list-vpc-origins devuelve la lista, no la elegibilidad de la cuenta)",
                "confirmacion en cuenta y prueba en fase autorizada",
            )
        )

    cuota = datos.get("cuota_vcpu_on_demand")
    if cuota is None or int(cuota) < 2:
        motivos.append(
            _motivo(
                "CUOTA_EC2_INSUFICIENTE",
                f"cuota de vCPU On-Demand observada: {cuota}; se requieren 2 instancias",
                "aws servicequotas get-service-quota (L-1216C47A)",
            )
        )
    zonas = datos.get("az_con_t4g_micro")
    if not isinstance(zonas, list) or len(zonas) < 2:
        motivos.append(
            _motivo(
                "TIPO_INSTANCIA_NO_CONFIRMADO",
                "t4g.micro no confirmado en al menos dos zonas de disponibilidad",
                "aws ec2 describe-instance-type-offerings",
            )
        )
    if datos.get("dynamodb_limites") is None:
        condiciones.append("Limites de DynamoDB no verificados; no impiden el diseno actual")
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
                "consola de facturacion (accion humana) o autorizacion especifica de "
                "Cost Explorer",
            )
        )
    condiciones.append(
        "Cost Explorer tiene costo por solicitud (USD 0.01); su uso requiere aprobacion "
        "humana concreta"
    )
    return motivos, condiciones


def _evaluar_precios(
    resumen_costos: dict, catalogo: dict
) -> tuple[list[dict], list[str]]:
    motivos: list[dict] = []
    condiciones: list[str] = []
    escenario = resumen_costos["escenarios"]["C_CLOUDFRONT_VPC_ORIGIN_PRIVADO"]
    def _cuenta_como_verificada(partida: dict) -> bool:
        if partida["estado_evidencia"] == ESTADO_VERIFICADO:
            return True
        return (
            partida["estado_evidencia"] == modelo_costos.ESTADO_SIN_CARGO_OFICIAL
            and partida["subtotal_usd"] == "0.0000"
            and bool(partida.get("fuente_oficial"))
        )

    no_verificadas = [
        p["clave"]
        for p in escenario["partidas"]
        if p["requerida"] and dinero.dec(p["cantidad"]) > 0 and not _cuenta_como_verificada(p)
    ]
    reservas = [
        p["clave"] for p in escenario["partidas"] if not p["requerida"] and p["subtotal_usd"]
    ]
    if reservas:
        reserva_total = dinero.sumar(
            [p["subtotal_usd"] for p in escenario["partidas"] if not p["requerida"] and p["subtotal_usd"]]
        )
        condiciones.append(
            f"El bruto verificado excluye reservas de modelado no facturables ({', '.join(reservas)}): "
            f"{dinero.formatear(reserva_total)} USD"
        )
    if no_verificadas:
        motivos.append(
            _motivo(
                "PRECIOS_NO_VERIFICADOS",
                f"{len(no_verificadas)} partidas requeridas sin precio verificado en AWS: "
                f"{', '.join(no_verificadas)}",
                "aws pricing get-products por SKU en us-east-2 y actualizacion del catalogo",
            )
        )
    if escenario["total_bruto_verificado_45d_usd"] is None:
        motivos.append(
            _motivo(
                "COSTO_VERIFICADO_DESCONOCIDO",
                "el bruto verificado a 45 dias no puede calcularse: hay partidas requeridas "
                "sin precio VERIFIED_AWS",
                "catalogo completo con evidencia AWS para todas las partidas requeridas",
            )
        )
    if escenario["total_bruto_45d_usd"] is None:
        motivos.append(
            _motivo(
                "COSTO_BRUTO_DESCONOCIDO",
                "el total bruto a 45 dias no puede calcularse (prioridad: partidas criticas)",
                "catalogo de precios completo para todas las partidas requeridas",
            )
        )

    redis = catalogo["items"].get("ELASTICACHE_REDIS_T4G_MICRO_HORA") or {}
    if redis.get("estado") != ESTADO_VERIFICADO or redis.get("precio_usd") is None:
        motivos.append(
            _motivo(
                "REDIS_OSS_SIN_PRECIO_VERIFICADO",
                f"precio de ElastiCache Redis OSS con estado {redis.get('estado')}",
                "aws pricing get-products serviceCode=AmazonElastiCache instanceType=cache.t4g.micro",
            )
        )
    alb = catalogo["items"].get("ALB_HORA") or {}
    if alb.get("estado") != ESTADO_VERIFICADO:
        motivos.append(
            _motivo(
                "ALB_SIN_PRECIO_VERIFICADO",
                f"precio de ALB con estado {alb.get('estado')}",
                "aws pricing get-products serviceCode=AWSELB",
            )
        )
    cf = catalogo["items"].get("EGRESO_INTERNET_GB") or {}
    if cf.get("estado") != ESTADO_VERIFICADO:
        motivos.append(
            _motivo(
                "CLOUDFRONT_SIN_PRECIO_VERIFICADO",
                f"precio de egreso CloudFront con estado {cf.get('estado')}",
                "aws pricing get-products serviceCode=AmazonCloudFront",
            )
        )

    supuestos = [p["clave"] for p in escenario["partidas"] if p["estado_evidencia"] == "SUPUESTO"]
    if supuestos:
        condiciones.append(
            f"Partidas de reserva/supuesto no tarifarias incluidas: {', '.join(supuestos)}"
        )
    return motivos, condiciones


def _evaluar_financiero(
    financiero: dict, creditos_verificados: str
) -> tuple[list[dict], list[str]]:
    motivos: list[dict] = []
    condiciones: list[str] = []

    if financiero["costo_bruto_verificado_usd"] is None:
        motivos.append(
            _motivo(
                "COBERTURA_NO_CALCULABLE",
                "sin bruto verificado no puede afirmarse cobertura de creditos",
                "precios VERIFIED_AWS de todas las partidas requeridas",
            )
        )
        return motivos, condiciones

    if financiero["cobertura_creditos"] != "TOTAL":
        motivos.append(
            _motivo(
                "CREDITOS_INSUFICIENTES",
                f"bruto verificado {financiero['costo_bruto_verificado_usd']} USD con margen "
                f"{financiero['margen_seguridad']} requiere "
                f"{financiero['creditos_requeridos_usd']} USD; verificados "
                f"{dinero.formatear(creditos_verificados)} USD "
                f"(cobertura {financiero['cobertura_creditos']}, sin cubrir "
                f"{financiero['importe_sin_cobertura_usd']})",
                "verificar saldo real de creditos y reoptimizar topologia",
            )
        )

    if financiero["riesgo_cobro_personal"]:
        motivos.append(
            _motivo(
                "RIESGO_COBRO_PERSONAL",
                "existe importe potencialmente sin cobertura o impuestos indeterminados; "
                "la politica de gasto personal es cero, pero cero desembolso no es prueba "
                "de que no haya cobro",
                "cobertura TOTAL verificada e impuestos determinados",
            )
        )
    condiciones.append(
        "Politica de gasto personal: CERO. No se contempla pago con fondos propios ni plan Paid"
    )
    return motivos, condiciones


def evaluar(
    dir_evidencia: Path = DIR_EVIDENCIA,
    ruta_precios: Path | None = None,
    ruta_topologia: Path | None = None,
    hoy: date | None = None,
    margen_seguridad: str = MARGEN_SEGURIDAD,
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
        if cuenta.get("estado") == ESTADO_VERIFICADO:
            creditos_verificados = str(cuenta["datos"]["creditos_verificados_usd"])

    impuestos_determinados = (
        cuenta is not None
        and cuenta.get("estado") == ESTADO_VERIFICADO
        and str((cuenta.get("datos") or {}).get("impuestos", "UNKNOWN")).upper()
        == ESTADO_IMPUESTOS_DETERMINADOS
    )
    resumen_costos = modelo_costos.calcular_todo(
        catalogo, creditos_verificados, margen_seguridad, impuestos_determinados
    )
    financiero = resumen_costos["financiero"]

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
    m, c = _evaluar_precios(resumen_costos, catalogo)
    motivos += m
    condiciones += c
    m, c = _evaluar_financiero(financiero, creditos_verificados)
    motivos += m
    condiciones += c

    if free_tier is None or free_tier.get("estado") != ESTADO_VERIFICADO:
        condiciones.append("Uso del plan Free Tier no verificado (aws freetier get-free-tier-usage)")

    ruta_topo = ruta_topologia or (Path(__file__).with_name("evidencia") / "topologia_c_2026-11-06.json")
    topologia = json.loads(Path(ruta_topo).read_text(encoding="utf-8"))
    revision = requisitos.validar(topologia, dir_evidencia)

    if revision["estado_diseno"] != requisitos.ESTADO_DISENO_OK:
        motivos.append(
            _motivo(
                "TOPOLOGIA_INCUMPLE",
                "; ".join(
                    revision["dos_instancias"]
                    + revision["conectividad_privada"]
                    + revision["autocertificacion"]
                ),
                "corregir el diseno de dos instancias y conectividad privada",
            )
        )

    if not _tls_resuelto(topologia):
        motivos.append(
            _motivo(
                "TLS_NO_RESUELTO",
                "el tramo CloudFront->ALB es HTTP privado y no existe decision humana que lo "
                "acepte ni certificado extremo a extremo",
                "aceptacion documentada o recotizacion con ACM y nombre valido",
            )
        )

    condiciones.extend(revision["tls"])
    condiciones.append(
        "Instancias y balanceo reales: verificacion runtime "
        f"{revision['estado_despliegue']} hasta que exista autorizacion de despliegue"
    )

    bloques_bloqueantes = len(motivos)
    estado = "VIABLE" if bloques_bloqueantes == 0 else "BLOCKED_FINOPS"
    escenario = resumen_costos["escenarios"]["C_CLOUDFRONT_VPC_ORIGIN_PRIVADO"]
    return {
        "version": "2.0.0",
        "spec": "I-000-cuenta-finops",
        "fecha_evaluacion": hoy.isoformat(),
        "estado": estado,
        "escenario_evaluado": escenario["escenario"],
        "motivos": motivos,
        "motivos_bloqueantes": bloques_bloqueantes,
        "condiciones": condiciones,
        "resumen_financiero": {
            "costo_bruto_calculado_usd": financiero["costo_bruto_calculado_usd"],
            "costo_bruto_verificado_usd": financiero["costo_bruto_verificado_usd"],
            "partidas_sin_verificar": escenario["partidas_sin_verificar"],
            "creditos_declarados_usd": financiero["creditos_declarados_usd"],
            "creditos_verificados_usd": financiero["creditos_verificados_usd"],
            "margen_seguridad": financiero["margen_seguridad"],
            "creditos_requeridos_usd": financiero["creditos_requeridos_usd"],
            "cobertura_creditos": financiero["cobertura_creditos"],
            "importe_sin_cobertura_usd": financiero["importe_sin_cobertura_usd"],
            "impuestos": financiero["impuestos"],
            "politica_gasto_personal": financiero["politica_gasto_personal"],
            "desembolso_personal_autorizado_usd": financiero["desembolso_personal_autorizado_usd"],
            "riesgo_cobro_personal": financiero["riesgo_cobro_personal"],
            "residual_apagado_usd": escenario["residual_apagado_usd"],
        },
        "revision_topologia": revision,
        "nota": (
            "VIABLE exige cuenta Free verificada, vigencia de 45 dias, precios VERIFIED_AWS, "
            "cobertura TOTAL con margen, impuestos determinados y TLS resuelto; la ausencia "
            "de errores o la existencia de creditos no eleva el estado"
        ),
    }


def _tls_resuelto(topologia: dict) -> bool:
    tls = topologia.get("tls") or {}
    if tls.get("origen_extremo_a_extremo") is True:
        return True
    return tls.get("aceptacion_humana_documentada") is True


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Viabilidad FinOps I-000")
    parser.add_argument("--evidencia", type=Path, default=DIR_EVIDENCIA)
    parser.add_argument("--precios", type=Path, default=None)
    parser.add_argument("--topologia", type=Path, default=None)
    parser.add_argument("--margen-seguridad", default=MARGEN_SEGURIDAD)
    parser.add_argument("--salida", type=Path, default=None)
    args = parser.parse_args(argv)

    decision = evaluar(
        args.evidencia, args.precios, args.topologia, margen_seguridad=args.margen_seguridad
    )
    texto = json.dumps(decision, indent=2, ensure_ascii=False, sort_keys=True)
    destino = args.salida or (DIR_RESULTADOS / "viabilidad_i000.json")
    destino.parent.mkdir(parents=True, exist_ok=True)
    destino.write_text(texto + "\n", encoding="utf-8")
    print(texto)
    return 0


if __name__ == "__main__":
    sys.exit(main())
