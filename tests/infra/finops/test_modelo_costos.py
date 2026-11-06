"""Pruebas del modelo determinista de costos a 45 días (revisión 2).

Cubren la separación exigida por la Corrección B:

- costo bruto **calculado** frente a costo bruto **verificado**;
- ``None`` cuando falta verificar una partida requerida (nunca 0);
- créditos declarados frente a verificados;
- cobertura efectiva, importe sin cobertura y riesgo de cobro personal;
- tramos tarifarios y precios desconocidos.
"""

from __future__ import annotations

import json
import sys
import unittest
from decimal import Decimal
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(RAIZ / "infra" / "finops"))

import dinero  # noqa: E402
import escenarios  # noqa: E402
import modelo_costos  # noqa: E402

RUTA_PRECIOS = RAIZ / "infra" / "finops" / "precios_us-east-2.json"
RUTA_V3 = RAIZ / "sdd" / "02-infraestructura" / "FINOPS_45_DIAS_MODELO_V3.json"


class ModeloCostos(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.catalogo = modelo_costos.cargar_precios(RUTA_PRECIOS)
        cls.resumen = modelo_costos.calcular_todo(cls.catalogo, "0")

    def test_ventana_de_45_dias(self) -> None:
        self.assertEqual(escenarios.DIAS_VENTANA, 45)
        self.assertEqual(escenarios.HORAS_VENTANA, 1080)
        self.assertEqual(
            self.resumen["escenarios"]["C_CLOUDFRONT_VPC_ORIGIN_PRIVADO"]["horas_encendido"],
            "1080",
        )

    def test_aritmetica_del_modelo_v3_se_reproduce(self) -> None:
        """La aritmética histórica V3 sigue siendo reproducible con sus tarifas."""

        v3 = json.loads(RUTA_V3.read_text(encoding="utf-8"))
        suma = dinero.sumar([item["total_45d_usd"] for item in v3["items"]])
        self.assertEqual(
            dinero.formatear(suma), dinero.formatear(v3["estimado_total_45d_usd"])
        )

    def test_total_es_la_suma_de_sus_partidas(self) -> None:
        escenario = self.resumen["escenarios"]["C_CLOUDFRONT_VPC_ORIGIN_PRIVADO"]
        suma = dinero.sumar(
            [p["subtotal_usd"] for p in escenario["partidas"] if p["subtotal_usd"] is not None]
        )
        self.assertEqual(dinero.formatear(suma), escenario["total_conocido_usd"])
        self.assertEqual(escenario["total_bruto_45d_usd"], escenario["total_conocido_usd"])

    def test_topologia_c_sin_ipv4_ni_route53(self) -> None:
        escenario = self.resumen["escenarios"]["C_CLOUDFRONT_VPC_ORIGIN_PRIVADO"]
        con_cantidad = {
            p["clave"]
            for p in escenario["partidas"]
            if dinero.dec(p["cantidad"]) > 0
        }
        self.assertNotIn("IPV4_EC2", con_cantidad)
        self.assertNotIn("IPV4_ALB", con_cantidad)
        self.assertNotIn("ROUTE53", con_cantidad)
        self.assertNotIn("NAT", con_cantidad)
        self.assertIn("GW_ENDPOINTS", con_cantidad)

    def test_escenario_b_no_cumple_24x7(self) -> None:
        b = self.resumen["escenarios"]["B_VENTANAS_8H"]
        self.assertFalse(b["cumple_24x7"])
        self.assertFalse(b["valido_para_demostracion"])
        self.assertEqual(b["horas_encendido"], "360")

    def test_escenario_a_incluye_ipv4_y_es_piso(self) -> None:
        a = self.resumen["escenarios"]["A_24X7_ALB_PUBLICO"]
        claves = {p["clave"]: dinero.dec(p["cantidad"]) for p in a["partidas"]}
        self.assertEqual(claves["IPV4_EC2"], Decimal("2160"))
        self.assertTrue(any("piso, no un techo" in c for c in a["condiciones"]))
        self.assertGreater(
            dinero.dec(a["total_bruto_45d_usd"]),
            dinero.dec(self.resumen["escenarios"]["C_CLOUDFRONT_VPC_ORIGIN_PRIVADO"]["total_bruto_45d_usd"]),
        )


class BrutoVerificado(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.catalogo = modelo_costos.cargar_precios(RUTA_PRECIOS)
        cls.resumen = modelo_costos.calcular_todo(cls.catalogo, "0")

    def test_bruto_verificado_es_none_si_falta_una_partida(self) -> None:
        escenario = self.resumen["escenarios"]["C_CLOUDFRONT_VPC_ORIGIN_PRIVADO"]
        self.assertIsNone(escenario["total_bruto_verificado_45d_usd"])
        self.assertTrue(escenario["partidas_sin_verificar"])
        self.assertIn("CERT_CF", escenario["partidas_sin_verificar"])

    def test_bruto_verificado_existe_con_todo_verificado(self) -> None:
        catalogo = json.loads(json.dumps(self.catalogo))
        for item in catalogo["items"].values():
            if item["estado"] != "UNKNOWN":
                item["estado"] = "VERIFIED_AWS"
        escenario = modelo_costos.calcular_escenario(
            escenarios.escenario_c_cloudfront_privado(), catalogo
        )
        self.assertEqual(escenario["total_bruto_verificado_45d_usd"], escenario["total_bruto_45d_usd"])
        self.assertEqual(escenario["partidas_sin_verificar"], [])

    def test_grados_de_evidencia_contabilizados(self) -> None:
        escenario = self.resumen["escenarios"]["C_CLOUDFRONT_VPC_ORIGIN_PRIVADO"]
        conteo = escenario["grados_evidencia"]
        self.assertEqual(sum(conteo.values()), len(escenario["partidas"]))
        self.assertGreater(conteo.get("VERIFIED_AWS", 0), 0)
        self.assertGreater(conteo.get("DOC_ONLY", 0) + conteo.get("HIPOTESIS", 0), 0)


class PreciosDesconocidos(unittest.TestCase):
    def setUp(self) -> None:
        self.catalogo = modelo_costos.cargar_precios(RUTA_PRECIOS)

    def _con(self, sku: str, precio, estado: str) -> dict:
        catalogo = json.loads(json.dumps(self.catalogo))
        catalogo["items"][sku]["precio_usd"] = precio
        catalogo["items"][sku]["estado"] = estado
        return catalogo

    def test_precio_desconocido_no_es_cero(self) -> None:
        resultado = modelo_costos.calcular_escenario(
            escenarios.escenario_c_cloudfront_privado(), self._con("ALB_HORA", None, "UNKNOWN")
        )
        self.assertIsNone(resultado["total_bruto_45d_usd"])
        self.assertIn("ALB_FIJO: precio UNKNOWN", resultado["partidas_sin_precio"])

    def test_sku_ausente_no_es_cero(self) -> None:
        catalogo = json.loads(json.dumps(self.catalogo))
        del catalogo["items"]["ALB_HORA"]
        resultado = modelo_costos.calcular_escenario(
            escenarios.escenario_c_cloudfront_privado(), catalogo
        )
        self.assertIsNone(resultado["total_bruto_45d_usd"])
        self.assertIn("ALB_FIJO: SKU ausente", resultado["partidas_sin_precio"])

    def test_catalogo_rechaza_estado_invalido(self) -> None:
        catalogo = self._con("ALB_HORA", "0.0225", "INVENTADO")
        import tempfile

        with tempfile.NamedTemporaryFile("w", suffix=".json", delete=False, encoding="utf-8") as fh:
            json.dump(catalogo, fh)
            ruta = Path(fh.name)
        try:
            with self.assertRaises(ValueError):
                modelo_costos.cargar_precios(ruta)
        finally:
            ruta.unlink(missing_ok=True)


class CoberturaDeCreditos(unittest.TestCase):
    def test_cobertura_total(self) -> None:
        fin = modelo_costos.calcular_cobertura("84.0000", "84.0000", "120")
        self.assertEqual(fin["cobertura_creditos"], "TOTAL")
        self.assertEqual(fin["importe_sin_cobertura_usd"], "0.0000")
        self.assertTrue(fin["creditos_cubren_bruto"])

    def test_cobertura_parcial(self) -> None:
        fin = modelo_costos.calcular_cobertura("84.0000", "84.0000", "50")
        self.assertEqual(fin["cobertura_creditos"], "PARCIAL")
        self.assertEqual(fin["importe_sin_cobertura_usd"], "34.0000")
        self.assertFalse(fin["creditos_cubren_bruto"])

    def test_cobertura_ninguna(self) -> None:
        fin = modelo_costos.calcular_cobertura("84.0000", "84.0000", "0")
        self.assertEqual(fin["cobertura_creditos"], "NINGUNA")
        self.assertEqual(fin["importe_sin_cobertura_usd"], "84.0000")

    def test_sin_bruto_verificado_la_cobertura_es_unknown(self) -> None:
        fin = modelo_costos.calcular_cobertura(None, "84.0000", "120")
        self.assertEqual(fin["cobertura_creditos"], "UNKNOWN")
        self.assertIsNone(fin["importe_sin_cobertura_usd"])
        self.assertIsNone(fin["creditos_cubren_bruto"])

    def test_margen_de_seguridad_aumenta_lo_requerido(self) -> None:
        fin = modelo_costos.calcular_cobertura("100.0000", "100.0000", "120", margen_seguridad="0.10")
        self.assertEqual(fin["creditos_requeridos_usd"], "110.0000")
        self.assertEqual(fin["cobertura_creditos"], "TOTAL")
        fin_justo = modelo_costos.calcular_cobertura(
            "100.0000", "100.0000", "105", margen_seguridad="0.10"
        )
        self.assertEqual(fin_justo["cobertura_creditos"], "PARCIAL")

    def test_desembolso_cero_no_elimina_el_riesgo(self) -> None:
        fin = modelo_costos.calcular_cobertura(None, "84.0000", "0")
        self.assertEqual(fin["politica_gasto_personal"], "CERO")
        self.assertEqual(fin["desembolso_personal_autorizado_usd"], "0.0000")
        self.assertTrue(fin["riesgo_cobro_personal"])
        self.assertIn("no elimina", fin["nota_riesgo"])

    def test_declarados_y_verificados_se_informan_por_separado(self) -> None:
        fin = modelo_costos.calcular_cobertura("84.0000", "84.0000", "120", "120")
        self.assertEqual(fin["creditos_declarados_usd"], "120.0000")
        self.assertEqual(fin["creditos_verificados_usd"], "120.0000")

    def test_impuestos_siempre_unknown(self) -> None:
        self.assertEqual(
            modelo_costos.calcular_cobertura("84", "84", "120")["impuestos"], "UNKNOWN"
        )


class Residual(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.catalogo = modelo_costos.cargar_precios(RUTA_PRECIOS)
        cls.resumen = modelo_costos.calcular_todo(cls.catalogo, "0")

    def test_residual_menor_que_total(self) -> None:
        escenario = self.resumen["escenarios"]["C_CLOUDFRONT_VPC_ORIGIN_PRIVADO"]
        self.assertIsNotNone(escenario["residual_apagado_usd"])
        self.assertLess(
            dinero.dec(escenario["residual_apagado_usd"]),
            dinero.dec(escenario["total_bruto_45d_usd"]),
        )

    def test_residual_excluye_lineas_apagables_y_no_aws(self) -> None:
        escenario = self.resumen["escenarios"]["C_CLOUDFRONT_VPC_ORIGIN_PRIVADO"]
        partidas = set(escenario["partidas_residuales"])
        self.assertIn("EBS_GP3", partidas)
        self.assertNotIn("EC2_GO", partidas)
        self.assertNotIn("ALB_FIJO", partidas)
        self.assertNotIn("CI_CD", partidas)

    def test_residual_desconocido_no_es_cero(self) -> None:
        catalogo = json.loads(json.dumps(self.catalogo))
        catalogo["items"]["EBS_GP3_GB_MES"]["precio_usd"] = None
        catalogo["items"]["EBS_GP3_GB_MES"]["estado"] = "UNKNOWN"
        resultado = modelo_costos.calcular_escenario(
            escenarios.escenario_c_cloudfront_privado(), catalogo
        )
        self.assertIsNone(resultado["residual_apagado_usd"])
        self.assertIn("DESCONOCIDO:EBS_GP3", resultado["partidas_residuales"])


class Determinismo(unittest.TestCase):
    def test_salida_reproducible(self) -> None:
        catalogo = modelo_costos.cargar_precios(RUTA_PRECIOS)
        primera = json.dumps(modelo_costos.calcular_todo(catalogo, "0"), sort_keys=True)
        segunda = json.dumps(modelo_costos.calcular_todo(catalogo, "0"), sort_keys=True)
        self.assertEqual(primera, segunda)


if __name__ == "__main__":
    unittest.main()
