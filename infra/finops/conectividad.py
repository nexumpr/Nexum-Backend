"""Análisis de conectividad privada para dos EC2 sin NAT (I-000, revisión 5).

Objetivo: determinar, para cada dependencia real de una instancia privada, si
necesita salida de red y por qué mecanismo se resuelve. Los **gateway
endpoints** sólo cubren S3 y DynamoDB; cualquier otra API de AWS requiere
**interface endpoint** (PrivateLink) o NAT Gateway.

Precio verificado (Price List API, 2026-11-06): interface endpoint
`USE2-VpcEndpoint-Hours` = 0.01 USD/h **por AZ**. Con 2 AZ y 1080 h:
0.01 × 2 × 1080 = 21.60 USD por servicio para la ventana de 45 días.

Nada de esto se aprovisiona en I-000: el módulo sólo calcula el coste
obligatorio que el diseño vigente no cubre.
"""

from __future__ import annotations

import json
from decimal import Decimal
from pathlib import Path

import dinero

RAIZ = Path(__file__).resolve().parents[2]
RUTA_PRECIOS = Path(__file__).with_name("precios_us-east-2.json")
SKU_ENDPOINT = "VPC_INTERFACE_ENDPOINT_HORA"
HORAS_VENTANA = 1080
AZ_REQUERIDAS = 2

# Mecanismos admitidos y su naturaleza.
GATEWAY_ENDPOINT = "GATEWAY_ENDPOINT"
INTERFACE_ENDPOINT = "INTERFACE_ENDPOINT"
SIN_EGRESO = "SIN_EGRESO"
LINK_LOCAL = "LINK_LOCAL"

# Dependencias declaradas por el SDD para el Backend en subred privada.
DEPENDENCIAS = (
    {
        "dependencia": "DynamoDB",
        "requiere_red": True,
        "mecanismo": GATEWAY_ENDPOINT,
        "servicio_endpoint": None,
        "justificacion": "gateway endpoint nativo, sin cargo por hora ni por GB",
        "fuente": "https://aws.amazon.com/privatelink/pricing/",
    },
    {
        "dependencia": "S3 (artefactos, respaldos, terraform state)",
        "requiere_red": True,
        "mecanismo": GATEWAY_ENDPOINT,
        "servicio_endpoint": None,
        "justificacion": "gateway endpoint nativo, sin cargo por hora ni por GB",
        "fuente": "https://aws.amazon.com/privatelink/pricing/",
    },
    {
        "dependencia": "Secrets Manager (credenciales JWT, claves)",
        "requiere_red": True,
        "mecanismo": INTERFACE_ENDPOINT,
        "servicio_endpoint": "secretsmanager",
        "justificacion": "no existe gateway endpoint; requiere PrivateLink o NAT",
        "fuente": "https://docs.aws.amazon.com/secretsmanager/latest/userguide/vpc-endpoint-overview.html",
    },
    {
        "dependencia": "CloudWatch Logs (observabilidad del backend)",
        "requiere_red": True,
        "mecanismo": INTERFACE_ENDPOINT,
        "servicio_endpoint": "logs",
        "justificacion": "ingesta de logs desde subred privada sin NAT exige interface endpoint",
        "fuente": "https://docs.aws.amazon.com/AmazonCloudWatch/latest/logs/cloudwatch-logs-and-interface-VPC.html",
    },
    {
        "dependencia": "CloudWatch metrics (métricas personalizadas)",
        "requiere_red": True,
        "mecanismo": INTERFACE_ENDPOINT,
        "servicio_endpoint": "monitoring",
        "justificacion": "métricas del agente requieren interface endpoint o NAT",
        "fuente": "https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/cloudwatch-and-interface-VPC.html",
    },
    {
        "dependencia": "SSM (parches, comandos y despliegue en instancias privadas)",
        "requiere_red": True,
        "mecanismo": INTERFACE_ENDPOINT,
        "servicio_endpoint": "ssm",
        "justificacion": "sin NAT, el agente SSM necesita interface endpoints",
        "fuente": "https://docs.aws.amazon.com/systems-manager/latest/userguide/systems-manager-privatelink.html",
    },
    {
        "dependencia": "SSM Messages (canal de comandos)",
        "requiere_red": True,
        "mecanismo": INTERFACE_ENDPOINT,
        "servicio_endpoint": "ssmmessages",
        "justificacion": "canal de sesión/comandos del agente SSM",
        "fuente": "https://docs.aws.amazon.com/systems-manager/latest/userguide/systems-manager-privatelink.html",
    },
    {
        "dependencia": "EC2 Messages (canal del agente)",
        "requiere_red": True,
        "mecanismo": INTERFACE_ENDPOINT,
        "servicio_endpoint": "ec2messages",
        "justificacion": "canal del agente SSM",
        "fuente": "https://docs.aws.amazon.com/systems-manager/latest/userguide/systems-manager-privatelink.html",
    },
    {
        "dependencia": "Credenciales de instancia (IAM role)",
        "requiere_red": False,
        "mecanismo": LINK_LOCAL,
        "servicio_endpoint": None,
        "justificacion": "IMDS entrega credenciales temporales sin salida a internet",
        "fuente": "https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/instancedata-data-retrieval.html",
    },
    {
        "dependencia": "Sincronización horaria",
        "requiere_red": False,
        "mecanismo": LINK_LOCAL,
        "servicio_endpoint": None,
        "justificacion": "Amazon Time Sync 169.254.169.123 es link-local, sin cargo ni NAT",
        "fuente": "https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/set-time.html",
    },
    {
        "dependencia": "Health checks del ALB y tráfico de CloudFront",
        "requiere_red": False,
        "mecanismo": SIN_EGRESO,
        "servicio_endpoint": None,
        "justificacion": "tráfico entrante por VPC Origins; no requiere salida de la instancia",
        "fuente": "https://docs.aws.amazon.com/AmazonCloudFront/latest/DeveloperGuide/private-content-vpc-origins.html",
    },
)

ENDPOINTS_OBLIGATORIOS_SDD = (
    "secretsmanager",
    "logs",
    "monitoring",
    "ssm",
    "ssmmessages",
    "ec2messages",
)

# Subconjunto mínimo si se renuncia a parches por SSM en línea (no autorizado:
# cambiaría requisitos de administración). Se informa como comparador.
ENDPOINTS_MINIMOS = ("secretsmanager", "logs")


def precio_endpoint(catalogo: dict | None = None) -> Decimal:
    catalogo = catalogo or json.loads(RUTA_PRECIOS.read_text(encoding="utf-8"))
    item = catalogo["items"][SKU_ENDPOINT]
    if item.get("estado") != "VERIFIED_AWS" or item.get("precio_usd") is None:
        raise ValueError("precio de interface endpoint sin verificar")
    return dinero.dec(item["precio_usd"])


def costo_endpoints(servicios: tuple[str, ...], catalogo: dict | None = None) -> dict:
    """Coste de N interface endpoints en 2 AZ durante 1080 h (Decimal exacto)."""

    hora = precio_endpoint(catalogo)
    enis = len(servicios) * AZ_REQUERIDAS
    horas = dinero.dec(str(HORAS_VENTANA))
    coste = dinero.multiplicar(dinero.multiplicar(str(enis), str(hora)), str(HORAS_VENTANA))
    return {
        "servicios": sorted(servicios),
        "endpoints_por_servicio": 1,
        "az": AZ_REQUERIDAS,
        "enis_totales": enis,
        "horas": str(HORAS_VENTANA),
        "precio_hora_por_az_usd": dinero.formatear(hora),
        "costo_45d_usd": dinero.formatear(coste),
        "formula": f"{len(servicios)} servicios x {AZ_REQUERIDAS} AZ x {HORAS_VENTANA} h x {dinero.formatear(hora)} USD",
        "horas_decimal": dinero.formatear(horas),
    }


def evaluar(catalogo: dict | None = None) -> dict:
    """Clasifica cada dependencia y cuantifica lo que el diseño no cubre."""

    obligatorios = costo_endpoints(ENDPOINTS_OBLIGATORIOS_SDD, catalogo)
    minimos = costo_endpoints(ENDPOINTS_MINIMOS, catalogo)
    dependencias_sin_solucion_gratuita = [
        d["dependencia"]
        for d in DEPENDENCIAS
        if d["mecanismo"] == INTERFACE_ENDPOINT
    ]
    return {
        "version": "1.0.0",
        "fecha": "2026-11-06",
        "topologia": "2 EC2 privadas en 2 AZ, sin NAT, gateway endpoints S3/DynamoDB",
        "dependencias": [dict(d) for d in DEPENDENCIAS],
        "gateway_endpoints_cubren": ["DynamoDB", "S3"],
        "interface_endpoints_requeridos": sorted(ENDPOINTS_OBLIGATORIOS_SDD),
        "dependencias_sin_solucion_gratuita": dependencias_sin_solucion_gratuita,
        "costo_obligatorio_45d_usd": obligatorios["costo_45d_usd"],
        "detalle_obligatorio": obligatorios,
        "costo_minimo_sin_ssm_45d_usd": minimos["costo_45d_usd"],
        "detalle_minimo": minimos,
        "nota": (
            "Los gateway endpoints sólo cubren S3 y DynamoDB. Secrets Manager, CloudWatch Logs, "
            "metricas y SSM exigen interface endpoint o NAT; ambos tienen costo. Renunciar a SSM "
            "en linea cambiaria requisitos de administracion del SDD y no se asume."
        ),
    }


def recotizar(
    bruto_verificado_usd: str,
    creditos_verificados_usd: str,
    margen_seguridad: str = "0.10",
    catalogo: dict | None = None,
    incluir_ssm: bool = True,
) -> dict:
    """Recalcula el presupuesto completo con la conectividad obligatoria."""

    base = dinero.dec(bruto_verificado_usd)
    if incluir_ssm:
        conectividad = dinero.dec(evaluar(catalogo)["costo_obligatorio_45d_usd"])
        servicios = ENDPOINTS_OBLIGATORIOS_SDD
    else:
        conectividad = dinero.dec(evaluar(catalogo)["costo_minimo_sin_ssm_45d_usd"])
        servicios = ENDPOINTS_MINIMOS

    total = dinero.sumar([base, conectividad])
    requerido = dinero.cuantizar(total * (Decimal(1) + dinero.dec(margen_seguridad)))
    creditos = dinero.dec(creditos_verificados_usd)
    return {
        "bruto_verificado_sin_conectividad_usd": dinero.formatear(base),
        "costo_conectividad_usd": dinero.formatear(conectividad),
        "endpoints_incluidos": sorted(servicios),
        "bruto_total_usd": dinero.formatear(total),
        "margen_seguridad": dinero.formatear(dinero.dec(margen_seguridad)),
        "creditos_requeridos_usd": dinero.formatear(requerido),
        "creditos_verificados_usd": dinero.formatear(creditos),
        "cubierto": bool(requerido <= creditos),
        "importe_sin_cobertura_usd": dinero.formatear(dinero.maximo_cero(requerido - creditos)),
    }


def main() -> int:
    print(json.dumps(evaluar(), indent=2, ensure_ascii=False, sort_keys=True))
    print(
        json.dumps(
            recotizar("79.0430", "120", incluir_ssm=False),
            indent=2,
            ensure_ascii=False,
            sort_keys=True,
        )
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
