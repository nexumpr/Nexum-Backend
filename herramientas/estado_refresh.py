#!/usr/bin/env python3
"""Evalúa el estado de un instance refresh concreto de ASG (I-090).

Recibe el JSON de `describe-instance-refreshes` y un InstanceRefreshId. Emite la
acción que el workflow debe tomar; nunca autoriza operaciones con estados
desconocidos ni con un ID distinto.
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

ACTIVOS = {"Pending", "InProgress", "Cancelling", "Baking", "RollbackInProgress"}
CANCELADOS = {"Cancelled", "Failed"}
ROLLBACK_OK = {"RollbackSuccessful"}
EXITO = {"Successful"}


def evaluar(datos: dict, refresh_id: str, espera: str) -> str:
    refreshes = datos.get("InstanceRefreshes") or []
    objetivo = next((r for r in refreshes if r.get("InstanceRefreshId") == refresh_id), None)
    if objetivo is None:
        return "ID_NO_ENCONTRADO"
    estado = objetivo.get("Status")
    if estado in ACTIVOS:
        if estado == "RollbackInProgress":
            return "ROLLBACK_EN_PROGRESO"
        return "ESPERAR"
    if espera == "cancelacion":
        if estado in CANCELADOS:
            return "CANCELADO_OK"
        if estado in ROLLBACK_OK:
            return "CANCELADO_OK"
        if estado in EXITO:
            return "YA_TERMINADO"
        return "ESTADO_DESCONOCIDO"
    if espera == "recuperacion":
        if estado in EXITO or estado in ROLLBACK_OK:
            return "RECUPERACION_OK"
        if estado in CANCELADOS or estado == "RollbackFailed":
            return "RECUPERACION_FALLIDA"
        return "ESTADO_DESCONOCIDO"
    return "MODO_INVALIDO"


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description="Estado de instance refresh (I-090)")
    parser.add_argument("refreshes_json", type=Path)
    parser.add_argument("--id", required=True)
    parser.add_argument("--espera", choices=("cancelacion", "recuperacion"), required=True)
    parser.add_argument("--salida", type=Path, default=None)
    args = parser.parse_args(argv)
    if not args.refreshes_json.is_file():
        print("REFRESH_FAIL: no existe el JSON de refreshes")
        return 1
    try:
        datos = json.loads(args.refreshes_json.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        print(f"REFRESH_FAIL: JSON invalido: {exc}")
        return 1
    accion = evaluar(datos, args.id, args.espera)
    if args.salida is not None:
        args.salida.write_text(json.dumps({"accion": accion, "id": args.id}, ensure_ascii=False), encoding="utf-8")
    print(accion)
    return 1 if accion == "ESTADO_DESCONOCIDO" else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
