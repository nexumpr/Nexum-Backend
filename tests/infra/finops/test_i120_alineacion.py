"""Alineacion FinOps de I-120 con I-000.

Comprueba que el residual de I-120 no contradice el presupuesto de 45 dias ni lo
sustituye, y que nada se presenta como costo cero o costo observado sin evidencia.
"""

from __future__ import annotations

import json
import sys
import unittest
from decimal import Decimal
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(RAIZ / "herramientas"))

import nexum_operacion as op  # noqa: E402

INVENTARIO = RAIZ / "infra" / "terraform" / "operacion" / "inventario.json"
CATALOGO = RAIZ / "infra" / "finops" / "precios_us-east-2.json"
FIN_OPS = RAIZ / "infra" / "finops"
RESIDUAL_DOC = RAIZ / "sdd" / "_control" / "H08" / "INVENTARIO_RESIDUAL.md"

TOTAL_I000_45D = Decimal("102.7830")
MARGEN_I000 = Decimal("113.0613")
CREDITOS_VERIFICADOS = Decimal("120.0000")


class AlineacionI120(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.inv = op.cargar_inventario(INVENTARIO)
        cls.cat = op.cargar_catalogo(CATALOGO)
        cls.res = op.residual(cls.inv, cls.cat)

    def test_residual_cabe_en_el_presupuesto_de_i000(self) -> None:
        self.assertLessEqual(Decimal(self.res["total_45d_usd"]), TOTAL_I000_45D)

    def test_residual_con_margen_no_agota_creditos(self) -> None:
        con_margen = Decimal(self.res["total_45d_usd"]) * Decimal("1.10")
        self.assertLessEqual(con_margen, CREDITOS_VERIFICADOS)
        self.assertGreater(con_margen, Decimal("0"))

    def test_precios_vienen_de_skus_verificados(self) -> None:
        items = self.cat["items"]
        skus = {
            partida["sku"]
            for item in self.inv["items"]
            for partida in item.get("partidas", [])
        }
        desconocidos = {sku for sku in skus if sku not in items}
        self.assertEqual(desconocidos, set(), "SKU no declarados en el catalogo")
        sin_precio = {sku for sku in skus if items[sku].get("precio_usd") is None}
        self.assertEqual(sin_precio, set(), "SKU sin precio verificado no deben usarse")

    def test_solo_se_usan_skus_verificados_o_sin_cargo(self) -> None:
        items = self.cat["items"]
        permitidos = {"VERIFIED_AWS", "DOC_OFICIAL"}
        skus = {
            partida["sku"]
            for item in self.inv["items"]
            for partida in item.get("partidas", [])
        }
        no_verificados = {
            sku: items[sku].get("estado") for sku in skus if items[sku].get("estado") not in permitidos
        }
        self.assertEqual(no_verificados, {}, "Solo precio verificado o sin cargo documentado")

    def test_alarmas_incrementales_de_i110_cuentan(self) -> None:
        # I-000 presupuesto 3 alarmas; I-110 anade 5 hasta 8 (+0.5000 USD/mes).
        item = next(i for i in self.inv["items"] if i["nombre"] == "alarmas-cloudwatch")
        self.assertEqual(item["partidas"][0]["cantidad"], "8")
        self.assertIn("+0.5000", item["motivo"])
        self.assertEqual(
            Decimal(self.res["por_recurso_hora_usd"]["alarmas-cloudwatch"]), Decimal("0.0011")
        )

    def test_documento_no_declara_costo_cero_ni_observado(self) -> None:
        texto = RESIDUAL_DOC.read_text(encoding="utf-8")
        self.assertIn("NOT_VERIFIED", texto)
        self.assertIn("AWS_COST_REAL", texto)
        self.assertNotIn("coste real es 0", texto)
        self.assertNotIn("costo real cero", texto)
        self.assertIn("no lo reemplaza", texto)

    def test_matriz_finops_i000_sigue_siendo_la_autoridad(self) -> None:
        matriz = (FIN_OPS / "MATRIZ_COSTOS_SERVICIOS_I000.md").read_text(encoding="utf-8")
        self.assertIn("45", matriz)
        self.assertIn("USD", matriz)


if __name__ == "__main__":  # pragma: no cover
    unittest.main()
