#!/usr/bin/env python3
"""Certificación funcional de las 75 operaciones — inventario y cobertura verificables.

Por defecto hace dry-run: carga el OpenAPI, exige exactamente 75 `operationId`, comprueba
que cada operación tiene un escenario registrado con el mismo método y ruta, y devuelve:

    0  inventario y cobertura exactos (dry-run válido)
    2  certificación incompleta: falta/sobra una operación, método o ruta no coincide,
       o se pidió `--run` sin un ejecutor disponible
    1  fallo de ejecución

`--run` exige autorización explícita de mutaciones y un ejecutor cableado; en esta entrega
el ejecutor de escenarios todavía depende de `escenarios.Contexto` y el CLI no lo construye,
por lo que `--run` devuelve 2 con un mensaje exacto en lugar de fingir una certificación.
"""
from __future__ import annotations

import argparse
import json
import os
import sys
from pathlib import Path

AQUI = Path(__file__).resolve().parent
RAIZ = AQUI.parents[1]
sys.path.insert(0, str(AQUI))

import nucleo  # noqa: E402

OPENAPI_POR_DEFECTO = RAIZ / "sdd" / "03-endpoints" / "openapi" / "nexum-api-v1.json"

RC_OK = 0
RC_FALLO = 1
RC_INCOMPLETA = 2


def cargar_escenarios():
    """Importa el registro de escenarios; un módulo ausente no se simula."""
    try:
        import escenarios  # type: ignore
    except Exception as exc:  # noqa: BLE001
        return None, f"no se pudo importar escenarios: {type(exc).__name__}: {exc}"
    return escenarios, ""


def inventario(ruta_openapi: Path) -> dict:
    openapi = nucleo.cargar_openapi(ruta_openapi)
    return nucleo.extraer_operaciones(openapi)


def cobertura(operaciones: dict, escenarios) -> tuple[bool, list[str]]:
    """Devuelve (exacta, problemas) sin imprimir secretos."""
    if escenarios is None:
        return False, ["registro de escenarios no disponible"]
    registro = getattr(escenarios, "ESCENARIOS", None)
    orden = getattr(escenarios, "ORDEN_EJECUCION", ())
    if not isinstance(registro, dict):
        return False, ["ESCENARIOS no es un mapping E-ID → escenario"]
    informe = nucleo.verificar_registro(operaciones, registro, orden)
    problemas: list[str] = []
    problemas.extend(f"FALTA {eid}" for eid in getattr(informe, "faltantes", []))
    problemas.extend(f"SOBRA {eid}" for eid in getattr(informe, "sobrantes", []))
    problemas.extend(getattr(informe, "duplicados", []))
    problemas.extend(getattr(informe, "discrepancias", []))
    exacta = (
        len(operaciones) == 75
        and len(registro) == 75
        and not problemas
    )
    return exacta, problemas


def main(argv: list[str]) -> int:
    ap = argparse.ArgumentParser(description="Certificación 75/75 (inventario y cobertura)")
    ap.add_argument("--openapi", default=str(OPENAPI_POR_DEFECTO))
    ap.add_argument("--run", action="store_true", help="ejecutar escenarios (requiere ejecutor)")
    ap.add_argument("--allow-mutations", action="store_true")
    ap.add_argument("--base-url", default="")
    ap.add_argument("--out", default="")
    args = ap.parse_args(argv)

    try:
        operaciones = inventario(Path(args.openapi))
    except Exception as exc:  # noqa: BLE001
        print(json.dumps({"estado": "INVENTARIO_INVALIDO", "detalle": f"{type(exc).__name__}"}))
        return RC_INCOMPLETA

    escenarios, error_escenarios = cargar_escenarios()
    exacta, problemas = cobertura(operaciones, escenarios)

    resumen = {
        "operaciones_openapi": len(operaciones),
        "escenarios": len(getattr(escenarios, "ESCENARIOS", {}) or {}),
        "cobertura_exacta": exacta,
        "problemas": problemas[:50],
        "modo": "dry-run",
        "aws_consultado": False,
        "ejecutado_contra_produccion": False,
    }
    if error_escenarios:
        resumen["problemas"].append(error_escenarios)

    if args.run:
        if not args.allow_mutations:
            resumen["estado"] = "MUTACIONES_NO_AUTORIZADAS"
            print(json.dumps(resumen, ensure_ascii=False))
            return RC_INCOMPLETA
        resumen["estado"] = "EJECUTOR_PENDIENTE"
        resumen["detalle"] = (
            "El ejecutor de escenarios requiere construir escenarios.Contexto; "
            "no se ejecutó nada para no fingir una certificación."
        )
        print(json.dumps(resumen, ensure_ascii=False))
        return RC_INCOMPLETA

    resumen["estado"] = "COBERTURA_EXACTA" if exacta else "COBERTURA_INCOMPLETA"
    if args.out:
        Path(args.out).write_text(json.dumps(resumen, ensure_ascii=False, indent=2), encoding="utf-8")
    print(json.dumps(resumen, ensure_ascii=False))
    return RC_OK if exacta else RC_INCOMPLETA


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
