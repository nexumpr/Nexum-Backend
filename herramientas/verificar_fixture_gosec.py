#!/usr/bin/env python3
"""Verifica la prueba negativa de gosec (I-100).

El job de seguridad ejecuta gosec sobre un fixture con `math/rand` y espera que
falle con G404. Este script exige código de salida distinto de 0 y el hallazgo
concreto, para no aceptar un error de compilación o ruta como detección.
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description="Prueba negativa de gosec (I-100)")
    parser.add_argument("salida", type=Path)
    parser.add_argument("codigo", type=int)
    parser.add_argument("--regla", default="G404")
    args = parser.parse_args(argv)
    if args.codigo == 0:
        print("GOSEC_FIXTURE_FAIL: gosec no falló ante el fixture inseguro")
        return 1
    if not args.salida.is_file():
        print("GOSEC_FIXTURE_FAIL: no existe la salida de gosec")
        return 1
    texto = args.salida.read_text(encoding="utf-8", errors="replace")
    if args.regla not in texto:
        print(f"GOSEC_FIXTURE_FAIL: no aparece la regla {args.regla} en la salida")
        return 1
    print(f"GOSEC_FIXTURE_PASS: detección real de {args.regla} con salida {args.codigo}")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
