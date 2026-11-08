#!/usr/bin/env python3
"""Verifica el checksum SHA-256 de un objeto S3 tras la subida (I-090).

`aws s3api head-object --checksum-mode ENABLED` devuelve `ChecksumSHA256` en
base64. Se compara con el digest hexadecimal autorizado.
"""

from __future__ import annotations

import argparse
import base64
import json
import sys
from pathlib import Path


def validar(datos: dict, sha256_hex: str) -> list[str]:
    checksum = datos.get("ChecksumSHA256")
    if not checksum:
        return ["SIN_CHECKSUM_SHA256"]
    try:
        esperado = base64.b64encode(bytes.fromhex(sha256_hex)).decode("ascii")
    except ValueError:
        return ["SHA256_HEX_INVALIDO"]
    if checksum != esperado:
        return ["CHECKSUM_NO_COINCIDE"]
    return []


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description="Checksum de objeto S3 (I-090)")
    parser.add_argument("head_json", type=Path)
    parser.add_argument("--sha256", required=True)
    args = parser.parse_args(argv)
    if not args.head_json.is_file():
        print("OBJETO_S3_FAIL: no existe el JSON de head-object")
        return 1
    try:
        datos = json.loads(args.head_json.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        print(f"OBJETO_S3_FAIL: JSON invalido: {exc}")
        return 1
    problemas = validar(datos, args.sha256)
    if problemas:
        print("OBJETO_S3_FAIL")
        for problema in problemas:
            print(" -", problema)
        return 1
    print("OBJETO_S3_PASS: checksum SHA-256 coincide con el release")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
