"""FinOps del egreso FCM/OAuth por NAT Gateway 1 AZ.

Las pruebas recalculan la tabla desde el catálogo `precios_us-east-2.json`:
no aceptan totales hardcodeados sin comprobar las partidas, los estados de
evidencia y las sensibilidades. Los precios NAT son `SUPUESTO` y se declaran
`NOT_VERIFIED`; la prueba lo exige explícitamente para que nadie los presente
como verificados.
"""

from __future__ import annotations

import sys
import unittest
from decimal import Decimal
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(RAIZ / "infra" / "finops"))

import dinero  # noqa: E402
import egreso_fcm  # noqa: E402


class SupuestosDeTrafico(unittest.TestCase):
    def test_trafico_base_8_y_45_dias(self) -> None:
        self.assertEqual(
            egreso_fcm.trafico_gb(8, 1),
            Decimal("0.030517578125"),
        )
        self.assertEqual(
            egreso_fcm.trafico_gb(45, 1),
            Decimal("0.171661376953125"),
        )

    def test_escenario_alto_es_diez_veces(self) -> None:
        base = egreso_fcm.trafico_gb(8, 1)
        alto = egreso_fcm.trafico_gb(8, 10)
        self.assertEqual(alto, base * Decimal(10))


class TablaDesdeCatalogo(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.catalogo = egreso_fcm.cargar_catalogo()
        cls.tabla = egreso_fcm.calcular_tabla(
            cls.catalogo,
            creditos_verificados_usd=egreso_fcm.CREDITOS_VERIFICADOS_2026_11_24,
        )

    def _escenario(self, clave: str) -> dict:
        return self.tabla["escenarios"][clave]

    def test_cada_partida_usa_el_precio_del_catalogo(self) -> None:
        for clave in ("base_8d", "base_45d", "alto_8d", "alto_45d"):
            escenario = self._escenario(clave)
            for partida in escenario["partidas"]:
                item = self.catalogo["items"][partida["sku"]]
                self.assertEqual(
                    partida["estado_evidencia"],
                    item["estado"],
                    f"{clave}:{partida['sku']}",
                )
                self.assertEqual(
                    partida["precio_unitario_usd"],
                    dinero.dec(item["precio_usd"]),
                    f"{clave}:{partida['sku']}",
                )
                self.assertEqual(
                    partida["subtotal_usd"],
                    dinero.multiplicar(partida["cantidad"], item["precio_usd"]),
                    f"{clave}:{partida['sku']}",
                )

    def test_suma_de_partidas_igual_al_total(self) -> None:
        for clave in ("base_8d", "base_45d", "alto_8d", "alto_45d"):
            escenario = self._escenario(clave)
            suma = dinero.sumar([p["subtotal_usd"] for p in escenario["partidas"]])
            self.assertEqual(suma, escenario["total_modelado_usd"], clave)

    def test_horas_de_cada_horizonte(self) -> None:
        self.assertEqual(self._escenario("base_8d")["horas"], Decimal(192))
        self.assertEqual(self._escenario("base_45d")["horas"], Decimal(1080))

    def test_limite_20_es_solo_incremental_de_8_dias(self) -> None:
        self.assertTrue(self.tabla["limite_20_8d_incremental_cumplido"])
        self.assertLessEqual(
            self._escenario("base_8d")["total_modelado_usd"],
            Decimal("20.00"),
        )
        self.assertGreater(
            self._escenario("base_45d")["total_modelado_usd"],
            Decimal("20.00"),
        )

    def test_eip_es_condicional_y_su_ausencia_reduce_el_total(self) -> None:
        base = self._escenario("base_8d")["total_modelado_usd"]
        sin_eip = self._escenario("base_8d_sin_eip")["total_modelado_usd"]
        self.assertEqual(base - sin_eip, Decimal("0.9600"))
        self.assertEqual(
            self._escenario("base_8d_sin_eip")["partidas"][1]["subtotal_usd"],
            Decimal("0.0000"),
        )

    def test_interaz_es_condicional(self) -> None:
        base = self._escenario("base_8d")["total_modelado_usd"]
        sin_interaz = self._escenario("base_8d_sin_interaz")["total_modelado_usd"]
        self.assertEqual(base - sin_interaz, Decimal("0.0003"))

    def test_escenario_alto_solo_mueve_trafico(self) -> None:
        base = self._escenario("base_8d")["total_modelado_usd"]
        alto = self._escenario("alto_8d")["total_modelado_usd"]
        self.assertGreater(alto, base)
        self.assertEqual(
            self._escenario("alto_8d")["trafico_gb"],
            self._escenario("base_8d")["trafico_gb"] * Decimal(10),
        )


class InfraExistenteMargenYCreditos(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.tabla = egreso_fcm.calcular_tabla(
            creditos_verificados_usd=egreso_fcm.CREDITOS_VERIFICADOS_2026_11_24,
        )

    def test_referencia_existente_y_prorrateo(self) -> None:
        infra = self.tabla["infra_existente"]
        self.assertEqual(infra["i000_45d_usd"], Decimal("102.7830"))
        self.assertEqual(infra["ses_endpoint_2az_45d_usd"], Decimal("21.6000"))
        self.assertEqual(infra["referencia_45d_usd"], Decimal("124.3830"))
        self.assertEqual(infra["prorrateo_8d_usd"], Decimal("22.1125"))

    def test_totales_del_entorno_y_margen(self) -> None:
        base_8 = self.tabla["escenarios"]["base_8d"]["total_modelado_usd"]
        base_45 = self.tabla["escenarios"]["base_45d"]["total_modelado_usd"]
        self.assertEqual(self.tabla["total_entorno_8d_usd"], dinero.sumar([Decimal("22.1125"), base_8]))
        self.assertEqual(self.tabla["total_entorno_45d_usd"], dinero.sumar([Decimal("124.3830"), base_45]))
        self.assertEqual(
            self.tabla["margen_10_8d_usd"],
            dinero.cuantizar(self.tabla["total_entorno_8d_usd"] * Decimal("1.10")),
        )
        self.assertEqual(
            self.tabla["margen_10_45d_usd"],
            dinero.cuantizar(self.tabla["total_entorno_45d_usd"] * Decimal("1.10")),
        )

    def test_creditos_son_contexto_no_autorizacion(self) -> None:
        self.assertTrue(self.tabla["creditos_no_son_autorizacion"])
        self.assertFalse(self.tabla["total_45d_cubierto_por_creditos"])
        self.assertTrue(self.tabla["total_8d_cubierto_por_creditos"])
        self.assertEqual(
            self.tabla["creditos_verificados_usd"],
            Decimal("130.491975"),
        )


class PreciosNoVerificados(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.catalogo = egreso_fcm.cargar_catalogo()

    def test_el_sku_nat_original_sigue_unknown(self) -> None:
        item = self.catalogo["items"]["NAT_GATEWAY_HORA"]
        self.assertEqual(item["estado"], "UNKNOWN")
        self.assertIsNone(item["precio_usd"])

    def test_los_supuestos_nat_estan_marcados(self) -> None:
        for sku in (
            "NAT_GATEWAY_HORA_SUPUESTO",
            "NAT_GATEWAY_PROCESAMIENTO_GB_SUPUESTO",
            "NAT_EGRESO_INTERNET_GB_SUPUESTO",
        ):
            self.assertEqual(self.catalogo["items"][sku]["estado"], "SUPUESTO", sku)

    def test_no_se_presenta_ningun_total_nat_como_verificado(self) -> None:
        tabla = egreso_fcm.calcular_tabla()
        for clave in ("base_8d", "base_45d", "alto_8d", "alto_45d"):
            escenario = tabla["escenarios"][clave]
            self.assertEqual(
                escenario["estado"],
                "MODELADO_CON_SUPUESTOS_NO_VERIFICADOS",
                clave,
            )
            self.assertGreater(escenario["subtotal_supuesto_usd"], Decimal("0"))


if __name__ == "__main__":
    unittest.main()
