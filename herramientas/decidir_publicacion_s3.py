#!/usr/bin/env python3
"""Decide si un artefacto S3 puede publicarse sin sobrescribir (I-090).

- Subida nueva correcta → PUBLICACION_NUEVA_OK.
- `If-None-Match: *` falla porque ya existe y el checksum coincide → REUTILIZAR_OK.
- Objeto existente con checksum distinto → ARTIFACT_IMMUTABILITY_FAIL.
- Sin objeto verificable → FAIL (falla cerrado).

Nunca autoriza sobrescribir ni borrar.
"""

from __future__ import annotations

import argparse
import base64
import json
import sys
from pathlib import Path


def evaluar(put_exit: int, cabeza: dict | None, sha256_hex: str) -> str:
    try:
        esperado = base64.b64encode(bytes.fromhex(sha256_hex)).decode("ascii")
    except ValueError:
        return "SHA256_INVALIDO"
    if put_exit == 0:
        # La subida nueva debe verificarse igualmente contra el objeto final.
        if cabeza is None or "ChecksumSHA256" not in cabeza:
            return "PUBLICACION_SIN_CHECKSUM"
        return "PUBLICACION_NUEVA_OK" if cabeza["ChecksumSHA256"] == esperado else "CHECKSUM_NO_COINCIDE"
    if cabeza is None:
        return "OBJETO_NO_VERIFICABLE"
    checksum = cabeza.get("ChecksumSHA256")
    if not checksum:
        return "OBJETO_SIN_CHECKSUM"
    if checksum != esperado:
        return "ARTIFACT_IMMUTABILITY_FAIL"
    return "REUTILIZAR_OK"


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description="Inmutabilidad de artefacto S3 (I-090)")
    parser.add_argument("--put-exit", type=int, required=True)
    parser.add_argument("--cabeza", type=Path, default=None)
    parser.add_argument("--sha256", required=True)
    args = parser.parse_args(argv)
    cabeza = None
    if args.cabeza is not None and args.cabeza.is_file():
        try:
            cabeza = json.loads(args.cabeza.read_text(encoding="utf-8"))
        except json.JSONDecodeError:
            print("ARTIFACT_IMMUTABILITY_FAIL: head-object invalido")
            return 1
    resultado = evaluar(args.put_exit, cabeza, args.sha256)
    if resultado in ("PUBLICACION_NUEVA_OK", "REUTILIZAR_OK"):
        print(f"ARTIFACT_IMMUTABILITY_PASS: {resultado}")
        return 0
    print(f"ARTIFACT_IMMUTABILITY_FAIL: {resultado}")
    return 1


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
