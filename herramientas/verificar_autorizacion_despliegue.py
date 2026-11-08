#!/usr/bin/env python3
"""Verifica la autorización de un despliegue manual de I-090.

No despliega ni consulta AWS: valida en el runner que el release es inmutable, que
la frase de autorización está ligada al SHA exacto, que el operador confirmó y que
el despliegue está habilitado. Cualquier fallo devuelve 1 y bloquea el job.
"""

from __future__ import annotations

import argparse
import re
import sys

PATRON_SHA = re.compile(r"^[0-9a-f]{40}$")
PATRON_RELEASE = re.compile(r"^v\d+\.\d+\.\d+\+[0-9a-f]{12}$")
PATRON_DIGEST = re.compile(r"^[0-9a-f]{64}$")


def validar(
    sha: str,
    release_id: str,
    frase: str,
    confirmar: str,
    habilitado: str,
    sha256_binario: str,
    total_nodos: int,
    lt_version: str = "1",
) -> list[str]:
    problemas: list[str] = []
    if not PATRON_SHA.match(sha):
        problemas.append("SHA_INVALIDO")
    if not PATRON_RELEASE.match(release_id):
        problemas.append("RELEASE_ID_INVALIDO")
    elif not release_id.endswith("+" + sha[:12]):
        problemas.append("RELEASE_NO_CORRESPONDE_AL_SHA")
    if not PATRON_DIGEST.match(sha256_binario):
        problemas.append("SHA256_BINARIO_INVALIDO")
    if frase != f"AUTORIZO_DESPLIEGUE_{sha}":
        problemas.append("FRASE_NO_CORRESPONDE_AL_SHA")
    if confirmar != "SI":
        problemas.append("CONFIRMACION_AUSENTE")
    if habilitado != "true":
        problemas.append("CD_NO_HABILITADO")
    if total_nodos < 2:
        problemas.append("NODOS_INSUFICIENTES")
    if not re.fullmatch(r"[0-9]+", lt_version or ""):
        problemas.append("LT_VERSION_INVALIDA")
    return problemas


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description="Autorización de despliegue I-090")
    parser.add_argument("--sha", required=True)
    parser.add_argument("--release-id", required=True)
    parser.add_argument("--frase", required=True)
    parser.add_argument("--confirmar", required=True)
    parser.add_argument("--habilitado", required=True)
    parser.add_argument("--sha256-binario", required=True)
    parser.add_argument("--total-nodos", type=int, default=2)
    parser.add_argument("--lt-version", default="1")
    args = parser.parse_args(argv)
    problemas = validar(
        args.sha,
        args.release_id,
        args.frase,
        args.confirmar,
        args.habilitado,
        args.sha256_binario,
        args.total_nodos,
        args.lt_version,
    )
    if problemas:
        print("AUTORIZACION_DESPLIEGUE_FAIL")
        for problema in problemas:
            print(" -", problema)
        return 1
    print("AUTORIZACION_DESPLIEGUE_PASS: release, SHA y autorización coherentes")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
