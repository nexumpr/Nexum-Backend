#!/usr/bin/env python3
"""Decide si un rollback debe cancelar un instance refresh activo (I-090).

No ejecuta AWS: recibe `describe-instance-refreshes` y emite la acción que el
workflow debe aplicar antes de iniciar un refresh de rollback.
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

ACTIVOS = {"Pending", "InProgress", "Cancelling", "Baking"}


def decidir(datos: dict) -> dict:
    refreshes = datos.get("InstanceRefreshes") or []
    activos = [r for r in refreshes if r.get("Status") in ACTIVOS]
    if activos:
        return {
            "accion": "CANCELAR_Y_ESPERAR",
            "refresh_id": activos[0].get("InstanceRefreshId"),
            "estado": activos[0].get("Status"),
        }
    return {"accion": "PROCEDER", "refresh_id": None, "estado": (refreshes[0].get("Status") if refreshes else "SIN_REFRESH")}


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description="Decisión de rollback (I-090)")
    parser.add_argument("refreshes_json", type=Path)
    parser.add_argument("--salida", type=Path, required=True)
    args = parser.parse_args(argv)
    if not args.refreshes_json.is_file():
        print("ROLLBACK_DECISION_FAIL: no existe el JSON de refreshes")
        return 1
    try:
        datos = json.loads(args.refreshes_json.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        print(f"ROLLBACK_DECISION_FAIL: JSON invalido: {exc}")
        return 1
    decision = decidir(datos)
    args.salida.write_text(json.dumps(decision, ensure_ascii=False), encoding="utf-8")
    print(json.dumps(decision, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
