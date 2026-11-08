#!/usr/bin/env python3
"""Prepara (sin ejecutar) un escaneo DAST autorizado de I-100.

No lanza ZAP ni ninguna herramienta ofensiva. Sólo valida objetivo, evidencia de
autorización humana, datos sintéticos, ventana, rate limit y alcance, y emite un
plan reproducible. Sin un documento de Gate DAST verificable devuelve
BLOCKED_AUTHORIZATION y no genera plan.

La cadena `--evidencia-autorizacion` no es una autorización por sí misma: debe ser
un archivo dentro del directorio de control H08, con la frase APRUEBO_GATE_DAST,
el host del objetivo y una caducidad vigente.
"""

from __future__ import annotations

import argparse
import datetime as dt
import json
import re
import sys
from pathlib import Path
from urllib.parse import urlparse

HOSTS_PRIVADOS = ("localhost", "127.0.0.1", "::1")
SUFIJOS_PRIVADOS = (".local", ".internal", ".test")
MAX_RPS = 5
MAX_DURACION = dt.timedelta(hours=4)
FRASE_GATE = "APRUEBO_GATE_DAST"
PATRON_CADUCIDAD = re.compile(r"Caduca el \*\*(\d{4}-\d{2}-\d{2})\*\*")


def _leer_evidencia(ruta: Path, raiz_control: Path, host: str, hoy: dt.date) -> list[str]:
    problemas: list[str] = []
    try:
        resuelta = ruta.resolve()
        base = raiz_control.resolve()
        resuelta.relative_to(base)
    except (OSError, ValueError):
        return ["EVIDENCIA_FUERA_DE_CONTROL"]
    if not resuelta.is_file():
        return ["EVIDENCIA_AUSENTE"]
    texto = resuelta.read_text(encoding="utf-8", errors="replace")
    if FRASE_GATE not in texto:
        problemas.append("EVIDENCIA_SIN_FRASE_GATE")
    if host not in texto:
        problemas.append("EVIDENCIA_NO_CORRESPONDE_AL_HOST")
    caducidad = PATRON_CADUCIDAD.search(texto)
    if caducidad:
        try:
            if hoy > dt.date.fromisoformat(caducidad.group(1)):
                problemas.append("EVIDENCIA_CADUCADA")
        except ValueError:
            problemas.append("EVIDENCIA_CADUCIDAD_INVALIDA")
    else:
        problemas.append("EVIDENCIA_SIN_CADUCIDAD")
    return problemas


def validar(
    objetivo: str,
    autorizacion: str,
    datos_sinteticos: str,
    inicio: str,
    fin: str,
    rps: int,
    alcance: list[str],
    evidencia: Path | None,
    raiz_control: Path,
    hoy: dt.date,
) -> list[str]:
    problemas: list[str] = []
    url = urlparse(objetivo)
    if url.scheme not in ("http", "https") or not url.hostname:
        return ["OBJETIVO_INVALIDO"]
    host = (url.hostname or "").lower()
    privado = host in HOSTS_PRIVADOS or any(host.endswith(s) for s in SUFIJOS_PRIVADOS)
    if not privado:
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
    if evidencia is None:
        problemas.append("GATE_DAST_AUSENTE")
    else:
        problemas += _leer_evidencia(evidencia, raiz_control, host, hoy)
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
    parser.add_argument("--evidencia-autorizacion", type=Path, default=None)
    parser.add_argument("--raiz-control", type=Path, default=Path("sdd/_control/H08"))
    parser.add_argument("--fecha", type=str, default=None, help="fecha de referencia YYYY-MM-DD")
    parser.add_argument("--salida", type=Path, default=None)
    args = parser.parse_args(argv)
    try:
        hoy = dt.date.fromisoformat(args.fecha) if args.fecha else dt.date.today()
    except ValueError:
        print("DAST_PLAN_FAIL: fecha invalida")
        return 2
    alcance = [ruta.strip() for ruta in args.alcance.split(",") if ruta.strip()]
    problemas = validar(
        args.objetivo, args.autorizacion, args.datos_sinteticos,
        args.ventana_inicio, args.ventana_fin, args.rps, alcance,
        args.evidencia_autorizacion, args.raiz_control, hoy,
    )
    if "GATE_DAST_AUSENTE" in problemas or any(p.startswith("EVIDENCIA_") for p in problemas):
        print("DAST_BLOCKED_AUTHORIZATION: no hay Gate DAST verificable; no se genera plan")
        for problema in problemas:
            if problema.startswith("EVIDENCIA_"):
                print(" -", problema)
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
        "estado": "PLAN_AUTORIZADO_NO_EJECUTADO",
    }
    if args.salida is not None:
        args.salida.write_text(json.dumps(plan, ensure_ascii=False, indent=2), encoding="utf-8")
    print("DAST_PLAN_PASS: plan autorizado preparado; ejecución sigue requiriendo operador y ventana")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
