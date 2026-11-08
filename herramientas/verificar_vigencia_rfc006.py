#!/usr/bin/env python3
"""Vigencia automatizada de la excepción AWS-0011 (RFC-006, I-080).

Impide que la excepción del escáner IaC siga activa cuando:

- la RFC-006 no está en estado APROBADA o figura revocada/sin efecto;
- la fecha actual supera la caducidad declarada (2026-12-02 por defecto);
- la caducidad de la RFC y la de `.trivyignore.yaml` no coinciden;
- existe un Gate de despliegue AWS aprobado, que obliga a reevaluar la excepción;
- `.trivyignore.yaml` no contiene la entrada exacta `AWS-0011` para
  `modules/balanceo/main.tf` o le falta `expired_at`.

No consulta AWS ni modifica nada. Código de salida 1 si procede bloquear.
"""

from __future__ import annotations

import argparse
import datetime as dt
import re
import sys
from pathlib import Path

RFC_REL = "sdd/_control/H08/RFC-006-EXCEPCION_WAF_I030.md"
IGNORE_REL = ".trivyignore.yaml"
GATE_GLOB = "GATE_DESPLIEGUE_AWS*.md"
PATRON_CADUCIDAD_RFC = re.compile(r"Caduca el \*\*(\d{4}-\d{2}-\d{2})\*\*")
PATRON_EXPIRED_AT = re.compile(r"expired_at:\s*(\d{4}-\d{2}-\d{2})")
MARCADORES_GATE_APROBADO = ("APRUEBO_DESPLIEGUE", "APROBADO_PARA_DESPLIEGUE", "MERGED", "APPLIED")


def _fecha(texto: str, patron: re.Pattern[str]) -> dt.date | None:
    coincidencia = patron.search(texto)
    if not coincidencia:
        return None
    return dt.date.fromisoformat(coincidencia.group(1))


def _bloque_aws0011(ignore: str) -> str | None:
    lineas = ignore.splitlines()
    for indice, linea in enumerate(lineas):
        if re.match(r"^\s*-\s+id:\s*AWS-0011\s*$", linea):
            bloque = [linea]
            for siguiente in lineas[indice + 1:]:
                if re.match(r"^\s*-\s+id:", siguiente):
                    break
                bloque.append(siguiente)
            return "\n".join(bloque)
    return None


def validar(raiz: Path, hoy: dt.date) -> list[str]:
    problemas: list[str] = []
    rfc = raiz / RFC_REL
    ignore = raiz / IGNORE_REL
    if not rfc.is_file():
        return [f"FALTA_RFC006 {RFC_REL}"]
    if not ignore.is_file():
        return [f"FALTA_IGNORE {IGNORE_REL}"]

    texto_rfc = rfc.read_text(encoding="utf-8")
    texto_ignore = ignore.read_text(encoding="utf-8")

    if "**APROBADA" not in texto_rfc:
        problemas.append("RFC006_NO_APROBADA")
    for marca in ("**REVOCADA", "SIN EFECTO", "**DENEGADA"):
        if marca in texto_rfc:
            problemas.append(f"RFC006_REVOCADA_O_SIN_EFECTO ({marca})")

    caducidad_rfc = _fecha(texto_rfc, PATRON_CADUCIDAD_RFC)
    caducidad_ignore = _fecha(texto_ignore, PATRON_EXPIRED_AT)
    if caducidad_rfc is None:
        problemas.append("RFC006_SIN_CADUCIDAD")
    if caducidad_ignore is None:
        problemas.append("IGNORE_SIN_EXPIRED_AT")
    if caducidad_rfc and caducidad_ignore and caducidad_rfc != caducidad_ignore:
        problemas.append(f"CADUCIDADES_DISTINTAS rfc={caducidad_rfc} ignore={caducidad_ignore}")
    caducidad = caducidad_rfc or caducidad_ignore
    if caducidad and hoy > caducidad:
        problemas.append(f"RFC006_CADUCADA caducidad={caducidad} hoy={hoy}")

    bloque = _bloque_aws0011(texto_ignore)
    if bloque is None:
        problemas.append("IGNORE_SIN_AWS0011")
    else:
        if "modules/balanceo/main.tf" not in bloque:
            problemas.append("AWS0011_RUTA_NO_AUTORIZADA")
        if "RFC-006" not in bloque:
            problemas.append("AWS0011_SIN_REFERENCIA_RFC006")
        if "expired_at:" not in bloque:
            problemas.append("AWS0011_SIN_EXPIRED_AT")

    for gate in sorted((raiz / "sdd" / "_control" / "H08").glob(GATE_GLOB)):
        contenido = gate.read_text(encoding="utf-8", errors="replace")
        if any(marca in contenido for marca in MARCADORES_GATE_APROBADO):
            problemas.append(f"GATE_DESPLIEGUE_AWS_PRESENTE {gate.name}")
    return problemas


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description="Vigencia de la excepción RFC-006 (I-080)")
    parser.add_argument("--raiz", type=Path, default=Path(__file__).resolve().parents[1])
    parser.add_argument("--fecha", type=str, default=None, help="fecha de referencia YYYY-MM-DD (pruebas)")
    args = parser.parse_args(argv)
    try:
        hoy = dt.date.fromisoformat(args.fecha) if args.fecha else dt.date.today()
    except ValueError:
        print(f"RFC006_VIGENCIA_FAIL: fecha invalida {args.fecha}")
        return 2
    problemas = validar(args.raiz.resolve(), hoy)
    if problemas:
        print("RFC006_VIGENCIA_FAIL")
        for problema in problemas:
            print(" -", problema)
        return 1
    print(f"RFC006_VIGENCIA_PASS: excepcion AWS-0011 vigente y aprobada (referencia {hoy})")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
