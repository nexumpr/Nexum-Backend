#!/usr/bin/env python3
"""Verifica la prueba negativa del escáner IaC (I-080).

El escáner (Trivy) se ejecuta sobre un fixture deliberadamente inseguro y se espera
código de salida 1. Este script exige además que el informe JSON contenga al menos
un hallazgo HIGH/CRITICAL, para no aceptar un fallo por error de parseo o ruta
inexistente como si fuera una detección real.

Uso:
    python3 herramientas/verificar_fixture_iac.py <trivy.json> <exit_code>
"""

from __future__ import annotations

import json
import sys
from pathlib import Path

SEVERIDADES_BLOQUEANTES = {"HIGH", "CRITICAL"}


def hallazgos_bloqueantes(informe: Path) -> list[tuple[str, str, str]]:
    datos = json.loads(informe.read_text(encoding="utf-8"))
    encontrados: list[tuple[str, str, str]] = []
    for resultado in datos.get("Results") or []:
        for misconfig in resultado.get("Misconfigurations") or []:
            severidad = str(misconfig.get("Severity", "")).upper()
            if severidad in SEVERIDADES_BLOQUEANTES:
                encontrados.append((str(misconfig.get("ID", "")), severidad, str(resultado.get("Target", ""))))
    return encontrados


def main(argv: list[str]) -> int:
    if len(argv) != 3:
        print("uso: verificar_fixture_iac.py <trivy.json> <exit_code>")
        return 2
    informe = Path(argv[1])
    try:
        codigo = int(argv[2])
    except ValueError:
        print(f"FIXTURE_IAC_FAIL: exit_code no numerico: {argv[2]}")
        return 1
    if not informe.is_file():
        print(f"FIXTURE_IAC_FAIL: no existe el informe {informe}")
        return 1
    try:
        hallazgos = hallazgos_bloqueantes(informe)
    except (json.JSONDecodeError, OSError) as exc:
        print(f"FIXTURE_IAC_FAIL: informe invalido: {exc}")
        return 1
    if codigo != 1:
        print(f"FIXTURE_IAC_FAIL: se esperaba exit 1 del escaner, obtenido {codigo}")
        return 1
    if not hallazgos:
        print("FIXTURE_IAC_FAIL: el escaner devolvio 1 pero no hay hallazgos HIGH/CRITICAL")
        return 1
    for identificador, severidad, objetivo in sorted(hallazgos):
        print(f"FIXTURE_IAC_DETECTADO: {severidad} {identificador} en {objetivo}")
    print(f"FIXTURE_IAC_PASS: {len(hallazgos)} hallazgo(s) HIGH/CRITICAL y exit 1")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
