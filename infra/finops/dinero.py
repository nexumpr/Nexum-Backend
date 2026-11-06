"""Aritmética monetaria exacta para FinOps I-000.

Reglas (constitution R-030, spec I-000 INF-000-04):
- Nunca se usa ``float`` para importes; solo ``Decimal`` desde ``str``/``int``.
- Los totales se cuantizan a 4 decimales de USD (0.0001) con HALF_UP.
- Un precio ``None`` significa DESCONOCIDO; jamás se interpreta como cero.
"""

from __future__ import annotations

from decimal import ROUND_HALF_UP, Decimal, InvalidOperation

CENT = Decimal("0.01")
DIEZ_MIL = Decimal("0.0001")
MICRO = Decimal("0.000001")

TIPOS_NUMERICOS_PERMITIDOS = (Decimal, int, str)


class ImporteInvalido(ValueError):
    """Importe no representable de forma exacta."""


def dec(valor: Decimal | int | str) -> Decimal:
    """Convierte a ``Decimal`` exacto; rechaza ``float`` y ``bool``."""
    if isinstance(valor, bool):
        raise ImporteInvalido("booleano no es un importe")
    if isinstance(valor, float):
        raise ImporteInvalido(
            "punto flotante prohibido en importes monetarios; use str o Decimal"
        )
    if isinstance(valor, Decimal):
        return valor
    if isinstance(valor, int):
        return Decimal(valor)
    if isinstance(valor, str):
        try:
            return Decimal(valor.strip())
        except (InvalidOperation, AttributeError) as exc:
            raise ImporteInvalido(f"importe no numerico: {valor!r}") from exc
    raise ImporteInvalido(f"tipo no soportado: {type(valor).__name__}")


def cuantizar(valor: Decimal | int | str, quantum: Decimal = DIEZ_MIL) -> Decimal:
    """Redondea HALF_UP al quantum indicado (por defecto 0.0001 USD)."""
    return dec(valor).quantize(quantum, rounding=ROUND_HALF_UP)


def multiplicar(cantidad: Decimal | int | str, precio: Decimal | int | str) -> Decimal:
    """cantidad x precio con cuantización de totales (4 decimales)."""
    return cuantizar(dec(cantidad) * dec(precio))


def sumar(valores) -> Decimal:
    """Suma exacta de importes ya cuantizados."""
    total = Decimal("0")
    for valor in valores:
        total += dec(valor)
    return cuantizar(total)


def a_centavos(valor: Decimal | int | str) -> int:
    """Convierte a centavos enteros (para pruebas y comparaciones)."""
    return int(cuantizar(valor, CENT) * 100)


def desde_centavos(centavos: int) -> Decimal:
    """Reconstruye un importe exacto desde centavos."""
    return cuantizar(Decimal(int(centavos)) / Decimal(100), CENT)


def formatear(valor: Decimal | int | str, decimales: int = 4) -> str:
    """Formato estable con separador decimal punto, sin separador de miles."""
    quantum = Decimal(1).scaleb(-decimales)
    return f"{cuantizar(valor, quantum):.{decimales}f}"


def maximo_cero(valor: Decimal | int | str) -> Decimal:
    """max(0, valor) usado en el cálculo de cobro neto."""
    resultado = dec(valor)
    return resultado if resultado > 0 else Decimal("0.0000")
