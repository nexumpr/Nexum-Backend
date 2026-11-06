"""Pruebas del soporte monetario exacto (sin punto flotante)."""

from __future__ import annotations

import sys
import unittest
from decimal import Decimal
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[3] / "infra" / "finops"))

import dinero  # noqa: E402


class DineroExacto(unittest.TestCase):
    def test_rechaza_float(self) -> None:
        with self.assertRaises(dinero.ImporteInvalido):
            dinero.dec(0.1)
        with self.assertRaises(dinero.ImporteInvalido):
            dinero.multiplicar("2", 0.5)

    def test_suma_decimal_sin_error_binario(self) -> None:
        self.assertEqual(dinero.sumar(["0.1", "0.2"]), Decimal("0.3000"))
        self.assertNotEqual(0.1 + 0.2, 0.3)

    def test_multiplicacion_cuantizada_half_up(self) -> None:
        self.assertEqual(dinero.multiplicar("2160", "0.0084"), Decimal("18.1440"))
        self.assertEqual(dinero.cuantizar("0.00005"), Decimal("0.0001"))
        self.assertEqual(dinero.cuantizar("0.00004"), Decimal("0.0000"))

    def test_centavos_ida_y_vuelta(self) -> None:
        self.assertEqual(dinero.a_centavos("18.1440"), 1814)
        self.assertEqual(dinero.desde_centavos(1814), Decimal("18.14"))

    def test_formato_estable(self) -> None:
        self.assertEqual(dinero.formatear("84.058"), "84.0580")
        self.assertEqual(dinero.formatear("0"), "0.0000")

    def test_maximo_cero(self) -> None:
        self.assertEqual(dinero.maximo_cero("-5"), Decimal("0.0000"))
        self.assertEqual(dinero.maximo_cero("7"), Decimal("7"))


if __name__ == "__main__":
    unittest.main()
