#!/usr/bin/env python3
"""Verifica que las instancias InService se iniciaron con el release autorizado.

Recibe un archivo con una UserData base64 por línea (salida de
`aws ec2 describe-instance-attribute --attribute userData --output text`).
Exige el número de instancias y que todas referencien la URI y el digest del release.
"""

from __future__ import annotations

import argparse
import base64
import binascii
import sys
from pathlib import Path


def validar(lineas: list[str], bucket: str, release_id: str, sha256: str, instancias: int) -> list[str]:
    problemas: list[str] = []
    limpias = [linea.strip() for linea in lineas if linea.strip()]
    if len(limpias) != instancias:
        problemas.append(f"INSTANCIAS_INESPERADAS {len(limpias)} != {instancias}")
    uri = f"s3://{bucket}/releases/{release_id}/nexum-backend-linux-arm64"
    for indice, codificada in enumerate(limpias, start=1):
        try:
            user_data = base64.b64decode(codificada).decode("utf-8", errors="replace")
        except (binascii.Error, ValueError):
            problemas.append(f"USERDATA_INVALIDA {indice}")
            continue
        if uri not in user_data:
            problemas.append(f"INSTANCIA_SIN_URI {indice}")
        if sha256.lower() not in user_data.lower():
            problemas.append(f"INSTANCIA_SIN_DIGEST {indice}")
    return problemas


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description="Release efectivo en instancias (I-090)")
    parser.add_argument("userdata_txt", type=Path)
    parser.add_argument("--bucket", required=True)
    parser.add_argument("--release-id", required=True)
    parser.add_argument("--sha256", required=True)
    parser.add_argument("--instancias", type=int, default=2)
    args = parser.parse_args(argv)
    if not args.userdata_txt.is_file():
        print("INSTANCIAS_RELEASE_FAIL: no existe el archivo de UserData")
        return 1
    lineas = args.userdata_txt.read_text(encoding="utf-8", errors="replace").splitlines()
    problemas = validar(lineas, args.bucket, args.release_id, args.sha256, args.instancias)
    if problemas:
        print("INSTANCIAS_RELEASE_FAIL")
        for problema in problemas:
            print(" -", problema)
        return 1
    print(f"INSTANCIAS_RELEASE_PASS: {args.instancias} instancias con el release autorizado")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
