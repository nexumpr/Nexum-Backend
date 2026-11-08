#!/usr/bin/env python3
"""Simulador determinista del despliegue ASG de I-090 (sin AWS).

Modela la decisión del workflow a partir de un estado de entrada y devuelve la
acción: ABORTAR (sin mutar), ROLLBACK o DESPLEGAR. Sirve para probar las reglas
de capacidad, salud, HTTP y restauración de max_size sin invocar AWS.
"""

from __future__ import annotations

import json
import sys
from pathlib import Path

MAX_TRANSITORIO = 3
MAX_FINAL = 2
NODOS_REQUERIDOS = 2


def evaluar(entrada: dict) -> dict:
    if entrada.get("in_service") != NODOS_REQUERIDOS or entrada.get("healthy_antes") != NODOS_REQUERIDOS:
        return {"accion": "ABORTAR", "mutar": False, "max_final": MAX_FINAL, "motivos": ["NODOS_INSUFICIENTES_ANTES"]}
    if entrada.get("max_size") != MAX_TRANSITORIO:
        return {"accion": "ABORTAR", "mutar": False, "max_final": MAX_FINAL, "motivos": ["MAX_SIZE_INESPERADO"]}
    if entrada.get("refresh") != "Successful":
        return {"accion": "ROLLBACK", "mutar": True, "max_final": MAX_FINAL, "motivos": ["REFRESH_NO_EXITOSO"]}
    if entrada.get("healthy_despues") != NODOS_REQUERIDOS:
        return {"accion": "ROLLBACK", "mutar": True, "max_final": MAX_FINAL, "motivos": ["NODOS_INSUFICIENTES_DESPUES"]}
    if entrada.get("http_live") != 200:
        return {"accion": "ROLLBACK", "mutar": True, "max_final": MAX_FINAL, "motivos": ["HTTP_LIVE_NO_200"]}
    if entrada.get("readiness_required") and entrada.get("http_ready") != 200:
        return {"accion": "ROLLBACK", "mutar": True, "max_final": MAX_FINAL, "motivos": ["HTTP_READY_NO_200"]}
    return {"accion": "DESPLEGAR", "mutar": True, "max_final": MAX_FINAL, "motivos": []}


def main(argv: list[str]) -> int:
    if len(argv) != 2:
        print("uso: simular_despliegue.py <entrada.json>")
        return 2
    ruta = Path(argv[1])
    if not ruta.is_file():
        print("SIMULACION_FAIL: no existe la entrada")
        return 1
    try:
        entrada = json.loads(ruta.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        print(f"SIMULACION_FAIL: JSON invalido: {exc}")
        return 1
    print(json.dumps(evaluar(entrada), ensure_ascii=False))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
