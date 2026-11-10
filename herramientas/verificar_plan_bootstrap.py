"""Verificador LOCAL del plan de bootstrap de Terraform (I-070).

Comprueba que un `terraform show -json` corresponde exactamente al inventario
revisado y que el fichero `.tfplan` que se va a aplicar tiene el SHA-256
esperado. No consulta AWS, no ejecuta Terraform y no aplica nada.

Motivo: el 2026-11-10 se detecto un `bootstrap.tfplan` con
``applyable=false``, ``errored=true`` y **1 de 8** recursos porque se genero con
un perfil AWS inexistente, sin bloque ``provider`` y sin ``sufijo_cuenta``.
Aplicar ese artefacto habria fallado o, peor, habria creado infraestructura
parcial. Este control falla cerrado.

Codigos: 0 PASS, 1 FAIL, 2 uso invalido.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import sys
from pathlib import Path
from typing import Any

PROVEEDOR_AWS = "registry.terraform.io/hashicorp/aws"
PROVEEDOR_LOCAL = "terraform.io/builtin/terraform"

DIRECCIONES_ESPERADAS = frozenset(
    {
        "aws_s3_bucket.state",
        "aws_s3_bucket_versioning.state",
        "aws_s3_bucket_server_side_encryption_configuration.state",
        "aws_s3_bucket_public_access_block.state",
        "aws_s3_bucket_ownership_controls.state",
        "aws_s3_bucket_policy.state",
        "aws_s3_bucket_lifecycle_configuration.state",
        "terraform_data.validacion_bootstrap",
    }
)

REGION_ESPERADA = "us-east-2"
PATRON_BUCKET = re.compile(r"^nexum-h08i-tfstate-[a-z0-9][a-z0-9-]{0,40}$")


def sha256_archivo(ruta: Path) -> str:
    digest = hashlib.sha256()
    with ruta.open("rb") as manejador:
        for bloque in iter(lambda: manejador.read(1024 * 1024), b""):
            digest.update(bloque)
    return digest.hexdigest().upper()


def evaluar(plan: Any) -> list[str]:
    """Devuelve la lista de problemas; vacia significa plan aceptable."""
    problemas: list[str] = []
    if not isinstance(plan, dict):
        return ["PLAN_MALFORMADO no_es_objeto"]

    if plan.get("errored") is not False:
        problemas.append("PLAN_ERROREADO")
    if plan.get("complete") is not True:
        problemas.append("PLAN_INCOMPLETO")
    if plan.get("applyable") is not True:
        problemas.append("PLAN_NO_APLICABLE")

    cambios = plan.get("resource_changes")
    if not isinstance(cambios, list):
        problemas.append("PLAN_SIN_RESOURCE_CHANGES")
        return problemas

    direcciones: set[str] = set()
    for cambio in cambios:
        if not isinstance(cambio, dict):
            problemas.append("CAMBIO_MALFORMADO")
            continue
        direccion = cambio.get("address")
        tipo = cambio.get("type")
        proveedor = cambio.get("provider_name")
        accion = cambio.get("change", {}).get("actions") if isinstance(cambio.get("change"), dict) else None
        if not isinstance(direccion, str) or not isinstance(tipo, str):
            problemas.append("CAMBIO_MALFORMADO")
            continue
        if direccion in direcciones:
            problemas.append("DIRECCION_DUPLICADA " + direccion)
        direcciones.add(direccion)

        esperado = PROVEEDOR_AWS if tipo.startswith("aws_") else PROVEEDOR_LOCAL
        if proveedor != esperado:
            problemas.append("PROVEEDOR_INESPERADO " + direccion)

        if not isinstance(accion, list) or not accion:
            problemas.append("ACCION_AUSENTE " + direccion)
            continue
        if "delete" in accion:
            problemas.append("DESTRUCCION " + direccion)
        if "update" in accion:
            problemas.append("ACTUALIZACION " + direccion)
        if sorted(accion) != ["create"]:
            problemas.append("ACCION_NO_ES_CREATE " + direccion)

        if cambio.get("change", {}).get("importing"):
            problemas.append("IMPORTACION " + direccion)

    faltantes = DIRECCIONES_ESPERADAS - direcciones
    for direccion in sorted(faltantes):
        problemas.append("RECURSO_FALTANTE " + direccion)
    sobrantes = direcciones - DIRECCIONES_ESPERADAS
    for direccion in sorted(sobrantes):
        problemas.append("RECURSO_NO_PREVISTO " + direccion)

    variables = plan.get("variables")
    if not isinstance(variables, dict):
        problemas.append("VARIABLES_AUSENTES")
    else:
        sufijo = variables.get("sufijo_cuenta", {})
        sufijo = sufijo.get("value") if isinstance(sufijo, dict) else None
        if not isinstance(sufijo, str) or not sufijo.strip():
            problemas.append("SUFIJO_CUENTA_VACIO")
        region = variables.get("region", {})
        region = region.get("value") if isinstance(region, dict) else None
        if region != REGION_ESPERADA:
            problemas.append("REGION_INESPERADA " + str(region))

    valores = (plan.get("planned_values") or {}).get("root_module", {}).get("resources") or []
    bucket = None
    for recurso in valores:
        if isinstance(recurso, dict) and recurso.get("address") == "aws_s3_bucket.state":
            bucket = (recurso.get("values") or {}).get("bucket")
    if not isinstance(bucket, str) or not PATRON_BUCKET.match(bucket):
        problemas.append("BUCKET_INESPERADO " + str(bucket))

    return problemas


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Verifica el plan de bootstrap de I-070.")
    parser.add_argument("--plan-json", required=True, help="salida de `terraform show -json`")
    parser.add_argument("--tfplan", required=True, help="fichero .tfplan a aplicar")
    parser.add_argument("--sha-esperado", required=True, help="SHA-256 hex revisado")
    args = parser.parse_args(argv)

    plan_json = Path(args.plan_json)
    tfplan = Path(args.tfplan)
    if not plan_json.is_file() or not tfplan.is_file():
        print(json.dumps({"resultado": "USO_INVALIDO", "motivo": "archivo inexistente"}))
        return 2

    try:
        plan = json.loads(plan_json.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        print(json.dumps({"resultado": "USO_INVALIDO", "motivo": str(exc)}))
        return 2

    esperado = args.sha_esperado.strip().upper()
    if not re.fullmatch(r"[0-9A-F]{64}", esperado):
        print(json.dumps({"resultado": "USO_INVALIDO", "motivo": "sha esperado no es 64 hex"}))
        return 2

    sha = sha256_archivo(tfplan)
    problemas = evaluar(plan)
    if sha != esperado:
        problemas.append("SHA_NO_COINCIDE esperado=" + esperado + " obtenido=" + sha)

    resultado = "PASS" if not problemas else "FAIL"
    print(
        json.dumps(
            {
                "resultado": resultado,
                "problemas": problemas,
                "sha256": sha,
                "recursos": len(plan.get("resource_changes") or []),
            },
            ensure_ascii=False,
        )
    )
    return 0 if resultado == "PASS" else 1


if __name__ == "__main__":
    sys.exit(main())
