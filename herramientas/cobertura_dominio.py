#!/usr/bin/env python3
"""Umbral de cobertura del dominio (I-080 / INF-080-04).

Lee un `coverprofile` generado por `go test -coverpkg` y calcula la cobertura de
sentencias de los paquetes de `internal/domain/`. El perfil puede contener bloques
repetidos cuando se combinan varios binarios de test: se deduplica por localización
tomando el máximo de ejecuciones, igual que hace `go tool cover`.

Uso:
    python3 herramientas/cobertura_dominio.py coverage.out [--umbral 80]

Salida: tabla por paquete y total. Código de salida 1 si no alcanza el umbral o si
el perfil no contiene bloques del dominio (falla cerrado). No consulta AWS.
"""

from __future__ import annotations

import argparse
import sys
from collections import defaultdict
from pathlib import Path

PREFIJO_DOMINIO = "internal/domain/"


def cargar_bloques(perfil: Path) -> dict[str, tuple[int, int, str]]:
    """Devuelve {localización: (count, numStmt, paquete)} deduplicado por bloque."""
    if not perfil.is_file():
        raise FileNotFoundError(f"no existe el perfil de cobertura: {perfil}")
    bloques: dict[str, tuple[int, int, str]] = {}
    for linea in perfil.read_text(encoding="utf-8").splitlines():
        if not linea or linea.startswith("mode:"):
            continue
        partes = linea.rsplit(" ", 2)
        if len(partes) != 3:
            continue
        localizacion, num_stmt, cuenta = partes[0], partes[1], partes[2]
        try:
            num = int(num_stmt)
            ejecuciones = int(cuenta)
        except ValueError:
            continue
        if PREFIJO_DOMINIO not in localizacion:
            continue
        archivo = localizacion.split(":", 1)[0]
        paquete = archivo.split(PREFIJO_DOMINIO, 1)[1].split("/", 1)[0]
        previo = bloques.get(localizacion)
        if previo is None or ejecuciones > previo[0]:
            bloques[localizacion] = (ejecuciones, num, paquete)
    return bloques


def calcular(bloques: dict[str, tuple[int, int, str]]) -> dict[str, tuple[int, int]]:
    """Cobertura por paquete: {paquete: (cubiertas, total)}."""
    datos: dict[str, list[int]] = defaultdict(lambda: [0, 0])
    for ejecuciones, num, paquete in bloques.values():
        datos[paquete][1] += num
        if ejecuciones > 0:
            datos[paquete][0] += num
    return {k: (v[0], v[1]) for k, v in datos.items()}


def evaluar(perfil: Path, umbral: float) -> int:
    bloques = cargar_bloques(perfil)
    if not bloques:
        print("COBERTURA_DOMINIO_FAIL: el perfil no contiene bloques de internal/domain/")
        return 1
    datos = calcular(bloques)
    cubiertas = sum(c for c, _ in datos.values())
    total = sum(t for _, t in datos.values())
    print("paquete                       cubiertas/total   cobertura")
    for paquete in sorted(datos):
        c, t = datos[paquete]
        print(f"{paquete:<28} {c:>6}/{t:<6} {100.0 * c / t:>9.1f}%")
    porcentaje = 100.0 * cubiertas / total if total else 0.0
    print(f"{'TOTAL':<28} {cubiertas:>6}/{total:<6} {porcentaje:>9.1f}%")
    if porcentaje + 1e-9 < umbral:
        print(f"COBERTURA_DOMINIO_FAIL: {porcentaje:.1f}% < umbral {umbral:.1f}%")
        return 1
    print(f"COBERTURA_DOMINIO_PASS: {porcentaje:.1f}% >= umbral {umbral:.1f}%")
    return 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Umbral de cobertura del dominio (I-080)")
    parser.add_argument("perfil", type=Path)
    parser.add_argument("--umbral", type=float, default=80.0)
    args = parser.parse_args(argv)
    if args.umbral <= 0 or args.umbral > 100:
        print("umbral fuera de rango (0, 100]")
        return 2
    try:
        return evaluar(args.perfil, args.umbral)
    except FileNotFoundError as exc:
        print(f"COBERTURA_DOMINIO_FAIL: {exc}")
        return 1


if __name__ == "__main__":
    sys.exit(main())
