#!/usr/bin/env python3
"""Verifica que la versión del Launch Template contiene el release autorizado.

Recibe el JSON de `aws ec2 describe-launch-template-versions` y decodifica su
UserData (base64). Exige la URI S3 del release y el SHA-256 del binario.
"""

from __future__ import annotations

import argparse
import base64
import binascii
import json
import re
import sys
from pathlib import Path

PATRON_DIGEST = re.compile(r"^[0-9a-f]{64}$")


def validar(datos: dict, bucket: str, release_id: str, sha256: str, version: str) -> list[str]:
    problemas: list[str] = []
    if not PATRON_DIGEST.match(sha256):
        return ["SHA256_INVALIDO"]
    versiones = datos.get("LaunchTemplateVersions") or []
    if not versiones:
        return ["LT_VERSION_AUSENTE"]
    lt = versiones[0]
    if str(lt.get("VersionNumber")) != version:
        problemas.append("LT_VERSION_DISTINTA")
    user_data_b64 = (lt.get("LaunchTemplateData") or {}).get("UserData")
    if not user_data_b64:
        problemas.append("LT_SIN_USERDATA")
        return problemas
    try:
        user_data = base64.b64decode(user_data_b64).decode("utf-8", errors="replace")
    except (binascii.Error, ValueError):
        problemas.append("LT_USERDATA_INVALIDA")
        return problemas
    uri = f"s3://{bucket}/releases/{release_id}/nexum-backend-linux-arm64"
    if uri not in user_data:
        problemas.append("LT_SIN_URI_DEL_RELEASE")
    if sha256.lower() not in user_data.lower():
        problemas.append("LT_SIN_DIGEST_DEL_RELEASE")
    return problemas


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description="Release en Launch Template (I-090)")
    parser.add_argument("lt_json", type=Path)
    parser.add_argument("--bucket", required=True)
    parser.add_argument("--release-id", required=True)
    parser.add_argument("--sha256", required=True)
    parser.add_argument("--version", required=True)
    args = parser.parse_args(argv)
    if not args.lt_json.is_file():
        print("LT_RELEASE_FAIL: no existe el JSON del Launch Template")
        return 1
    try:
        datos = json.loads(args.lt_json.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        print(f"LT_RELEASE_FAIL: JSON invalido: {exc}")
        return 1
    problemas = validar(datos, args.bucket, args.release_id, args.sha256, args.version)
    if problemas:
        print("LT_RELEASE_FAIL")
        for problema in problemas:
            print(" -", problema)
        return 1
    print("LT_RELEASE_PASS: el Launch Template contiene el release y digest autorizados")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
