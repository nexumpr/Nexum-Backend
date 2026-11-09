"""Verifica la prueba negativa de Semgrep (RFC-008).

El job `sast-semgrep` ejecuta las reglas reales contra `herramientas/semgrep/fixtures/inseguro`
y exige que **todas** las reglas propias detecten su patron. Si una regla deja de
detectar, este verificador falla: la prueba negativa no puede pasar por accidente.

Uso:
    python3 herramientas/verificar_fixture_semgrep.py <reporte.json> <codigo_salida>
"""

from __future__ import annotations

import json
import sys
from pathlib import Path

REGLAS_ESPERADAS = (
    "nexum-tls-skip-verify",
    "nexum-hash-debil-md5",
    "nexum-hash-debil-sha1",
    "nexum-http-cliente-sin-timeout",
)


def _ids(reporte: dict) -> set[str]:
    encontrados: set[str] = set()
    for resultado in reporte.get("results") or []:
        identificador = str(resultado.get("check_id", ""))
        encontrados.add(identificador.split(".")[-1])
    return encontrados


def verificar(ruta: Path, codigo: int) -> tuple[bool, list[str]]:
    problemas: list[str] = []

    if codigo != 1:
        problemas.append(
            f"se esperaba codigo de salida 1 (hallazgos con --error) y se obtuvo {codigo}"
        )
    if not ruta.is_file():
        problemas.append(f"no existe el reporte {ruta}")
        return False, problemas

    try:
        reporte = json.loads(ruta.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        problemas.append(f"reporte ilegible: {exc}")
        return False, problemas

    resultados = reporte.get("results") or []
    if not resultados:
        problemas.append("Semgrep no reporto ningun hallazgo en el fixture inseguro")
        return False, problemas

    rutas = {str(item.get("path", "")) for item in resultados}
    if not any("herramientas/semgrep/fixtures" in ruta.replace("\\", "/") for ruta in rutas):
        problemas.append("los hallazgos no corresponden al fixture esperado")

    detectadas = _ids(reporte)
    faltantes = [regla for regla in REGLAS_ESPERADAS if regla not in detectadas]
    if faltantes:
        problemas.append("reglas que no detectaron su patron: " + ", ".join(faltantes))

    return not problemas, problemas


def main(argv: list[str] | None = None) -> int:
    argumentos = list(sys.argv[1:] if argv is None else argv)
    if len(argumentos) != 2:
        print("uso: verificar_fixture_semgrep.py <reporte.json> <codigo_salida>", file=sys.stderr)
        return 2
    try:
        codigo = int(argumentos[1])
    except ValueError:
        print(f"codigo de salida no numerico: {argumentos[1]}", file=sys.stderr)
        return 2

    correcto, problemas = verificar(Path(argumentos[0]), codigo)
    if correcto:
        print("SEMGREP_FIXTURE_PASS: las cuatro reglas detectaron el fixture inseguro.")
        return 0
    for problema in problemas:
        print(f"::error::{problema}")
    return 1


if __name__ == "__main__":  # pragma: no cover
    sys.exit(main())
