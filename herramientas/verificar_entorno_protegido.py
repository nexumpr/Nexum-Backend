#!/usr/bin/env python3
"""Verifica que el entorno `aws-deploy` tiene revisores obligatorios.

Recibe el JSON de `gh api repos/<owner>/<repo>/environments/aws-deploy`.
Si el token no puede leerlo, el paso que lo invoca falla antes (falla cerrado).
"""

from __future__ import annotations

import json
import sys
from pathlib import Path

ENTORNO = "aws-deploy"


def validar(datos: dict) -> list[str]:
    problemas: list[str] = []
    if datos.get("name") != ENTORNO:
        problemas.append("ENTORNO_DISTINTO")
    reglas = datos.get("protection_rules") or []
    revisores = [
        regla for regla in reglas
        if regla.get("type") == "required_reviewers" and regla.get("reviewers")
    ]
    if not revisores:
        problemas.append("SIN_REVISORES_OBLIGATORIOS")
    return problemas


def main(argv: list[str]) -> int:
    if len(argv) != 2:
        print("uso: verificar_entorno_protegido.py <entorno.json>")
        return 2
    ruta = Path(argv[1])
    if not ruta.is_file():
        print("ENTORNO_PROTEGIDO_FAIL: no existe el JSON del entorno")
        return 1
    try:
        datos = json.loads(ruta.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        print(f"ENTORNO_PROTEGIDO_FAIL: JSON invalido: {exc}")
        return 1
    problemas = validar(datos)
    if problemas:
        print("ENTORNO_PROTEGIDO_FAIL")
        for problema in problemas:
            print(" -", problema)
        return 1
    print("ENTORNO_PROTEGIDO_PASS: aws-deploy exige revisores")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
