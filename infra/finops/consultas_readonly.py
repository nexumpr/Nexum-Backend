"""Catálogo y ejecutor de consultas AWS de SOLO LECTURA para I-000.

Correcciones aplicadas (H08-I, revisión 2):

- Los filtros de ``aws pricing get-products`` se modelan como datos estructurados
  y se renderizan en **un único** argumento ``--filters`` con la sintaxis
  abreviada oficial (``Type=TERM_MATCH,Field=...,Value=...``). Antes se repetía
  la opción ``--filters`` N veces, lo que producía argumentos ambiguos.
- Cada consulta declara un ``region_endpoint`` explícito (región del endpoint de
  la API) y, cuando aplica, una ``region_datos`` (región comercial cuyos precios
  se consultan). El comando resultante contiene **exactamente un** ``--region``.
  La Price List API sólo está disponible en ``us-east-1`` (y ``ap-south-1``);
  antes se emitía ``--region us-east-2`` y ``--region us-east-1`` a la vez.
- Los códigos de servicio se validan contra ``CODIGOS_SERVICIO_VALIDADOS``; el
  código de Elastic Load Balancing en Price List es ``AWSELB``, no
  ``ElasticLoadBalancing``.
- Cada consulta de precios declara la familia de producto y los atributos que
  debe cumplir el ítem, y la unidad esperada; así no se confunde capacidad
  reservada (``ReservedLCUUsage``) con consumo on-demand (``LCUUsage``).

Reglas de seguridad:

- Sólo se permiten verbos de lectura; el catálogo se valida en tiempo de carga.
- La salida cruda se escribe fuera del repositorio; al repositorio sólo va la
  versión sanitizada.
- Las consultas con costo por solicitud (Cost Explorer) NO se ejecutan sin
  ``--aprobar-facturable``; por defecto quedan NOT_RUN para decisión humana.
"""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
import tempfile
from dataclasses import dataclass, field
from pathlib import Path

import sanitizar

VERBOS_LECTURA = (
    "get-",
    "list-",
    "describe-",
    "simulate-",
    "search-",
    "lookup-",
    "batch-get-",
)

# Verbos mutantes prohibidos de forma explícita (defensa en profundidad).
VERBOS_PROHIBIDOS = (
    "create-",
    "update-",
    "delete-",
    "put-",
    "modify-",
    "start-",
    "stop-",
    "terminate-",
    "run-",
    "attach-",
    "detach-",
    "associate-",
    "disassociate-",
    "authorize-",
    "revoke-",
    "tag-",
    "untag-",
    "purchase-",
    "allocate-",
    "release-",
    "reboot-",
    "reset-",
    "enable-",
    "disable-",
    "import-",
)

# Códigos verificados contra AWS Price List (los que fallan devuelven
# ResourceNotFoundException; aquí sólo se admiten los confirmados).
CODIGOS_SERVICIO_VALIDADOS = frozenset(
    {
        "AmazonEC2",
        "AmazonElastiCache",
        "AWSELB",
        "AmazonDynamoDB",
        "AmazonCloudFront",
        "AmazonS3",
        "AmazonCloudWatch",
        "AmazonVPC",
        "AWSSecretsManager",
        "AWSDataTransfer",
    }
)

REGION_ENDPOINT_PRICING = "us-east-1"
REGION_COMERCIAL_DATOS = "us-east-2"
ETIQUETA_REGION_COMERCIAL = "US East (Ohio)"
ETIQUETA_REGION_GLOBAL = "United States"


@dataclass(frozen=True)
class Filtro:
    """Filtro TERM_MATCH de Price List: campo y valor exacto."""

    campo: str
    valor: str

    def render(self) -> str:
        return f"Type=TERM_MATCH,Field={self.campo},Value={self.valor}"


@dataclass(frozen=True)
class Consulta:
    id: str
    servicio: str
    descripcion: str
    servicio_api: str
    operacion: str
    argumentos: tuple[str, ...] = ()
    filtros: tuple[Filtro, ...] = ()
    region_endpoint: str | None = None
    region_datos: str | None = None
    clasificacion: str = "SIN_CARGO"
    costo_estimado_usd: str | None = "0"
    justificacion_costo: str = ""
    fuente_costo: str = ""
    requiere_aprobacion_especifica: bool = False
    codigo_servicio: str | None = None
    familia_producto: str | None = None
    atributos_esperados: tuple[tuple[str, str], ...] = ()
    unidad_esperada: str | None = None

    @property
    def es_pricing(self) -> bool:
        return self.servicio_api == "pricing"

    def comando(
        self,
        profile: str,
        region: str,
        account_id: str | None = None,
    ) -> list[str]:
        """Construye argv con exactamente un ``--region`` y un ``--filters``."""

        salida: list[str] = ["aws", self.servicio_api, self.operacion]
        for arg in self.argumentos:
            salida.append(
                arg.replace("{profile}", profile)
                .replace("{region}", region)
                .replace("{account_id}", account_id or "{account_id}")
            )
        if self.filtros:
            salida.append("--filters")
            salida.extend(filtro.render() for filtro in self.filtros)
        salida.extend(["--profile", profile])
        salida.extend(["--region", self.region_endpoint or region])
        salida.extend(["--output", "json"])
        return salida


def _filtro(campo: str, valor: str) -> Filtro:
    return Filtro(campo=campo, valor=valor)


def _consulta_identidad() -> Consulta:
    return Consulta(
        id="sts_identidad",
        servicio="AWS STS",
        descripcion="Identidad efectiva de la sesion (cuenta y ARN, sanitizados)",
        servicio_api="sts",
        operacion="get-caller-identity",
        justificacion_costo="Llamada de identidad sin tarifa publicada por solicitud",
        fuente_costo="AWS STS API (consulta de identidad)",
    )


def _consulta_plan() -> Consulta:
    return Consulta(
        id="plan_free",
        servicio="AWS Free Tier",
        descripcion="Plan de cuenta (FREE/PAID), fecha de alta y expiracion",
        servicio_api="freetier",
        operacion="get-account-plan-state",
        justificacion_costo="API informativa de plan Free Tier, sin tarifa por solicitud publicada",
        fuente_costo="AWS Free Tier API",
    )


def _consulta_uso_free() -> Consulta:
    return Consulta(
        id="free_tier_uso",
        servicio="AWS Free Tier",
        descripcion="Uso del plan Free Tier por servicio",
        servicio_api="freetier",
        operacion="get-free-tier-usage",
        justificacion_costo="API informativa de uso Free Tier, sin tarifa por solicitud publicada",
        fuente_costo="AWS Free Tier API",
    )


def _consulta_presupuestos() -> Consulta:
    return Consulta(
        id="presupuestos",
        servicio="AWS Budgets",
        descripcion="Presupuestos existentes y umbrales (solo lectura)",
        servicio_api="budgets",
        operacion="describe-budgets",
        argumentos=("--account-id", "{account_id}"),
        justificacion_costo="Descripcion de presupuestos existentes sin crear ni modificar recursos",
        fuente_costo=(
            "AWS Budgets: los dos primeros presupuestos no tienen cargo; "
            "crear o modificar queda prohibido en I-000"
        ),
    )


def _consulta_cuota_vcpu() -> Consulta:
    return Consulta(
        id="cuota_ec2_vcpu",
        servicio="AWS Service Quotas",
        descripcion="Cuota de vCPU On-Demand Standard en us-east-2",
        # El nombre del servicio en AWS CLI es ``service-quotas``; ``servicequotas``
        # provoca "Found invalid choice".
        servicio_api="service-quotas",
        operacion="get-service-quota",
        argumentos=("--service-code", "ec2", "--quota-code", "L-1216C47A"),
        justificacion_costo="Consulta de cuota, sin tarifa por solicitud",
        fuente_costo="AWS Service Quotas API",
    )


def _consulta_tipos_instancia() -> Consulta:
    return Consulta(
        id="tipos_instancia",
        servicio="Amazon EC2",
        descripcion="Disponibilidad de t4g.micro por zona en us-east-2",
        servicio_api="ec2",
        operacion="describe-instance-type-offerings",
        argumentos=(
            "--location-type",
            "availability-zone",
            "--filters",
            "Name=instance-type,Values=t4g.micro",
        ),
        justificacion_costo="Consulta descriptiva, sin tarifa",
        fuente_costo="Amazon EC2 API",
    )


def _consulta_redis_versiones() -> Consulta:
    return Consulta(
        id="redis_versiones",
        servicio="Amazon ElastiCache",
        descripcion="Versiones de motor Redis disponibles en us-east-2",
        servicio_api="elasticache",
        operacion="describe-cache-engine-versions",
        argumentos=("--engine", "redis"),
        justificacion_costo="Consulta descriptiva de versiones, sin tarifa",
        fuente_costo="Amazon ElastiCache API",
    )


def _consulta_vpc_origins() -> Consulta:
    return Consulta(
        id="cloudfront_vpc_origins",
        servicio="Amazon CloudFront",
        descripcion="Capacidad de VPC Origins en la cuenta (solo lectura)",
        servicio_api="cloudfront",
        operacion="list-vpc-origins",
        justificacion_costo="Listado de configuracion, sin tarifa por solicitud",
        fuente_costo="Amazon CloudFront API",
    )


def _consulta_ddb_limites() -> Consulta:
    return Consulta(
        id="ddb_limites",
        servicio="Amazon DynamoDB",
        descripcion="Limites de cuenta DynamoDB (tablas, throughput)",
        servicio_api="dynamodb",
        operacion="describe-limits",
        justificacion_costo="Consulta descriptiva de limites, sin tarifa",
        fuente_costo="Amazon DynamoDB API",
    )


def _precio(
    id_consulta: str,
    descripcion: str,
    codigo_servicio: str,
    filtros: tuple[Filtro, ...],
    familia_producto: str | None,
    atributos_esperados: tuple[tuple[str, str], ...],
    unidad_esperada: str,
    region_datos: str = REGION_COMERCIAL_DATOS,
) -> Consulta:
    return Consulta(
        id=id_consulta,
        servicio="AWS Price List",
        descripcion=descripcion,
        servicio_api="pricing",
        operacion="get-products",
        argumentos=("--service-code", codigo_servicio),
        filtros=filtros,
        region_endpoint=REGION_ENDPOINT_PRICING,
        region_datos=region_datos,
        clasificacion="PRICE_LIST_SIN_CARGO",
        costo_estimado_usd="0",
        justificacion_costo=(
            "La Price List Query API no tiene tarifa por solicitud segun documentacion AWS"
        ),
        fuente_costo=(
            "https://docs.aws.amazon.com/awsaccountbilling/latest/aboutv2/using-pelong.html"
        ),
        codigo_servicio=codigo_servicio,
        familia_producto=familia_producto,
        atributos_esperados=atributos_esperados,
        unidad_esperada=unidad_esperada,
    )


def _consultas_precios() -> tuple[Consulta, ...]:
    """Consultas de precio con filtros precisos y unidad esperada por SKU."""

    ohio = _filtro("location", ETIQUETA_REGION_COMERCIAL)
    return (
        _precio(
            "precio_ec2_t4g_micro",
            "Precio on-demand de t4g.micro Linux compartido en us-east-2",
            "AmazonEC2",
            (
                _filtro("instanceType", "t4g.micro"),
                ohio,
                _filtro("operatingSystem", "Linux"),
                _filtro("tenancy", "Shared"),
                _filtro("preInstalledSw", "NA"),
                _filtro("capacitystatus", "Used"),
                _filtro("licenseModel", "No License required"),
            ),
            "Compute Instance",
            (("instanceType", "t4g.micro"), ("tenancy", "Shared"), ("usagetype", "USE2-BoxUsage:t4g.micro")),
            "Hrs",
        ),
        _precio(
            "precio_ebs_gp3",
            "Precio de almacenamiento EBS gp3 en us-east-2",
            "AmazonEC2",
            (_filtro("productFamily", "Storage"), _filtro("volumeApiName", "gp3"), ohio),
            "Storage",
            (("volumeApiName", "gp3"), ("usagetype", "USE2-EBS:VolumeUsage.gp3")),
            "GB-Mo",
        ),
        _precio(
            "precio_ebs_snapshot",
            "Precio de snapshot EBS estandar en us-east-2",
            "AmazonEC2",
            (
                _filtro("usagetype", "USE2-EBS:SnapshotUsage"),
                ohio,
            ),
            "Storage Snapshot",
            (("usagetype", "USE2-EBS:SnapshotUsage"),),
            "GB-Mo",
        ),
        _precio(
            "precio_ddb_pitr",
            "Precio de point-in-time recovery de DynamoDB en us-east-2",
            "AmazonDynamoDB",
            (_filtro("productFamily", "Database Storage"), ohio),
            "Database Storage",
            (("usagetype", "USE2-TimedPITRStorage-ByteHrs"),),
            "GB-Mo",
        ),
        _precio(
            "precio_elasticache_redis",
            "Precio on-demand de cache.t4g.micro motor Redis OSS en us-east-2",
            "AmazonElastiCache",
            (
                _filtro("instanceType", "cache.t4g.micro"),
                _filtro("cacheEngine", "Redis"),
                ohio,
            ),
            "Cache Instance",
            (
                ("instanceType", "cache.t4g.micro"),
                ("cacheEngine", "Redis"),
                # Excluye las variantes de soporte extendido, que son más caras.
                ("usagetype", "USE2-NodeUsage:cache.t4g.micro"),
            ),
            "Hrs",
        ),
        _precio(
            "precio_alb_hora",
            "Precio por hora de Application Load Balancer en us-east-2",
            "AWSELB",
            (
                _filtro("productFamily", "Load Balancer-Application"),
                ohio,
            ),
            "Load Balancer-Application",
            (("usagetype", "USE2-LoadBalancerUsage"),),
            "Hrs",
        ),
        _precio(
            "precio_alb_lcu",
            "Precio por LCU-hora on-demand de Application Load Balancer en us-east-2",
            "AWSELB",
            (
                _filtro("productFamily", "Load Balancer-Application"),
                ohio,
            ),
            "Load Balancer-Application",
            (("usagetype", "USE2-LCUUsage"),),
            "LCU-Hrs",
        ),
        _precio(
            "precio_ddb_storage",
            "Precio de almacenamiento DynamoDB on-demand en us-east-2",
            "AmazonDynamoDB",
            (_filtro("productFamily", "Database Storage"), ohio),
            "Database Storage",
            (("usagetype", "USE2-TimedStorage-ByteHrs"),),
            "GB-Mo",
        ),
        _precio(
            "precio_ddb_rru",
            "Precio por unidad de lectura on-demand en us-east-2",
            "AmazonDynamoDB",
            (_filtro("usagetype", "USE2-ReadRequestUnits"),),
            "Amazon DynamoDB PayPerRequest Throughput",
            (("usagetype", "USE2-ReadRequestUnits"),),
            "ReadRequestUnits",
        ),
        _precio(
            "precio_ddb_wru",
            "Precio por unidad de escritura on-demand en us-east-2",
            "AmazonDynamoDB",
            (_filtro("usagetype", "USE2-WriteRequestUnits"),),
            "Amazon DynamoDB PayPerRequest Throughput",
            (("usagetype", "USE2-WriteRequestUnits"),),
            "WriteRequestUnits",
        ),
        _precio(
            "precio_s3_standard",
            "Precio de S3 Standard en us-east-2",
            "AmazonS3",
            (
                _filtro("productFamily", "Storage"),
                _filtro("storageClass", "General Purpose"),
                _filtro("volumeType", "Standard"),
                ohio,
            ),
            "Storage",
            (("storageClass", "General Purpose"), ("usagetype", "USE2-TimedStorage-ByteHrs")),
            "GB-Mo",
        ),
        _precio(
            "precio_cloudwatch_logs_ingesta",
            "Precio de ingesta de logs en CloudWatch us-east-2",
            "AmazonCloudWatch",
            (
                _filtro("productFamily", "Data Payload"),
                _filtro("usagetype", "USE2-DataProcessing-Bytes"),
                ohio,
            ),
            "Data Payload",
            (("usagetype", "USE2-DataProcessing-Bytes"),),
            "GB",
        ),
        _precio(
            "precio_ipv4_publica",
            "Precio de direccion IPv4 publica en uso en us-east-2",
            "AmazonVPC",
            (_filtro("usagetype", "USE2-PublicIPv4:InUseAddress"), ohio),
            None,
            (("usagetype", "USE2-PublicIPv4:InUseAddress"),),
            "Hrs",
        ),
        _precio(
            "precio_cloudfront_egreso",
            "Precio de transferencia de datos de CloudFront a Estados Unidos",
            "AmazonCloudFront",
            (_filtro("usagetype", "US-DataTransfer-Out-Bytes"),),
            "Data Transfer",
            (("usagetype", "US-DataTransfer-Out-Bytes"),),
            "GB",
            region_datos="global",
        ),
        _precio(
            "precio_ddb_backup",
            "Precio de almacenamiento de backup on-demand de DynamoDB en us-east-2",
            "AmazonDynamoDB",
            (
                _filtro("productFamily", "Amazon DynamoDB On-Demand Backup Storage"),
                ohio,
            ),
            "Amazon DynamoDB On-Demand Backup Storage",
            (("usagetype", "USE2-TimedBackupStorage-ByteHrs"),),
            "GB-Month",
        ),
        _precio(
            "precio_ddb_restore",
            "Precio de restauracion de DynamoDB por GB en us-east-2",
            "AmazonDynamoDB",
            (_filtro("productFamily", "Amazon DynamoDB Restore Data Size"), ohio),
            "Amazon DynamoDB Restore Data Size",
            (("usagetype", "USE2-RestoreDataSize-Bytes"),),
            "GB",
        ),
        _precio(
            "precio_cw_logs_storage",
            "Precio de almacenamiento de logs en CloudWatch us-east-2",
            "AmazonCloudWatch",
            (_filtro("usagetype", "USE2-TimedStorage-ByteHrs"), ohio),
            None,
            (("usagetype", "USE2-TimedStorage-ByteHrs"),),
            "GB-Mo",
        ),
        _precio(
            "precio_cw_metrica",
            "Precio de metrica personalizada de CloudWatch en us-east-2",
            "AmazonCloudWatch",
            (_filtro("usagetype", "USE2-CW:MetricMonitorUsage"), ohio),
            "Metric",
            (("usagetype", "USE2-CW:MetricMonitorUsage"),),
            "Metrics",
        ),
        _precio(
            "precio_cw_alarma",
            "Precio de alarma estandar de CloudWatch en us-east-2",
            "AmazonCloudWatch",
            (_filtro("usagetype", "USE2-CW:AlarmMonitorUsage"), ohio),
            "Alarm",
            (("usagetype", "USE2-CW:AlarmMonitorUsage"),),
            "Alarms",
        ),
        _precio(
            "precio_secret_manager",
            "Precio de secreto almacenado en Secrets Manager us-east-2",
            "AWSSecretsManager",
            (_filtro("usagetype", "USE2-AWSSecretsManager-Secrets"), ohio),
            "Secret",
            (("usagetype", "USE2-AWSSecretsManager-Secrets"),),
            "Secrets",
        ),
        _precio(
            "precio_s3_solicitudes",
            "Precio de solicitudes S3 Tier1 (PUT/COPY/POST/LIST) en us-east-2",
            "AmazonS3",
            (_filtro("usagetype", "USE2-Requests-Tier1"), ohio),
            "API Request",
            (("usagetype", "USE2-Requests-Tier1"),),
            "Requests",
        ),
        _precio(
            "precio_interaz",
            "Precio de transferencia regional (inter-AZ) en us-east-2",
            "AWSDataTransfer",
            (_filtro("usagetype", "USE2-DataTransfer-Regional-Bytes"),),
            "Data Transfer",
            (("usagetype", "USE2-DataTransfer-Regional-Bytes"),),
            "GB",
        ),
    )


def _consulta_costo_explorer() -> Consulta:
    return Consulta(
        id="costo_explorer",
        servicio="AWS Cost Explorer",
        descripcion="Costo y uso observado por servicio y periodo",
        servicio_api="ce",
        operacion="get-cost-and-usage",
        argumentos=(
            "--time-period",
            "Start=2026-09-01,End=2026-11-06",
            "--granularity",
            "MONTHLY",
            "--metrics",
            "UnblendedCost",
        ),
        region_endpoint="us-east-1",
        clasificacion="FACTURABLE_POR_SOLICITUD",
        costo_estimado_usd="0.01",
        justificacion_costo=(
            "Cost Explorer cobra por solicitud de API; no se ejecuta sin aprobacion humana especifica"
        ),
        fuente_costo="https://aws.amazon.com/aws-cost-management/aws-cost-explorer/pricing/",
        requiere_aprobacion_especifica=True,
    )


def _catalogo() -> tuple[Consulta, ...]:
    return (
        _consulta_identidad(),
        _consulta_plan(),
        _consulta_uso_free(),
        _consulta_presupuestos(),
        _consulta_cuota_vcpu(),
        _consulta_tipos_instancia(),
        _consulta_redis_versiones(),
        _consulta_vpc_origins(),
        _consulta_ddb_limites(),
    ) + _consultas_precios() + (_consulta_costo_explorer(),)


CATALOGO: tuple[Consulta, ...] = _catalogo()


def catalogo_por_id() -> dict[str, Consulta]:
    return {c.id: c for c in CATALOGO}


def _problemas_region(consulta: Consulta) -> list[str]:
    problemas: list[str] = []
    comando = consulta.comando("{profile}", REGION_COMERCIAL_DATOS)
    apariciones = comando.count("--region")
    if apariciones != 1:
        problemas.append(f"{consulta.id}: el comando contiene {apariciones} --region (debe ser 1)")
    if "--region" in comando:
        valor = comando[comando.index("--region") + 1]
        esperado = consulta.region_endpoint or REGION_COMERCIAL_DATOS
        if valor != esperado:
            problemas.append(
                f"{consulta.id}: --region={valor} contradice region_endpoint={esperado}"
            )
    return problemas


def _problemas_filtros(consulta: Consulta) -> list[str]:
    """En Price List el comando debe llevar exactamente un ``--filters``."""

    problemas: list[str] = []
    if not consulta.es_pricing:
        # Otras APIs (EC2, ElastiCache) usan su propio esquema de --filters,
        # que se declara en ``argumentos`` y no se rige por la regla anterior.
        return problemas

    comando = consulta.comando("{profile}", REGION_COMERCIAL_DATOS)
    apariciones = comando.count("--filters")
    if consulta.filtros and apariciones != 1:
        problemas.append(
            f"{consulta.id}: usa --filters {apariciones} veces (debe ser exactamente 1)"
        )
    if not consulta.filtros and apariciones:
        problemas.append(f"{consulta.id}: declara --filters sin filtros estructurados")
    if any(arg == "--filters" for arg in consulta.argumentos):
        problemas.append(
            f"{consulta.id}: --filters incrustado en argumentos (rompe el único argumento de filtros)"
        )
    return problemas


def _problemas_pricing(consulta: Consulta) -> list[str]:
    problemas: list[str] = []
    if not consulta.es_pricing:
        if consulta.codigo_servicio or consulta.filtros:
            problemas.append(f"{consulta.id}: filtros estructurados sólo aplican a pricing")
        return problemas

    if consulta.codigo_servicio not in CODIGOS_SERVICIO_VALIDADOS:
        problemas.append(
            f"{consulta.id}: codigo de servicio no validado {consulta.codigo_servicio!r}"
        )
    if consulta.region_endpoint != REGION_ENDPOINT_PRICING:
        problemas.append(
            f"{consulta.id}: la Price List API exige endpoint {REGION_ENDPOINT_PRICING}, "
            f"no {consulta.region_endpoint}"
        )
    if not consulta.filtros:
        problemas.append(f"{consulta.id}: consulta de precio sin filtros")
    if not consulta.atributos_esperados:
        problemas.append(f"{consulta.id}: sin atributos esperados declarados")
    if not consulta.unidad_esperada:
        problemas.append(f"{consulta.id}: sin unidad esperada declarada")
    if not consulta.familia_producto:
        # AWS omite productFamily en algunos productos (p. ej. IPv4 pública);
        # en ese caso el ítem se identifica por usagetype.
        if not any(campo == "usagetype" for campo, _ in consulta.atributos_esperados):
            problemas.append(
                f"{consulta.id}: sin familia de producto y sin usagetype identificador"
            )

    campos = {f.campo for f in consulta.filtros}
    if len(campos) != len(consulta.filtros):
        problemas.append(f"{consulta.id}: filtros duplicados sobre el mismo campo")

    if consulta.region_datos == REGION_COMERCIAL_DATOS:
        anclado_por_uso = any(
            f.campo == "usagetype" and f.valor.startswith("USE2-")
            for f in consulta.filtros
        )
        if ("location", ETIQUETA_REGION_COMERCIAL) not in {
            (f.campo, f.valor) for f in consulta.filtros
        } and not anclado_por_uso:
            problemas.append(
                f"{consulta.id}: precio de us-east-2 sin filtro location={ETIQUETA_REGION_COMERCIAL} "
                "ni usagetype USE2-"
            )
        if any(
            f.campo == "location" and f.valor != ETIQUETA_REGION_COMERCIAL
            for f in consulta.filtros
        ):
            problemas.append(f"{consulta.id}: filtro de location contradictorio")
    return problemas


def validar_catalogo() -> list[str]:
    """Verifica verbos, región, filtros, códigos y clasificación de costo."""

    problemas: list[str] = []
    for consulta in CATALOGO:
        if not consulta.operacion.startswith(VERBOS_LECTURA):
            problemas.append(
                f"{consulta.id}: operacion no es de lectura ({consulta.operacion})"
            )
        for arg in consulta.argumentos:
            if arg in (
                "create",
                "update",
                "delete",
                "put",
                "modify",
                "start",
                "stop",
                "terminate",
            ):
                problemas.append(f"{consulta.id}: argumento mutante {arg}")
            if arg.startswith(VERBOS_PROHIBIDOS):
                problemas.append(f"{consulta.id}: verbo prohibido {arg}")
        if not consulta.clasificacion:
            problemas.append(f"{consulta.id}: sin clasificacion de costo")
        if consulta.requiere_aprobacion_especifica and not consulta.costo_estimado_usd:
            problemas.append(f"{consulta.id}: facturable sin costo estimado declarado")
        problemas.extend(_problemas_region(consulta))
        problemas.extend(_problemas_filtros(consulta))
        problemas.extend(_problemas_pricing(consulta))
    return problemas


def describir(consulta: Consulta) -> dict:
    return {
        "id": consulta.id,
        "servicio": consulta.servicio,
        "descripcion": consulta.descripcion,
        "servicio_api": consulta.servicio_api,
        "operacion": consulta.operacion,
        "region_endpoint": consulta.region_endpoint or REGION_COMERCIAL_DATOS,
        "region_datos": consulta.region_datos,
        "codigo_servicio": consulta.codigo_servicio,
        "familia_producto": consulta.familia_producto,
        "unidad_esperada": consulta.unidad_esperada,
        "filtros": [{"campo": f.campo, "valor": f.valor} for f in consulta.filtros],
        "clasificacion": consulta.clasificacion,
        "costo_estimado_usd": consulta.costo_estimado_usd,
        "requiere_aprobacion_especifica": consulta.requiere_aprobacion_especifica,
        "justificacion_costo": consulta.justificacion_costo,
        "fuente_costo": consulta.fuente_costo,
        "comando": " ".join(consulta.comando("{profile}", REGION_COMERCIAL_DATOS)),
    }


RAIZ_REPO = Path(__file__).resolve().parents[2]
EVIDENCIA = Path(__file__).with_name("evidencia")
def directorio_crudo_por_defecto() -> Path:
    """Directorio temporal nativo (nunca ``$env:TEMP`` literal ni el repo).

    En Windows, ``tempfile.gettempdir()`` resuelve ``%TEMP%`` sin depender de la
    sintaxis de PowerShell, que producía ``NotADirectoryError: WinError 267``.
    """

    base = os.environ.get("TEMP") or os.environ.get("TMP") or tempfile.gettempdir()
    candidato = Path(base) / "nexum_finops_crudo"
    try:
        candidato.mkdir(parents=True, exist_ok=True)
        return candidato
    except OSError:
        return Path(tempfile.gettempdir()) / "nexum_finops_crudo"


CRUDO_POR_DEFECTO = directorio_crudo_por_defecto()


def ejecutar(
    consulta: Consulta,
    profile: str,
    region: str,
    dir_crudo: Path,
    dir_evidencia: Path,
    account_id: str | None = None,
    aprobar_facturable: bool = False,
    timeout: int = 120,
) -> dict:
    """Ejecuta una consulta de sólo lectura y persiste evidencia sanitizada."""

    if consulta.requiere_aprobacion_especifica and not aprobar_facturable:
        return {
            "id": consulta.id,
            "estado": "NOT_RUN",
            "motivo": "consulta potencialmente facturable sin aprobacion humana especifica",
            "costo_estimado_usd": consulta.costo_estimado_usd,
        }

    comando = consulta.comando(profile, region, account_id)
    if "{account_id}" in comando:
        return {
            "id": consulta.id,
            "estado": "NOT_RUN",
            "motivo": "requiere account id no proporcionado",
        }

    try:
        proceso = subprocess.run(
            comando, capture_output=True, text=True, timeout=timeout, check=False
        )
    except FileNotFoundError:
        return {"id": consulta.id, "estado": "NOT_RUN", "motivo": "AWS CLI no disponible"}

    dir_crudo.mkdir(parents=True, exist_ok=True)
    dir_evidencia.mkdir(parents=True, exist_ok=True)
    crudo = dir_crudo / f"{consulta.id}.raw.txt"
    crudo.write_text(proceso.stdout or "", encoding="utf-8")

    salida = proceso.stdout or ""
    saneada = sanitizar.sanitizar_json(salida) if salida.strip() else ""
    destino = dir_evidencia / f"{consulta.id}.json"
    if saneada:
        destino.write_text(saneada + "\n", encoding="utf-8")

    error = (proceso.stderr or "").strip()
    if proceso.returncode != 0:
        mensaje = sanitizar.sanitizar({"detalle": error[:800]})
        estado = "FAIL"
        motivo = "la consulta devolvio error"
        texto = error.lower()
        if "expired" in texto or "session" in texto:
            estado = "NOT_VERIFIED"
            motivo = "sesion AWS invalida o expirada; reautenticar y repetir"
        elif "accessdenied" in texto or "not authorized" in texto:
            estado = "BLOCKED"
            motivo = "AccessDenied: la politica del perfil impide esta lectura"
        return {
            "id": consulta.id,
            "estado": estado,
            "motivo": motivo,
            "detalle_sanitizado": mensaje,
            "evidencia": destino.relative_to(RAIZ_REPO).as_posix() if destino.exists() else None,
        }

    return {
        "id": consulta.id,
        "estado": "PASS",
        "motivo": "consulta de solo lectura ejecutada",
        "evidencia": destino.relative_to(RAIZ_REPO).as_posix() if destino.exists() else None,
    }


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Consultas AWS de solo lectura I-000")
    parser.add_argument(
        "--listar", action="store_true", help="lista el catalogo y clasificacion de costo"
    )
    parser.add_argument(
        "--validar", action="store_true", help="valida que solo haya operaciones de lectura"
    )
    parser.add_argument("--ejecutar", metavar="ID", help="ejecuta una consulta del catalogo")
    parser.add_argument("--profile", default=os.environ.get("AWS_PROFILE", "nexum"))
    parser.add_argument("--region", default=REGION_COMERCIAL_DATOS)
    parser.add_argument("--account-id", default=None, help="cuenta AWS (no se persiste)")
    parser.add_argument(
        "--aprobar-facturable", action="store_true", help="aprueba costo por solicitud"
    )
    parser.add_argument("--dir-crudo", type=Path, default=CRUDO_POR_DEFECTO)
    parser.add_argument("--dir-evidencia", type=Path, default=EVIDENCIA)
    args = parser.parse_args(argv)

    if args.validar:
        problemas = validar_catalogo()
        if problemas:
            print("CATALOGO_INVALIDO")
            for problema in problemas:
                print(f"- {problema}")
            return 2
        print(
            f"CATALOGO_LECTURA_OK: {len(CATALOGO)} consultas, "
            "ninguna mutante, region y filtros validados"
        )
        return 0

    if args.listar:
        print(json.dumps([describir(c) for c in CATALOGO], indent=2, ensure_ascii=False))
        return 0

    if args.ejecutar:
        consulta = catalogo_por_id().get(args.ejecutar)
        if consulta is None:
            print(f"CONSULTA_DESCONOCIDA: {args.ejecutar}")
            return 2
        resultado = ejecutar(
            consulta,
            args.profile,
            args.region,
            args.dir_crudo,
            args.dir_evidencia,
            args.account_id,
            args.aprobar_facturable,
        )
        print(json.dumps(sanitizar.sanitizar(resultado), indent=2, ensure_ascii=False, sort_keys=True))
        return 0 if resultado["estado"] == "PASS" else 1

    parser.print_help()
    return 2


if __name__ == "__main__":
    sys.exit(main())
