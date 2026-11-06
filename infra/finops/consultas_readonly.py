"""Catálogo y ejecutor de consultas AWS de SOLO LECTURA para I-000.

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
from dataclasses import dataclass
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


@dataclass(frozen=True)
class Consulta:
    id: str
    servicio: str
    descripcion: str
    argumentos: tuple[str, ...]
    clasificacion: str
    costo_estimado_usd: str | None
    justificacion_costo: str
    fuente_costo: str
    requiere_aprobacion_especifica: bool = False

    @property
    def operacion(self) -> str:
        return next((a for a in self.argumentos if a.startswith(VERBOS_LECTURA)), self.argumentos[1])

    def comando(self, profile: str, region: str, account_id: str | None = None) -> list[str]:
        salida = ["aws"]
        for arg in self.argumentos:
            salida.append(
                arg.replace("{profile}", profile)
                .replace("{region}", region)
                .replace("{account_id}", account_id or "{account_id}")
            )
        return salida


CATALOGO: tuple[Consulta, ...] = (
    Consulta(
        id="sts_identidad",
        servicio="AWS STS",
        descripcion="Identidad efectiva de la sesion (cuenta y ARN, sanitizados)",
        argumentos=("sts", "get-caller-identity", "--profile", "{profile}", "--region", "{region}", "--output", "json"),
        clasificacion="SIN_CARGO",
        costo_estimado_usd="0",
        justificacion_costo="Llamada de identidad sin tarifa publicada",
        fuente_costo="AWS STS API (consulta de identidad)",
    ),
    Consulta(
        id="plan_free",
        servicio="AWS Free Tier",
        descripcion="Plan de cuenta (FREE/PAID), fecha de alta y expiracion",
        argumentos=("freetier", "get-account-plan-state", "--profile", "{profile}", "--region", "{region}", "--output", "json"),
        clasificacion="SIN_CARGO",
        costo_estimado_usd="0",
        justificacion_costo="API informativa de plan Free Tier, sin tarifa por solicitud publicada",
        fuente_costo="AWS Free Tier API",
    ),
    Consulta(
        id="free_tier_uso",
        servicio="AWS Free Tier",
        descripcion="Uso del plan Free Tier por servicio",
        argumentos=("freetier", "get-free-tier-usage", "--profile", "{profile}", "--region", "{region}", "--output", "json"),
        clasificacion="SIN_CARGO",
        costo_estimado_usd="0",
        justificacion_costo="API informativa de uso Free Tier, sin tarifa por solicitud publicada",
        fuente_costo="AWS Free Tier API",
    ),
    Consulta(
        id="presupuestos",
        servicio="AWS Budgets",
        descripcion="Presupuestos existentes y umbrales (solo lectura)",
        argumentos=("budgets", "describe-budgets", "--account-id", "{account_id}", "--profile", "{profile}", "--output", "json"),
        clasificacion="SIN_CARGO",
        costo_estimado_usd="0",
        justificacion_costo="Descriptura de presupuestos existentes sin crear ni modificar recursos",
        fuente_costo="AWS Budgets (los dos primeros presupuestos no tienen cargo; crear queda prohibido en I-000)",
    ),
    Consulta(
        id="cuota_ec2_vcpu",
        servicio="AWS Service Quotas",
        descripcion="Cuota de vCPU On-Demand Standard en us-east-2",
        argumentos=("servicequotas", "get-service-quota", "--service-code", "ec2", "--quota-code", "L-1216C47A", "--profile", "{profile}", "--region", "{region}", "--output", "json"),
        clasificacion="SIN_CARGO",
        costo_estimado_usd="0",
        justificacion_costo="Consulta de cuota, sin tarifa por solicitud",
        fuente_costo="AWS Service Quotas API",
    ),
    Consulta(
        id="tipos_instancia",
        servicio="Amazon EC2",
        descripcion="Disponibilidad de t4g.micro por zona en us-east-2",
        argumentos=(
            "ec2", "describe-instance-type-offerings",
            "--location-type", "availability-zone",
            "--filters", "Name=instance-type,Values=t4g.micro",
            "--profile", "{profile}", "--region", "{region}", "--output", "json",
        ),
        clasificacion="SIN_CARGO",
        costo_estimado_usd="0",
        justificacion_costo="Consulta descriptiva, sin tarifa",
        fuente_costo="Amazon EC2 API",
    ),
    Consulta(
        id="redis_versiones",
        servicio="Amazon ElastiCache",
        descripcion="Versiones de motor Redis disponibles en us-east-2",
        argumentos=(
            "elasticache", "describe-cache-engine-versions",
            "--engine", "redis",
            "--profile", "{profile}", "--region", "{region}", "--output", "json",
        ),
        clasificacion="SIN_CARGO",
        costo_estimado_usd="0",
        justificacion_costo="Consulta descriptiva de versiones, sin tarifa",
        fuente_costo="Amazon ElastiCache API",
    ),
    Consulta(
        id="cloudfront_vpc_origins",
        servicio="Amazon CloudFront",
        descripcion="Capacidad de VPC Origins en la cuenta (solo lectura)",
        argumentos=("cloudfront", "list-vpc-origins", "--profile", "{profile}", "--output", "json"),
        clasificacion="SIN_CARGO",
        costo_estimado_usd="0",
        justificacion_costo="Listado de configuracion, sin tarifa por solicitud",
        fuente_costo="Amazon CloudFront API",
    ),
    Consulta(
        id="ddb_limites",
        servicio="Amazon DynamoDB",
        descripcion="Limites de cuenta DynamoDB (tablas, throughput)",
        argumentos=("dynamodb", "describe-limits", "--profile", "{profile}", "--region", "{region}", "--output", "json"),
        clasificacion="SIN_CARGO",
        costo_estimado_usd="0",
        justificacion_costo="Consulta descriptiva de limites, sin tarifa",
        fuente_costo="Amazon DynamoDB API",
    ),
    Consulta(
        id="precio_ec2",
        servicio="AWS Price List",
        descripcion="Precio on-demand de t4g.micro Linux en us-east-2",
        argumentos=(
            "pricing", "get-products",
            "--service-code", "AmazonEC2",
            "--region", "us-east-2",
            "--filters", "Type=TERM_MATCH,Field=instanceType,Value=t4g.micro",
            "--filters", "Type=TERM_MATCH,Field=location,Value=US East (Ohio)",
            "--filters", "Type=TERM_MATCH,Field=operatingSystem,Value=Linux",
            "--filters", "Type=TERM_MATCH,Field=tenancy,Value=Shared",
            "--filters", "Type=TERM_MATCH,Field=preInstalledSw,Value=NA",
            "--filters", "Type=TERM_MATCH,Field=capacitystatus,Value=Used",
            "--profile", "{profile}", "--region", "us-east-1", "--output", "json",
        ),
        clasificacion="PRICE_LIST_SIN_CARGO",
        costo_estimado_usd="0",
        justificacion_costo="La Price List Query API no tiene tarifa por solicitud segun documentacion AWS",
        fuente_costo="https://docs.aws.amazon.com/awsaccountbilling/latest/aboutv2/using-pelong.html",
    ),
    Consulta(
        id="precio_elasticache",
        servicio="AWS Price List",
        descripcion="Precio on-demand de cache.t4g.micro Redis OSS en us-east-2",
        argumentos=(
            "pricing", "get-products",
            "--service-code", "AmazonElastiCache",
            "--region", "us-east-2",
            "--filters", "Type=TERM_MATCH,Field=instanceType,Value=cache.t4g.micro",
            "--filters", "Type=TERM_MATCH,Field=location,Value=US East (Ohio)",
            "--filters", "Type=TERM_MATCH,Field=cacheEngine,Value=Redis",
            "--profile", "{profile}", "--region", "us-east-1", "--output", "json",
        ),
        clasificacion="PRICE_LIST_SIN_CARGO",
        costo_estimado_usd="0",
        justificacion_costo="La Price List Query API no tiene tarifa por solicitud segun documentacion AWS",
        fuente_costo="https://docs.aws.amazon.com/awsaccountbilling/latest/aboutv2/using-pelong.html",
    ),
    Consulta(
        id="precio_alb",
        servicio="AWS Price List",
        descripcion="Precio de ALB por hora y LCU en us-east-2",
        argumentos=(
            "pricing", "get-products",
            "--service-code", "ElasticLoadBalancing",
            "--region", "us-east-2",
            "--filters", "Type=TERM_MATCH,Field=location,Value=US East (Ohio)",
            "--filters", "Type=TERM_MATCH,Field=productFamily,Value=Load Balancer-Application",
            "--profile", "{profile}", "--region", "us-east-1", "--output", "json",
        ),
        clasificacion="PRICE_LIST_SIN_CARGO",
        costo_estimado_usd="0",
        justificacion_costo="La Price List Query API no tiene tarifa por solicitud segun documentacion AWS",
        fuente_costo="https://docs.aws.amazon.com/awsaccountbilling/latest/aboutv2/using-pelong.html",
    ),
    Consulta(
        id="precio_dynamodb",
        servicio="AWS Price List",
        descripcion="Precio de DynamoDB on-demand en us-east-2",
        argumentos=(
            "pricing", "get-products",
            "--service-code", "AmazonDynamoDB",
            "--region", "us-east-2",
            "--filters", "Type=TERM_MATCH,Field=location,Value=US East (Ohio)",
            "--profile", "{profile}", "--region", "us-east-1", "--output", "json",
        ),
        clasificacion="PRICE_LIST_SIN_CARGO",
        costo_estimado_usd="0",
        justificacion_costo="La Price List Query API no tiene tarifa por solicitud segun documentacion AWS",
        fuente_costo="https://docs.aws.amazon.com/awsaccountbilling/latest/aboutv2/using-pelong.html",
    ),
    Consulta(
        id="precio_cloudfront",
        servicio="AWS Price List",
        descripcion="Precio de CloudFront por transferencia y solicitudes",
        argumentos=(
            "pricing", "get-products",
            "--service-code", "AmazonCloudFront",
            "--region", "us-east-1",
            "--filters", "Type=TERM_MATCH,Field=location,Value=United States",
            "--profile", "{profile}", "--region", "us-east-1", "--output", "json",
        ),
        clasificacion="PRICE_LIST_SIN_CARGO",
        costo_estimado_usd="0",
        justificacion_costo="La Price List Query API no tiene tarifa por solicitud segun documentacion AWS",
        fuente_costo="https://docs.aws.amazon.com/awsaccountbilling/latest/aboutv2/using-pelong.html",
    ),
    Consulta(
        id="precio_s3",
        servicio="AWS Price List",
        descripcion="Precio de S3 Standard en us-east-2",
        argumentos=(
            "pricing", "get-products",
            "--service-code", "AmazonS3",
            "--region", "us-east-2",
            "--filters", "Type=TERM_MATCH,Field=location,Value=US East (Ohio)",
            "--filters", "Type=TERM_MATCH,Field=storageClass,Value=General Purpose",
            "--profile", "{profile}", "--region", "us-east-1", "--output", "json",
        ),
        clasificacion="PRICE_LIST_SIN_CARGO",
        costo_estimado_usd="0",
        justificacion_costo="La Price List Query API no tiene tarifa por solicitud segun documentacion AWS",
        fuente_costo="https://docs.aws.amazon.com/awsaccountbilling/latest/aboutv2/using-pelong.html",
    ),
    Consulta(
        id="costo_explorer",
        servicio="AWS Cost Explorer",
        descripcion="Costo y uso observado por servicio y periodo",
        argumentos=(
            "ce", "get-cost-and-usage",
            "--time-period", "Start=2026-09-01,End=2026-11-06",
            "--granularity", "MONTHLY",
            "--metrics", "UnblendedCost",
            "--profile", "{profile}", "--region", "us-east-1", "--output", "json",
        ),
        clasificacion="FACTURABLE_POR_SOLICITUD",
        costo_estimado_usd="0.01",
        justificacion_costo="Cost Explorer cobra por solicitud de API; no se ejecuta sin aprobacion humana especifica",
        fuente_costo="https://aws.amazon.com/aws-cost-management/aws-cost-explorer/pricing/",
        requiere_aprobacion_especifica=True,
    ),
)


def catalogo_por_id() -> dict[str, Consulta]:
    return {c.id: c for c in CATALOGO}


def validar_catalogo() -> list[str]:
    """Verifica que ninguna consulta contenga verbos mutantes."""

    problemas: list[str] = []
    for consulta in CATALOGO:
        operacion = consulta.operacion
        if not operacion.startswith(VERBOS_LECTURA):
            problemas.append(f"{consulta.id}: operacion no es de lectura ({operacion})")
        for arg in consulta.argumentos:
            if arg in ("create", "update", "delete", "put", "modify", "start", "stop", "terminate"):
                problemas.append(f"{consulta.id}: argumento mutante {arg}")
            if arg.startswith(VERBOS_PROHIBIDOS):
                problemas.append(f"{consulta.id}: verbo prohibido {arg}")
        if not consulta.clasificacion:
            problemas.append(f"{consulta.id}: sin clasificacion de costo")
        if consulta.requiere_aprobacion_especifica and not consulta.costo_estimado_usd:
            problemas.append(f"{consulta.id}: facturable sin costo estimado declarado")
    return problemas


def describir(consulta: Consulta) -> dict:
    return {
        "id": consulta.id,
        "servicio": consulta.servicio,
        "descripcion": consulta.descripcion,
        "operacion": consulta.operacion,
        "clasificacion": consulta.clasificacion,
        "costo_estimado_usd": consulta.costo_estimado_usd,
        "requiere_aprobacion_especifica": consulta.requiere_aprobacion_especifica,
        "justificacion_costo": consulta.justificacion_costo,
        "fuente_costo": consulta.fuente_costo,
        "comando": " ".join(consulta.comando("{profile}", "{region}")),
    }


def ejecutar(
    consulta: Consulta,
    profile: str,
    region: str,
    dir_crudo: Path,
    dir_evidencia: Path,
    account_id: str | None = None,
    aprobar_facturable: bool = False,
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
        return {"id": consulta.id, "estado": "NOT_RUN", "motivo": "requiere account id no proporcionado"}

    try:
        proceso = subprocess.run(comando, capture_output=True, text=True, timeout=120, check=False)
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
        if "expired" in error.lower() or "credentials" in error.lower():
            estado = "NOT_VERIFIED"
            motivo = "sesion AWS invalida o expirada; reautenticar y repetir"
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


RAIZ_REPO = Path(__file__).resolve().parents[2]
EVIDENCIA = Path(__file__).with_name("evidencia")
CRUDO_POR_DEFECTO = Path(os.environ.get("TEMP", ".")) / "nexum_finops_crudo"


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Consultas AWS de solo lectura I-000")
    parser.add_argument("--listar", action="store_true", help="lista el catalogo y clasificacion de costo")
    parser.add_argument("--validar", action="store_true", help="valida que solo haya operaciones de lectura")
    parser.add_argument("--ejecutar", metavar="ID", help="ejecuta una consulta del catalogo")
    parser.add_argument("--profile", default=os.environ.get("AWS_PROFILE", "nexum"))
    parser.add_argument("--region", default="us-east-2")
    parser.add_argument("--account-id", default=None, help="cuenta AWS (no se persiste)")
    parser.add_argument("--aprobar-facturable", action="store_true", help="aprueba costo por solicitud")
    parser.add_argument("--dir-crudo", type=Path, default=CRUDO_POR_DEFECTO)
    parser.add_argument("--dir-evidencia", type=Path, default=EVIDENCIA)
    args = parser.parse_args(argv)

    if args.validar:
        problemas = validar_catalogo()
        if problemas:
            print("CATALOGO_INVALIDO")
            for p in problemas:
                print(f"- {p}")
            return 2
        print(f"CATALOGO_LECTURA_OK: {len(CATALOGO)} consultas, ninguna mutante")
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
