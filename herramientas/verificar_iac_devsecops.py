#!/usr/bin/env python3
"""Escaneo estático DevSecOps de IaC (I-100, INF-100-05).

Complementa a Trivy con comprobaciones explícitas exigidas por el SDD:

- Security groups con ingreso desde 0.0.0.0/0 o ::/0.
- Buckets S3 sin cifrado declarado.
- Políticas IAM con Action/Resource comodín.
- `force_destroy = true` en recursos persistentes.
- ACLs públicas.
- Archivos de Terraform state rastreados por Git.

No ejecuta Terraform ni consulta AWS. Código de salida 1 si hay hallazgos.
"""

from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
from pathlib import Path

PATRONES = {
    "SG_PUBLICO": re.compile(r"(cidr_blocks|ipv6_cidr_blocks)\s*=\s*\[[^\]]*(\"0\.0\.0\.0/0\"|\"::/0\")", re.S),
    "IAM_WILDCARD": re.compile(r"\"(Action|Resource)\"\s*:\s*\"\*\"|\b(Action|Resource)\s*=\s*\"\*\"", re.S),
    "FORCE_DESTROY": re.compile(r"force_destroy\s*=\s*true"),
    "ACL_PUBLICA": re.compile(r"acl\s*=\s*\"(public-read|public-read-write|authenticated-read)\""),
}
EXTENSIONES = (".tf", ".tftpl", ".json")


def _archivos(raiz: Path) -> list[Path]:
    return sorted(p for p in raiz.rglob("*") if p.is_file() and p.suffix in EXTENSIONES)


def _state_rastreado(raiz: Path) -> list[str]:
    try:
        salida = subprocess.run(
            ["git", "ls-files", "*.tfstate", "*.tfstate.*"],
            cwd=raiz, capture_output=True, text=True, timeout=30, check=False,
        )
    except (OSError, subprocess.SubprocessError):
        return []
    return [linea for linea in salida.stdout.splitlines() if linea.strip()]


def analizar(raiz: Path, comprobar_git: bool = True) -> list[dict]:
    hallazgos: list[dict] = []
    archivos = _archivos(raiz)
    texto_global = ""
    hay_bucket = False
    hay_cifrado = False
    for archivo in archivos:
        texto = archivo.read_text(encoding="utf-8", errors="replace")
        texto_global += texto
        if re.search(r"resource\s+\"aws_s3_bucket\"", texto):
            hay_bucket = True
        if re.search(r"aws_s3_bucket_server_side_encryption_configuration", texto):
            hay_cifrado = True
        for nombre, patron in PATRONES.items():
            for coincidencia in patron.finditer(texto):
                linea = texto[: coincidencia.start()].count("\n") + 1
                hallazgos.append({
                    "regla": nombre,
                    "severidad": "HIGH",
                    "archivo": archivo.relative_to(raiz).as_posix(),
                    "linea": linea,
                })
    if hay_bucket and not hay_cifrado:
        hallazgos.append({"regla": "S3_SIN_CIFRADO", "severidad": "HIGH", "archivo": ".", "linea": 1})
    if comprobar_git:
        for ruta in _state_rastreado(raiz):
            hallazgos.append({"regla": "TFSTATE_RASTREADO", "severidad": "CRITICAL", "archivo": ruta, "linea": 1})
    return hallazgos


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
    hallazgos = analizar(raiz, comprobar_git=not args.sin_git)
    if args.json is not None:
        args.json.write_text(json.dumps({"hallazgos": hallazgos}, ensure_ascii=False, indent=2), encoding="utf-8")
    if hallazgos:
        print("IAC_DEVSECOPS_FAIL")
        for hallazgo in hallazgos:
            print(f" - {hallazgo['severidad']} {hallazgo['regla']} {hallazgo['archivo']}:{hallazgo['linea']}")
        return 1
    print(f"IAC_DEVSECOPS_PASS: {len(_archivos(raiz))} archivos sin hallazgos")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
