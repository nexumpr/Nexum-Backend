"""Pruebas del modelo determinista de costos a 45 días (I-000)."""

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

    def test_reproduce_total_v3_documentado(self) -> None:
        """El recomputo de las partidas V3 debe dar exactamente 84.0580 USD."""

        escenario = self.resumen["escenarios"]["C_CLOUDFRONT_VPC_ORIGIN_PRIVADO"]
        self.assertEqual(escenario["total_bruto_45d_usd"], "84.0580")

    def test_total_v3_coincide_con_su_propio_json(self) -> None:
        v3 = json.loads(RUTA_V3.read_text(encoding="utf-8"))
        suma = dinero.sumar([item["total_45d_usd"] for item in v3["items"]])
        self.assertEqual(dinero.formatear(suma), dinero.formatear(v3["estimado_total_45d_usd"]))

    def test_ventana_de_45_dias(self) -> None:
        self.assertEqual(escenarios.DIAS_VENTANA, 45)
        self.assertEqual(escenarios.HORAS_VENTANA, 1080)
        self.assertEqual(
            self.resumen["escenarios"]["C_CLOUDFRONT_VPC_ORIGIN_PRIVADO"]["horas_encendido"], "1080"
        )

    def test_topologia_c_sin_ipv4_ni_route53(self) -> None:
        escenario = self.resumen["escenarios"]["C_CLOUDFRONT_VPC_ORIGIN_PRIVADO"]
        con_cantidad = {
            p["clave"]: dinero.dec(p["cantidad"]) for p in escenario["partidas"] if dinero.dec(p["cantidad"]) > 0
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

    def test_escenario_a_incluye_ipv4_y_no_es_techo(self) -> None:
        a = self.resumen["escenarios"]["A_24X7_ALB_PUBLICO"]
        self.assertTrue(a["cumple_24x7"])
        claves = {p["clave"]: dinero.dec(p["cantidad"]) for p in a["partidas"]}
        self.assertEqual(claves["IPV4_EC2"], Decimal("2160"))
        self.assertTrue(any("piso, no un techo" in c for c in a["condiciones"]))

    def test_precio_desconocido_no_es_cero(self) -> None:
        catalogo = json.loads(json.dumps(self.catalogo))
        catalogo["items"]["ALB_HORA"]["precio_usd"] = None
        catalogo["items"]["ALB_HORA"]["estado"] = "UNKNOWN"
        resultado = modelo_costos.calcular_escenario(
            escenarios.escenario_c_cloudfront_privado(), catalogo
        )
        self.assertIsNone(resultado["total_bruto_45d_usd"])
        self.assertIn("ALB_FIJO: precio UNKNOWN", resultado["partidas_sin_precio"])
        alb = next(p for p in resultado["partidas"] if p["clave"] == "ALB_FIJO")
        self.assertIsNone(alb["subtotal_usd"])

    def test_sku_ausente_no_es_cero(self) -> None:
        catalogo = json.loads(json.dumps(self.catalogo))
        del catalogo["items"]["ALB_HORA"]
        resultado = modelo_costos.calcular_escenario(
            escenarios.escenario_c_cloudfront_privado(), catalogo
        )
        self.assertIsNone(resultado["total_bruto_45d_usd"])
        self.assertIn("ALB_FIJO: SKU ausente", resultado["partidas_sin_precio"])

    def test_creditos_separados_del_bruto(self) -> None:
        fin = modelo_costos.calcular_creditos("84.0580", "120")
        self.assertEqual(fin["cobro_neto_usd"], "0.0000")
        self.assertTrue(fin["creditos_cubren_bruto"])
        self.assertEqual(fin["impuestos"], "UNKNOWN")
        self.assertEqual(fin["desembolso_personal_usd"], "0.0000")

    def test_creditos_cero_deja_bruto_como_neto(self) -> None:
        fin = modelo_costos.calcular_creditos("84.0580", "0")
        self.assertEqual(fin["cobro_neto_usd"], "84.0580")
        self.assertFalse(fin["creditos_cubren_bruto"])

    def test_creditos_parciales(self) -> None:
        fin = modelo_costos.calcular_creditos("84.0580", "50")
        self.assertEqual(fin["cobro_neto_usd"], "34.0580")
        self.assertFalse(fin["creditos_cubren_bruto"])

    def test_salida_determinista(self) -> None:
        primera = json.dumps(modelo_costos.calcular_todo(self.catalogo, "0"), sort_keys=True)
        segunda = json.dumps(modelo_costos.calcular_todo(self.catalogo, "0"), sort_keys=True)
        self.assertEqual(primera, segunda)

    def test_estados_de_evidencia_contabilizados(self) -> None:
        escenario = self.resumen["escenarios"]["C_CLOUDFRONT_VPC_ORIGIN_PRIVADO"]
        conteo = escenario["grados_evidencia"]
        self.assertEqual(sum(conteo.values()), len(escenario["partidas"]))
        self.assertEqual(conteo.get("VERIFIED_AWS", 0), 0, "sin consultas AWS no puede haber precios verificados")

    def test_residual_con_runtime_apagado(self) -> None:
        escenario = self.resumen["escenarios"]["C_CLOUDFRONT_VPC_ORIGIN_PRIVADO"]
        self.assertEqual(escenario["residual_apagado_usd"], "6.8440")
        self.assertLess(
            dinero.dec(escenario["residual_apagado_usd"]), dinero.dec(escenario["total_bruto_45d_usd"])
        )
        self.assertIn("EBS_GP3", escenario["partidas_residuales"])
        self.assertNotIn("EC2_GO", escenario["partidas_residuales"])
        self.assertNotIn("CI_CD", escenario["partidas_residuales"])

    def test_residual_con_precio_desconocido_no_es_cero(self) -> None:
        catalogo = json.loads(json.dumps(self.catalogo))
        catalogo["items"]["EBS_GP3_GB_MES"]["precio_usd"] = None
        catalogo["items"]["EBS_GP3_GB_MES"]["estado"] = "UNKNOWN"
        resultado = modelo_costos.calcular_escenario(
            escenarios.escenario_c_cloudfront_privado(), catalogo
        )
        self.assertIsNone(resultado["residual_apagado_usd"])
        self.assertIn("DESCONOCIDO:EBS_GP3", resultado["partidas_residuales"])

    def test_catalogo_rechaza_estado_invalido(self) -> None:
        catalogo = json.loads(json.dumps(self.catalogo))
        catalogo["items"]["ALB_HORA"]["estado"] = "INVENTADO"
        import tempfile

        with tempfile.NamedTemporaryFile("w", suffix=".json", delete=False, encoding="utf-8") as fh:
            json.dump(catalogo, fh)
            ruta = Path(fh.name)
        try:
            with self.assertRaises(ValueError):
                modelo_costos.cargar_precios(ruta)
        finally:
            ruta.unlink(missing_ok=True)


if __name__ == "__main__":
    unittest.main()
