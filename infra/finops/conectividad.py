"""Conectividad privada por modos operativos (I-000, revisión 6).

Política vigente del responsable: la API y DynamoDB permanecen disponibles
prácticamente 24/7 durante 45 días; los componentes **auxiliares** pueden
activarse por ventanas de mantenimiento.

Hallazgo que gobierna el diseño: cada interface endpoint cuesta
**0.01 USD/h por AZ**, y con 2 AZ son 21.60 USD por servicio para 1080 h. Con
79.0430 USD de bruto verificado y 120 USD de créditos, el margen sólo permite
**un** endpoint permanente (21.60 USD): dos permanentes ya dejan el presupuesto
sin cobertura.

Diseño resultante (escenario E):

- **Permanente (operación):** `logs`. Es lo único que el Backend necesita emitir
  de forma continua para conservar observabilidad y alarmas.
- **Métricas y alarmas sin `monitoring`:** se derivan de los propios logs con
  *metric filters* de CloudWatch (evaluación del lado del servicio), de modo que
  no hace falta que el agente llame a `PutMetricData`. No se degrada la
  obligación de métricas/alarmas de GOV-10.
- **Temporal (ventanas de mantenimiento):** `secretsmanager`, `ssm`,
  `ssmmessages`. Los secretos se cachean cifrados en EBS y se refrescan en la
  ventana; SSM sólo se usa para parches, diagnóstico y despliegues.
- **`ec2messages`:** no se presupuesta como permanente; el agente SSM moderno
  opera por `ssmmessages`. Se deja una reserva de contingencia para poder
  habilitarlo dentro de una ventana.
- **Sin NAT:** no se usa en ningún escenario.

Nada de esto se aprovisiona en I-000; el módulo sólo modela coste y horas.
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
MARGEN = "0.10"

PERMANENTE = "PERMANENTE"
TEMPORAL = "TEMPORAL"
NO_REQUERIDO = "NO_REQUERIDO"

# Catálogo de dependencias: quién origina la solicitud y qué modo exige.
SERVICIOS = (
    {
        "servicio": "logs",
        "dependencia": "CloudWatch Logs (observabilidad y alarmas por metric filter)",
        "origen": "Backend Go (emisión continua de logs) y agente de CloudWatch",
        "modo": PERMANENTE,
        "horas": HORAS_VENTANA,
        "justificacion": (
            "Es la única salida continua imprescindible; de sus logs se derivan métricas y "
            "alarmas con metric filters, sin endpoint de monitoring"
        ),
    },
    {
        "servicio": "secretsmanager",
        "dependencia": "Secrets Manager (claves JWT y credenciales)",
        "origen": "Backend Go al arrancar y al rotar",
        "modo": TEMPORAL,
        "justificacion": (
            "Los secretos se cachean cifrados en EBS (sobreviven al reinicio); la ventana se "
            "abre para rotar y refrescar la caché"
        ),
    },
    {
        "servicio": "ssm",
        "dependencia": "Systems Manager (parches, diagnóstico y despliegue)",
        "origen": "Agente SSM y GitHub Actions",
        "modo": TEMPORAL,
        "justificacion": "Sólo se usa en mantenimiento y despliegues; no en el servicio de la API",
    },
    {
        "servicio": "ssmmessages",
        "dependencia": "SSM Messages (canal de comandos del agente)",
        "origen": "Agente SSM",
        "modo": TEMPORAL,
        "justificacion": "Canal de sesión del agente moderno; sólo en ventana",
    },
    {
        "servicio": "monitoring",
        "dependencia": "CloudWatch métricas personalizadas",
        "origen": "Agente de CloudWatch",
        "modo": NO_REQUERIDO,
        "justificacion": (
            "Sustituido por metric filters sobre los logs ya ingeridos: métricas y alarmas "
            "sin endpoint adicional ni pérdida de la obligación de GOV-10"
        ),
    },
    {
        "servicio": "ec2messages",
        "dependencia": "SSM EC2 Messages (canal heredado del agente)",
        "origen": "Agente SSM",
        "modo": NO_REQUERIDO,
        "justificacion": (
            "El agente SSM previsto opera por ssmmessages; si una versión concreta lo exige, se "
            "habilita dentro de la ventana con la reserva de contingencia"
        ),
    },
)

PERMANENTES = tuple(s["servicio"] for s in SERVICIOS if s["modo"] == PERMANENTE)
TEMPORALES = tuple(s["servicio"] for s in SERVICIOS if s["modo"] == TEMPORAL)

# Ventanas de mantenimiento previstas en la operación (8 ventanas de 3 h).
VENTANAS = 8
HORAS_POR_VENTANA = 3
HORAS_TEMPORALES = VENTANAS * HORAS_POR_VENTANA
# Consumo de datos de los endpoints (0.01 USD/GB) y contingencia de ec2messages.
GB_ENDPOINTS = "20"
RESERVA_CONTINGENCIA_USD = "0.50"

DEPENDENCIAS_SIN_RED = (
    {
        "dependencia": "DynamoDB",
        "mecanismo": "GATEWAY_ENDPOINT",
        "costo": "0",
        "fuente": "https://aws.amazon.com/privatelink/pricing/",
    },
    {
        "dependencia": "S3 (artefactos, respaldos, terraform state)",
        "mecanismo": "GATEWAY_ENDPOINT",
        "costo": "0",
        "fuente": "https://aws.amazon.com/privatelink/pricing/",
    },
    {
        "dependencia": "Credenciales de instancia (IAM role)",
        "mecanismo": "LINK_LOCAL (IMDS)",
        "costo": "0",
        "fuente": "https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/instancedata-data-retrieval.html",
    },
    {
        "dependencia": "Sincronización horaria",
        "mecanismo": "LINK_LOCAL (169.254.169.123)",
        "costo": "0",
        "fuente": "https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/set-time.html",
    },
    {
        "dependencia": "Health checks del ALB y tráfico de CloudFront",
        "mecanismo": "ENTRADA por VPC Origins",
        "costo": "0",
        "fuente": "https://docs.aws.amazon.com/AmazonCloudFront/latest/DeveloperGuide/private-content-vpc-origins.html",
    },
)


def precio_endpoint(catalogo: dict | None = None) -> Decimal:
    catalogo = catalogo or json.loads(RUTA_PRECIOS.read_text(encoding="utf-8"))
    item = catalogo["items"][SKU_ENDPOINT]
    if item.get("estado") != "VERIFIED_AWS" or item.get("precio_usd") is None:
        raise ValueError("precio de interface endpoint sin verificar")
    return dinero.dec(item["precio_usd"])


def costo_servicios(
    servicios: tuple[str, ...], horas: int, catalogo: dict | None = None
) -> dict:
    hora = precio_endpoint(catalogo)
    enis = len(servicios) * AZ_REQUERIDAS
    coste = dinero.multiplicar(
        dinero.multiplicar(str(enis), str(hora)), str(horas)
    )
    return {
        "servicios": sorted(servicios),
        "az": AZ_REQUERIDAS,
        "enis": enis,
        "horas": horas,
        "precio_hora_por_az_usd": dinero.formatear(hora),
        "costo_usd": dinero.formatear(coste),
    }


def costo_conectividad(catalogo: dict | None = None) -> dict:
    permanentes = costo_servicios(PERMANENTES, HORAS_VENTANA, catalogo)
    temporales = costo_servicios(TEMPORALES, HORAS_TEMPORALES, catalogo)
    datos = dinero.multiplicar(GB_ENDPOINTS, "0.01")
    total = dinero.sumar(
        [
            permanentes["costo_usd"],
            temporales["costo_usd"],
            dinero.formatear(datos),
            RESERVA_CONTINGENCIA_USD,
        ]
    )
    return {
        "permanentes": permanentes,
        "temporales": temporales,
        "ventanas": VENTANAS,
        "horas_por_ventana": HORAS_POR_VENTANA,
        "horas_temporales": HORAS_TEMPORALES,
        "procesamiento_datos_gb": GB_ENDPOINTS,
        "procesamiento_datos_usd": dinero.formatear(datos),
        "reserva_contingencia_usd": RESERVA_CONTINGENCIA_USD,
        "costo_total_45d_usd": dinero.formatear(total),
        "formula": (
            f"{len(PERMANENTES)} permanentes x {AZ_REQUERIDAS} AZ x {HORAS_VENTANA} h + "
            f"{len(TEMPORALES)} temporales x {AZ_REQUERIDAS} AZ x {HORAS_TEMPORALES} h + datos + reserva"
        ),
    }


def escenarios(catalogo: dict | None = None) -> dict:
    """Compara los escenarios A-E exigidos por la revisión."""

    todos = tuple(s["servicio"] for s in SERVICIOS if s["modo"] != NO_REQUERIDO)

    def _escenario(nombre: str, permanente: tuple[str, ...], temporal: tuple[str, ...]) -> dict:
        p = costo_servicios(permanente, HORAS_VENTANA, catalogo) if permanente else {
            "costo_usd": "0.0000",
            "servicios": [],
            "horas": HORAS_VENTANA,
        }
        t = costo_servicios(temporal, HORAS_TEMPORALES, catalogo) if temporal else {
            "costo_usd": "0.0000",
            "servicios": [],
            "horas": HORAS_TEMPORALES,
        }
        total = dinero.sumar([p["costo_usd"], t["costo_usd"]])
        return {
            "escenario": nombre,
            "permanentes": list(permanente),
            "temporales": list(temporal),
            "costo_permanente_usd": p["costo_usd"],
            "costo_temporal_usd": t["costo_usd"],
            "costo_total_usd": dinero.formatear(total),
        }

    return {
        "A_todos_permanentes": _escenario("A", todos, ()),
        "B_solo_operacion": _escenario("B", PERMANENTES, ()),
        "C_operacion_mas_ventanas": _escenario("C", PERMANENTES, TEMPORALES),
        "D_una_az": {
            "escenario": "D",
            "nota": "Una sola AZ abarata pero convierte el endpoint en punto único de fallo; "
            "no se adopta para un componente crítico",
            "costo_total_usd": dinero.formatear(
                dinero.sumar(
                    [
                        costo_servicios(PERMANENTES, HORAS_VENTANA, catalogo)["costo_usd"],
                        costo_servicios(TEMPORALES, HORAS_TEMPORALES, catalogo)["costo_usd"],
                    ]
                )
                / Decimal(2)
            ),
        },
        "E_optimizado": _escenario("E", PERMANENTES, TEMPORALES),
    }


def recotizar(
    bruto_verificado_usd: str,
    creditos_verificados_usd: str,
    margen_seguridad: str = MARGEN,
    catalogo: dict | None = None,
) -> dict:
    """Presupuesto completo con el diseño E (permanente mínimo + ventanas)."""

    base = dinero.dec(bruto_verificado_usd)
    conectividad = costo_conectividad(catalogo)
    total = dinero.sumar([base, conectividad["costo_total_45d_usd"]])
    requerido = dinero.cuantizar(total * (Decimal(1) + dinero.dec(margen_seguridad)))
    creditos = dinero.dec(creditos_verificados_usd)
    sobrante = dinero.cuantizar(creditos - requerido)
    return {
        "diseno": "E",
        "bruto_verificado_sin_conectividad_usd": dinero.formatear(base),
        "costo_conectividad_usd": conectividad["costo_total_45d_usd"],
        "detalle_conectividad": conectividad,
        "bruto_total_usd": dinero.formatear(total),
        "margen_seguridad": dinero.formatear(dinero.dec(margen_seguridad)),
        "creditos_requeridos_usd": dinero.formatear(requerido),
        "creditos_verificados_usd": dinero.formatear(creditos),
        "cubierto": bool(requerido <= creditos),
        "importe_sin_cobertura_usd": dinero.formatear(dinero.maximo_cero(requerido - creditos)),
        "sobrante_usd": dinero.formatear(dinero.maximo_cero(sobrante)),
        "consumo_estimado_por_dia_usd": dinero.formatear(
            total / dinero.dec(str(HORAS_VENTANA // 24))
        ),
    }


def evaluar(catalogo: dict | None = None) -> dict:
    return {
        "version": "2.0.0",
        "fecha": "2026-11-06",
        "politica": "API y DynamoDB 24/7; auxiliares por ventanas de mantenimiento",
        "servicios": [dict(s) for s in SERVICIOS],
        "permanentes": list(PERMANENTES),
        "temporales": list(TEMPORALES),
        "sin_red": [dict(d) for d in DEPENDENCIAS_SIN_RED],
        "conectividad": costo_conectividad(catalogo),
        "escenarios": escenarios(catalogo),
        "nota": (
            "Un solo endpoint permanente cabe en el presupuesto; dos ya lo exceden. "
            "monitoring se sustituye por metric filters sobre logs; ec2messages por ssmmessages."
        ),
    }


def main() -> int:
    print(json.dumps(evaluar(), indent=2, ensure_ascii=False, sort_keys=True))
    print(json.dumps(recotizar("79.0430", "120"), indent=2, ensure_ascii=False, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
