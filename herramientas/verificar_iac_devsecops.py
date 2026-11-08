#!/usr/bin/env python3
"""Escaneo estático DevSecOps de IaC (I-100, INF-100-05).

Complementa a Trivy (escáner principal) con comprobaciones explícitas exigidas por
el SDD. No ejecuta Terraform ni consulta AWS.

Reglas:
- SG_PUBLICO: ingreso desde 0.0.0.0/0 o ::/0 en `cidr_blocks`, `ipv6_cidr_blocks`,
  `cidr_ipv4` o `cidr_ipv6`.
- S3_SIN_CIFRADO: cada bucket `aws_s3_bucket` debe tener una configuración de cifrado
  que lo referencie directamente.
- S3_CIFRADO_NO_VERIFICABLE: existe cifrado pero no se puede asociar al bucket
  (expresión dinámica); se trata como hallazgo, no como PASS.
- IAM_WILDCARD: comodín `*` en Action/Resource escalar o dentro de listas.
- FORCE_DESTROY, ACL_PUBLICA.
- TFSTATE_RASTREADO / GIT_CHECK_FAILED: si Git no puede verificar, falla cerrado.

Límite declarado: el análisis es textual. No resuelve variables, módulos externos
ni expresiones dinámicas; cuando no puede asociar una configuración, reporta
`*_NO_VERIFICABLE` en lugar de asumir que es segura.
"""

from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
from pathlib import Path

EXTENSIONES = (".tf", ".tftpl", ".json")
PATRON_BUCKET = re.compile(r'resource\s+"aws_s3_bucket"\s+"([^"]+)"')
PATRON_CIFRADO = re.compile(r'resource\s+"aws_s3_bucket_server_side_encryption_configuration"\s+"([^"]+)"')
PATRON_BUCKET_REF = re.compile(r'bucket\s*=\s*aws_s3_bucket\.([A-Za-z0-9_-]+)')
PATRON_SSE = re.compile(r"sse_algorithm\s*=\s*\"([^\"]+)\"")
PATRON_SG = re.compile(
    r"(cidr_blocks|ipv6_cidr_blocks|cidr_ipv4|cidr_ipv6)\s*=\s*(\[[^\]]*\]|\"[^\"]*\")",
    re.S,
)
PATRON_IAM_LISTA = re.compile(r"[\"']?(Action|Resource|actions|resources)[\"']?\s*[:=]\s*(\[[^\]]*\])", re.S)
PATRON_IAM_ESCALAR = re.compile(r"[\"']?(Action|Resource|actions|resources)[\"']?\s*[:=]\s*[\"']\*[\"']")
PATRON_FORCE = re.compile(r"force_destroy\s*=\s*true")
PATRON_ACL = re.compile(r"acl\s*=\s*\"(public-read|public-read-write|authenticated-read)\"")
PATRON_INGRESO = re.compile(r'type\s*=\s*"ingress"|aws_vpc_security_group_ingress_rule')


def _archivos(raiz: Path) -> list[Path]:
    return sorted(p for p in raiz.rglob("*") if p.is_file() and p.suffix in EXTENSIONES)


def _linea(texto: str, posicion: int) -> int:
    return texto[:posicion].count("\n") + 1


def _state_rastreado(raiz: Path) -> tuple[list[str], str | None]:
    try:
        salida = subprocess.run(
            ["git", "ls-files", "*.tfstate", "*.tfstate.*"],
            cwd=raiz, capture_output=True, text=True, timeout=30, check=False,
        )
    except (OSError, subprocess.SubprocessError) as exc:
        return [], f"{type(exc).__name__}: {exc}"
    if salida.returncode != 0:
        return [], (salida.stderr.strip() or f"git exit {salida.returncode}")
    return [linea for linea in salida.stdout.splitlines() if linea.strip()], None


def analizar(raiz: Path, comprobar_git: bool = True) -> tuple[list[dict], str | None]:
    hallazgos: list[dict] = []
    archivos = _archivos(raiz)
    textos: dict[Path, str] = {}
    buckets: list[tuple[str, Path, int]] = []
    cifrados: dict[str, set[str]] = {}
    cifrado_dinamico = False

    for archivo in archivos:
        texto = archivo.read_text(encoding="utf-8", errors="replace")
        textos[archivo] = texto
        for coincidencia in PATRON_BUCKET.finditer(texto):
            buckets.append((coincidencia.group(1), archivo, _linea(texto, coincidencia.start())))
        for bloque in re.finditer(
            r'resource\s+"aws_s3_bucket_server_side_encryption_configuration"\s+"[^"]+"\s*\{(.*?)\n\}',
            texto, re.S,
        ):
            cuerpo = bloque.group(1)
            referencias = {m.group(1) for m in PATRON_BUCKET_REF.finditer(cuerpo)}
            if referencias:
                for referencia in referencias:
                    cifrados.setdefault(referencia, set()).add("directa")
            else:
                cifrado_dinamico = True

    for archivo, texto in textos.items():
        for coincidencia in PATRON_SG.finditer(texto):
            valor = coincidencia.group(2)
            if "0.0.0.0/0" in valor or "::/0" in valor:
                if PATRON_INGRESO.search(texto):
                    hallazgos.append({
                        "regla": "SG_PUBLICO", "severidad": "HIGH",
                        "archivo": archivo.relative_to(raiz).as_posix(),
                        "linea": _linea(texto, coincidencia.start()),
                    })
        for coincidencia in PATRON_IAM_LISTA.finditer(texto):
            if re.search(r"[\"']\*[\"']", coincidencia.group(2)):
                hallazgos.append({
                    "regla": "IAM_WILDCARD", "severidad": "HIGH",
                    "archivo": archivo.relative_to(raiz).as_posix(),
                    "linea": _linea(texto, coincidencia.start()),
                })
        for coincidencia in PATRON_IAM_ESCALAR.finditer(texto):
            hallazgos.append({
                "regla": "IAM_WILDCARD", "severidad": "HIGH",
                "archivo": archivo.relative_to(raiz).as_posix(),
                "linea": _linea(texto, coincidencia.start()),
            })
        for coincidencia in PATRON_FORCE.finditer(texto):
            hallazgos.append({
                "regla": "FORCE_DESTROY", "severidad": "HIGH",
                "archivo": archivo.relative_to(raiz).as_posix(),
                "linea": _linea(texto, coincidencia.start()),
            })
        for coincidencia in PATRON_ACL.finditer(texto):
            hallazgos.append({
                "regla": "ACL_PUBLICA", "severidad": "HIGH",
                "archivo": archivo.relative_to(raiz).as_posix(),
                "linea": _linea(texto, coincidencia.start()),
            })

    for nombre, archivo, linea in buckets:
        if nombre in cifrados:
            continue
        regla = "S3_CIFRADO_NO_VERIFICABLE" if cifrado_dinamico else "S3_SIN_CIFRADO"
        hallazgos.append({
            "regla": regla, "severidad": "HIGH",
            "archivo": archivo.relative_to(raiz).as_posix(), "linea": linea,
        })

    error_git = None
    if comprobar_git:
        rastreados, error_git = _state_rastreado(raiz)
        for ruta in rastreados:
            hallazgos.append({"regla": "TFSTATE_RASTREADO", "severidad": "CRITICAL", "archivo": ruta, "linea": 1})
        if error_git is not None:
            hallazgos.append({"regla": "GIT_CHECK_FAILED", "severidad": "CRITICAL", "archivo": ".", "linea": 1})
    return hallazgos, error_git


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description="Escaneo IaC DevSecOps (I-100)")
    parser.add_argument("--raiz", type=Path, default=Path("infra/terraform"))
    parser.add_argument("--sin-git", action="store_true")
    parser.add_argument("--json", type=Path, default=None)
    args = parser.parse_args(argv)
    raiz = args.raiz.resolve()
    if not raiz.is_dir():
        print(f"IAC_DEVSECOPS_FAIL: no existe {raiz}")
        return 1
    hallazgos, error_git = analizar(raiz, comprobar_git=not args.sin_git)
    if args.json is not None:
        args.json.write_text(
            json.dumps({"hallazgos": hallazgos, "git_error": error_git}, ensure_ascii=False, indent=2),
            encoding="utf-8",
        )
    if hallazgos:
        print("IAC_DEVSECOPS_FAIL")
        for hallazgo in hallazgos:
            print(f" - {hallazgo['severidad']} {hallazgo['regla']} {hallazgo['archivo']}:{hallazgo['linea']}")
        return 1
    print(f"IAC_DEVSECOPS_PASS: {len(_archivos(raiz))} archivos sin hallazgos")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
