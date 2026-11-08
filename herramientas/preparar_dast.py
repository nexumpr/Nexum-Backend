#!/usr/bin/env python3
"""Prepara (sin ejecutar) un escaneo DAST autorizado de I-100.

No lanza ZAP ni ninguna herramienta ofensiva. Sólo valida objetivo, autorización,
datos sintéticos, ventana, rate limit y alcance, y emite un plan reproducible.
Sin `APRUEBO_GATE_DAST` devuelve BLOCKED_AUTHORIZATION y no genera plan.
"""

from __future__ import annotations

import argparse
import datetime as dt
import json
import sys
from pathlib import Path
from urllib.parse import urlparse

HOSTS_PRIVADOS = ("localhost", "127.0.0.1", "::1")
SUFIJOS_PRIVADOS = (".local", ".internal", ".test")
MAX_RPS = 5
MAX_DURACION = dt.timedelta(hours=4)


def validar(
    objetivo: str,
    autorizacion: str,
    datos_sinteticos: str,
    inicio: str,
    fin: str,
    rps: int,
    alcance: list[str],
    gate: str,
    permitir_remoto: bool,
) -> list[str]:
    problemas: list[str] = []
    url = urlparse(objetivo)
    if url.scheme not in ("http", "https") or not url.hostname:
        problemas.append("OBJETIVO_INVALIDO")
    host = (url.hostname or "").lower()
    privado = host in HOSTS_PRIVADOS or any(host.endswith(s) for s in SUFIJOS_PRIVADOS)
    if not privado and not permitir_remoto:
        problemas.append("OBJETIVO_NO_AISLADO")
    if autorizacion != f"AUTORIZO_DAST_{host}":
        problemas.append("AUTORIZACION_NO_CORRESPONDE_AL_OBJETIVO")
    if datos_sinteticos != "SI":
        problemas.append("DATOS_NO_SINTETICOS")
    try:
        t_inicio = dt.datetime.fromisoformat(inicio)
        t_fin = dt.datetime.fromisoformat(fin)
        if t_fin <= t_inicio:
            problemas.append("VENTANA_INVALIDA")
        elif t_fin - t_inicio > MAX_DURACION:
            problemas.append("VENTANA_EXCESIVA")
    except ValueError:
        problemas.append("VENTANA_INVALIDA")
    if rps < 1 or rps > MAX_RPS:
        problemas.append("RPS_FUERA_DE_LIMITE")
    if not alcance or any(not ruta.startswith("/") for ruta in alcance):
        problemas.append("ALCANCE_INVALIDO")
    if gate != "APRUEBO_GATE_DAST":
        problemas.append("GATE_DAST_AUSENTE")
    return problemas


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description="Plan DAST de I-100 (no ejecuta)")
    parser.add_argument("--objetivo", required=True)
    parser.add_argument("--autorizacion", required=True)
    parser.add_argument("--datos-sinteticos", required=True)
    parser.add_argument("--ventana-inicio", required=True)
    parser.add_argument("--ventana-fin", required=True)
    parser.add_argument("--rps", type=int, default=2)
    parser.add_argument("--alcance", required=True, help="rutas separadas por coma")
    parser.add_argument("--gate", default="")
    parser.add_argument("--permitir-remoto", action="store_true")
    parser.add_argument("--salida", type=Path, default=None)
    args = parser.parse_args(argv)
    alcance = [ruta.strip() for ruta in args.alcance.split(",") if ruta.strip()]
    problemas = validar(
        args.objetivo, args.autorizacion, args.datos_sinteticos,
        args.ventana_inicio, args.ventana_fin, args.rps, alcance,
        args.gate, args.permitir_remoto,
    )
    if "GATE_DAST_AUSENTE" in problemas:
        print("DAST_BLOCKED_AUTHORIZATION: falta APRUEBO_GATE_DAST; no se genera plan")
        return 1
    if problemas:
        print("DAST_PLAN_FAIL")
        for problema in problemas:
            print(" -", problema)
        return 1
    plan = {
        "herramienta": "zap-api-scan",
        "objetivo": args.objetivo,
        "alcance": alcance,
        "datos": "sinteticos",
        "ventana": {"inicio": args.ventana_inicio, "fin": args.ventana_fin},
        "rps": args.rps,
        "estado": "PLAN_PREPARADO_NO_EJECUTADO",
    }
    if args.salida is not None:
        args.salida.write_text(json.dumps(plan, ensure_ascii=False, indent=2), encoding="utf-8")
    print("DAST_PLAN_PASS: plan preparado; ejecución requiere Gate DAST y ventana autorizada")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
