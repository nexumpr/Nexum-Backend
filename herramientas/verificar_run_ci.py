#!/usr/bin/env python3
"""Verifica autenticidad de un run de CI para promocion (I-090).

Recibe el JSON de `gh api .../actions/runs/<id>` y, opcionalmente, el de
`.../runs/<id>/artifacts`. No consulta la red.

Criterios: repositorio, workflow productor, evento, rama, estado/conclusión, SHA
exacto y artefactos requeridos presentes en ese run.
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

REPOSITORIO = "nexumpr/Nexum-Backend"
WORKFLOW = ".github/workflows/go-backend.yml"
EVENTO = "push"
RAMA = "main"


def validar(datos: dict, sha_esperado: str, workflow: str, evento: str, rama: str) -> list[str]:
    problemas: list[str] = []
    if datos.get("status") != "completed":
        problemas.append("RUN_NO_COMPLETADO")
    if datos.get("conclusion") != "success":
        problemas.append("RUN_NO_VERDE")
    if datos.get("head_sha") != sha_esperado:
        problemas.append("RUN_SHA_DISTINTO")
    if (datos.get("repository") or {}).get("full_name") != REPOSITORIO:
        problemas.append("RUN_REPOSITORIO_DISTINTO")
    if datos.get("head_branch") != rama:
        problemas.append("RUN_RAMA_NO_AUTORIZADA")
    if datos.get("event") != evento:
        problemas.append("RUN_EVENTO_NO_AUTORIZADO")
    if datos.get("path") != workflow:
        problemas.append("RUN_WORKFLOW_NO_AUTORIZADO")
    return problemas


def validar_artefactos(datos: dict | None, requeridos: list[str]) -> list[str]:
    if not requeridos:
        return []
    if datos is None:
        return ["ARTEFACTOS_NO_VERIFICABLES"]
    nombres = {art.get("name") for art in datos.get("artifacts") or []}
    return [f"ARTEFACTO_FALTANTE {nombre}" for nombre in requeridos if nombre not in nombres]


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description="Verificación de run de CI (I-090)")
    parser.add_argument("run_json", type=Path)
    parser.add_argument("sha_esperado")
    parser.add_argument("--workflow-path", default=WORKFLOW)
    parser.add_argument("--evento", default=EVENTO)
    parser.add_argument("--rama", default=RAMA)
    parser.add_argument("--artefactos-json", type=Path, default=None)
    parser.add_argument("--artefactos", default="")
    args = parser.parse_args(argv)
    if not args.run_json.is_file():
        print("RUN_CI_FAIL: no existe el JSON del run")
        return 1
    try:
        datos = json.loads(args.run_json.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        print(f"RUN_CI_FAIL: JSON invalido: {exc}")
        return 1
    problemas = validar(datos, args.sha_esperado, args.workflow_path, args.evento, args.rama)
    requeridos = [n for n in args.artefactos.split(",") if n]
    artefactos = None
    if args.artefactos_json is not None:
        if not args.artefactos_json.is_file():
            problemas.append("ARTEFACTOS_JSON_AUSENTE")
        else:
            artefactos = json.loads(args.artefactos_json.read_text(encoding="utf-8"))
    problemas += validar_artefactos(artefactos, requeridos)
    if problemas:
        print("RUN_CI_FAIL")
        for problema in problemas:
            print(" -", problema)
        return 1
    print("RUN_CI_PASS: run autentico, verde, del workflow/SHA autorizados y con artefactos")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
