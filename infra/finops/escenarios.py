"""Escenarios de disponibilidad y partidas de costo para I-000 (INF-000-02/03).

Cada partida declara cantidades como cadenas decimales exactas y referencia un
SKU del catálogo ``precios_us-east-2.json``. Ninguna partida introduce precios:
el motor de costos los toma del catálogo, de modo que una tarifa desconocida no
puede convertirse en cero por accidente.
"""

from __future__ import annotations

from dataclasses import dataclass, field

DIAS_VENTANA = 45
HORAS_VENTANA = DIAS_VENTANA * 24  # 1080
MESES_VENTANA = "1.5"


@dataclass(frozen=True)
class Partida:
    """Línea de costo de un escenario."""

    clave: str
    concepto: str
    cantidad: str
    unidad: str
    sku: str
    requerida: bool = True
    nota: str = ""


@dataclass(frozen=True)
class Escenario:
    """Escenario de disponibilidad con su conjunto de partidas."""

    clave: str
    nombre: str
    descripcion: str
    horas_encendido: str
    cumple_24x7: bool
    cumple_dos_instancias: bool
    valido_para_demostracion: bool
    partidas: tuple[Partida, ...]
    condiciones: tuple[str, ...] = field(default=())

    @property
    def partidas_sin_cantidad(self) -> tuple[Partida, ...]:
        return tuple(p for p in self.partidas if p.cantidad.strip() in ("0", "0.0", "0.00"))


def _partidas_base_24x7() -> tuple[Partida, ...]:
    """Partidas comunes del entorno 24x7 con 2 instancias, ALB y Redis OSS.

    Las cantidades reproducen el modelo H05 V3 (FINOPS_45_DIAS_MODELO_V3.json,
    32 partidas, 2026-11-05) para poder verificar su aritmética sin alterarla.
    """

    return (
        Partida("EC2_GO", "EC2 Go (2 instancias)", "2160", "hora-instancia", "EC2_T4G_MICRO_HORA"),
        Partida("ALB_FIJO", "ALB fijo", "1080", "hora-ALB", "ALB_HORA"),
        Partida("ALB_LCU", "ALB LCU", "1080", "LCU-hora", "ALB_LCU_HORA"),
        Partida("REDIS_OSS", "ElastiCache Redis OSS", "1080", "nodo-hora", "ELASTICACHE_REDIS_T4G_MICRO_HORA"),
        Partida("EBS_GP3", "EBS gp3", "30", "GB-mes", "EBS_GP3_GB_MES"),
        Partida("EBS_SNAPSHOTS", "EBS snapshots", "3", "GB-mes", "EBS_SNAPSHOT_GB_MES"),
        Partida("DDB_STORAGE", "DynamoDB storage", "3", "GB-mes", "DDB_STORAGE_GB_MES"),
        Partida("DDB_LECTURAS", "DynamoDB lecturas", "2", "millon-RRU", "DDB_RRU_MILLON"),
        Partida("DDB_ESCRITURAS", "DynamoDB escrituras", "0.6", "millon-WRU", "DDB_WRU_MILLON"),
        Partida("DDB_PITR", "DynamoDB PITR", "3", "GB-mes", "DDB_PITR_GB_MES"),
        Partida("DDB_BACKUP", "DynamoDB backup", "3", "GB-mes", "DDB_BACKUP_GB_MES"),
        Partida("DDB_RESTORE", "DynamoDB restore", "2", "GB-restaurado", "DDB_RESTORE_GB"),
        Partida("CW_INGESTA", "CloudWatch ingesta de logs", "3", "GB", "CW_LOGS_INGESTA_GB"),
        Partida("CW_RETENCION", "CloudWatch retencion de logs", "4.5", "GB-mes", "CW_LOGS_ALMACEN_GB_MES"),
        Partida("CW_METRICAS", "CloudWatch metricas personalizadas", "4.5", "metrica-mes", "CW_METRICA_MES"),
        Partida("CW_ALARMAS", "CloudWatch alarmas", "3", "alarma-mes", "CW_ALARMA_MES"),
        Partida("SECRETS", "Secrets Manager", "3", "secreto-mes", "SECRETS_SECRETO_MES"),
        Partida("S3_STATE", "S3 Terraform state", "3", "GB-mes", "S3_STANDARD_GB_MES"),
        Partida("S3_SOLICITUDES", "S3 solicitudes", "1", "lote", "S3_SOLICITUDES_LOTE"),
        Partida("EGRESO", "Egreso a internet", "10", "GB", "EGRESO_INTERNET_GB"),
        Partida("REDIS_INTERAZ", "Transferencia inter-AZ Redis", "10", "GB", "ELASTICACHE_INTERAZ_GB"),
        Partida("CI_CD", "Reserva CI/CD", "1", "lote", "CI_CD_RESERVA", requerida=False),
        Partida("CERT_CF", "Certificado CloudFront por defecto", "1", "certificado", "CLOUDFRONT_CERTIFICADO"),
    )


def escenario_c_cloudfront_privado() -> Escenario:
    """Topología seleccionable C: CloudFront + VPC Origin + ALB privado + 2 EC2 privadas."""

    return Escenario(
        clave="C_CLOUDFRONT_VPC_ORIGIN_PRIVADO",
        nombre="C — CloudFront + VPC Origin + ALB privado + 2 EC2 privadas (24x7)",
        descripcion=(
            "HTTPS de navegador a CloudFront con dominio *.cloudfront.net; origen VPC hacia ALB "
            "privado; 2 instancias EC2 privadas en AZ distintas; S3 y DynamoDB por gateway "
            "endpoints; Redis OSS privado; sin NAT, sin IPv4 publica, sin Route 53 propio."
        ),
        horas_encendido=str(HORAS_VENTANA),
        cumple_24x7=True,
        cumple_dos_instancias=True,
        valido_para_demostracion=True,
        partidas=_partidas_base_24x7()
        + (
            Partida("IPV4_ALB", "IPv4 publica del ALB", "0", "IP-hora", "IPV4_HORA", nota="ALB interno, sin IP publica"),
            Partida("IPV4_EC2", "IPv4 publica de las instancias", "0", "IP-hora", "IPV4_HORA", nota="EC2 privadas, sin IP publica"),
            Partida("ROUTE53", "Zona Route 53", "0", "zona-mes", "ROUTE53_ZONA_MES", nota="Se usa nombre cloudfront.net"),
            Partida("NAT", "NAT Gateway", "0", "gateway-hora", "NAT_GATEWAY_HORA", nota="Sin NAT por diseno"),
            Partida("IFACE_ENDPOINTS", "Interface endpoints", "0", "endpoint-hora", "INTERFACE_ENDPOINT_HORA", nota="Solo gateway endpoints"),
            Partida("VPC_ORIGIN", "CloudFront VPC Origin", "1", "funcionalidad", "CLOUDFRONT_VPC_ORIGIN"),
            Partida("GW_ENDPOINTS", "Gateway endpoints S3+DynamoDB", "2", "endpoint", "VPC_GATEWAY_ENDPOINT"),
            Partida("RESERVA_CF", "Reserva prudencial CloudFront variable", "1", "reserva", "RESERVA_CLOUDFRONT_VARIABLE", requerida=False),
        ),
        condiciones=(
            "Disponibilidad de CloudFront VPC Origins en la cuenta/SCP no confirmada",
            "TLS extremo a extremo hasta ALB no incluido: tramo CloudFront->ALB es HTTP privado",
            "SKU exacto de ElastiCache Redis OSS pendiente de revalidacion",
        ),
    )


def escenario_a_alb_publico() -> Escenario:
    """Escenario A del SDD: 2 instancias 24x7 con ALB publico (comparador historico)."""

    return Escenario(
        clave="A_24X7_ALB_PUBLICO",
        nombre="A — 2 instancias 24x7 con ALB publico y IPv4 publicas",
        descripcion=(
            "Dos instancias encendidas simultaneamente 24x7 durante 45 dias con ALB "
            "internet-facing e IPv4 publicas por instancia. Es el comparador historico de H05."
        ),
        horas_encendido=str(HORAS_VENTANA),
        cumple_24x7=True,
        cumple_dos_instancias=True,
        valido_para_demostracion=True,
        partidas=_partidas_base_24x7()
        + (
            Partida("IPV4_EC2", "IPv4 publica de las instancias", "2160", "IP-hora", "IPV4_HORA"),
            Partida("ROUTE53", "Zona Route 53", "0", "zona-mes", "ROUTE53_ZONA_MES", nota="No se adquiere dominio"),
        ),
        condiciones=(
            "No reproduce el importe historico V2 103.41 USD por diferencias de SKU; es un recomputo V3",
            "Incluye costo de IPv4 publica de las instancias que la topologia C elimina",
            "No modela IPv4 publica del ALB internet-facing: el importe es un piso, no un techo",
            "Excluye la reserva prudencial de CloudFront porque esta topologia no usa CloudFront",
        ),
    )


def escenario_b_ventanas() -> Escenario:
    """Escenario B del SDD: ventanas de prueba, NO cumple 24x7."""

    horas_ventana = str(DIAS_VENTANA * 8)  # 360 h, 8 h/dia
    partidas = (
        Partida("EC2_GO", "EC2 Go (2 instancias en ventanas)", "720", "hora-instancia", "EC2_T4G_MICRO_HORA"),
        Partida("ALB_FIJO", "ALB fijo en ventanas", horas_ventana, "hora-ALB", "ALB_HORA"),
        Partida("ALB_LCU", "ALB LCU en ventanas", horas_ventana, "LCU-hora", "ALB_LCU_HORA"),
        Partida("REDIS_OSS", "Redis OSS en ventanas", horas_ventana, "nodo-hora", "ELASTICACHE_REDIS_T4G_MICRO_HORA"),
        Partida("EBS_GP3", "EBS gp3 persistente", "30", "GB-mes", "EBS_GP3_GB_MES"),
        Partida("DDB_STORAGE", "DynamoDB storage", "3", "GB-mes", "DDB_STORAGE_GB_MES"),
        Partida("DDB_LECTURAS", "DynamoDB lecturas", "2", "millon-RRU", "DDB_RRU_MILLON"),
        Partida("DDB_ESCRITURAS", "DynamoDB escrituras", "0.6", "millon-WRU", "DDB_WRU_MILLON"),
        Partida("CW_INGESTA", "CloudWatch ingesta de logs", "3", "GB", "CW_LOGS_INGESTA_GB"),
        Partida("S3_STATE", "S3 Terraform state", "3", "GB-mes", "S3_STANDARD_GB_MES"),
        Partida("EGRESO", "Egreso a internet", "10", "GB", "EGRESO_INTERNET_GB"),
    )
    return Escenario(
        clave="B_VENTANAS_8H",
        nombre="B — 2 instancias simultaneas solo en ventanas de prueba (8 h/dia)",
        descripcion=(
            "Reduce el encendido a ventanas de prueba manteniendo 2 instancias simultaneas "
            "solo durante la ventana. NO satisface disponibilidad 24x7 del entorno requerido."
        ),
        horas_encendido=horas_ventana,
        cumple_24x7=False,
        cumple_dos_instancias=True,
        valido_para_demostracion=False,
        partidas=partidas,
        condiciones=("No presentar como 24x7", "No satisface el requisito de disponibilidad continua"),
    )


def escenarios() -> dict[str, Escenario]:
    """Catálogo ordenado de escenarios por clave."""

    catalogo = (
        escenario_c_cloudfront_privado(),
        escenario_a_alb_publico(),
        escenario_b_ventanas(),
    )
    return {e.clave: e for e in catalogo}
