#!/usr/bin/env python3
"""Verifica que el run de CI es verde, del repositorio correcto y del SHA autorizado.

Uso: verificar_run_ci.py <run.json> <sha_esperado>
No consulta la red: recibe el JSON de `gh api .../actions/runs/<id>`.
"""

from __future__ import annotations

import json
import sys
from pathlib import Path

REPOSITORIO = "nexumpr/Nexum-Backend"
RAMAS_PERMITIDAS = {"main"}


def validar(datos: dict, sha_esperado: str) -> list[str]:
    problemas: list[str] = []
    if datos.get("status") != "completed":
        problemas.append("RUN_NO_COMPLETADO")
    if datos.get("conclusion") != "success":
        problemas.append("RUN_NO_VERDE")
    if datos.get("head_sha") != sha_esperado:
        problemas.append("RUN_SHA_DISTINTO")
    if (datos.get("repository") or {}).get("full_name") != REPOSITORIO:
        problemas.append("RUN_REPOSITORIO_DISTINTO")
    if datos.get("head_branch") not in RAMAS_PERMITIDAS:
        problemas.append("RUN_RAMA_NO_AUTORIZADA")
    return problemas


def main(argv: list[str]) -> int:
    if len(argv) != 3:
        print("uso: verificar_run_ci.py <run.json> <sha_esperado>")
        return 2
    ruta = Path(argv[1])
    if not ruta.is_file():
        print("RUN_CI_FAIL: no existe el JSON del run")
        return 1
    try:
        datos = json.loads(ruta.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        print(f"RUN_CI_FAIL: JSON invalido: {exc}")
        return 1
    problemas = validar(datos, argv[2])
    if problemas:
        print("RUN_CI_FAIL")
        for problema in problemas:
            print(" -", problema)
        return 1
    print("RUN_CI_PASS: run completado, verde y del SHA autorizado")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
