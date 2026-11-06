"""Pruebas de los requisitos de topología: dos instancias y red privada sin NAT."""

from __future__ import annotations

import json
import sys
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(RAIZ / "infra" / "finops"))

import requisitos  # noqa: E402

TOPOLOGIA = RAIZ / "infra" / "finops" / "evidencia" / "topologia_c_2026-11-06.json"


class RequisitosTopologia(unittest.TestCase):
    def setUp(self) -> None:
        self.topologia = json.loads(TOPOLOGIA.read_text(encoding="utf-8"))

    def test_diseno_actual_es_valido(self) -> None:
        self.assertEqual(requisitos.validar_dos_instancias(self.topologia), [])
        self.assertEqual(requisitos.validar_conectividad_privada(self.topologia), [])
        self.assertEqual(requisitos.validar(self.topologia)["dos_instancias"], [])

    def test_una_sola_instancia_falla(self) -> None:
        self.topologia["instancias"] = self.topologia["instancias"][:1]
        violaciones = requisitos.validar_dos_instancias(self.topologia)
        self.assertTrue(any("exactamente 2" in v for v in violaciones))

    def test_instancias_en_misma_az_falla(self) -> None:
        self.topologia["instancias"][1]["az"] = self.topologia["instancias"][0]["az"]
        self.assertIn("LAS_DOS_INSTANCIAS_COMPARTEN_AZ", requisitos.validar_dos_instancias(self.topologia))

    def test_instancia_simulada_falla(self) -> None:
        self.topologia["instancias"][0]["simulado"] = True
        violaciones = requisitos.validar_dos_instancias(self.topologia)
        self.assertTrue(any("simulado" in v for v in violaciones))

    def test_procesos_en_la_misma_maquina_falla(self) -> None:
        self.topologia["instancias"][0]["misma_maquina"] = True
        violaciones = requisitos.validar_dos_instancias(self.topologia)
        self.assertTrue(any("misma_maquina" in v for v in violaciones))

    def test_balanceador_debe_ser_real(self) -> None:
        self.topologia["balanceador"]["real"] = False
        self.assertIn("BALANCEADOR_NO_DECLARADO_REAL", requisitos.validar_dos_instancias(self.topologia))

    def test_nat_presente_falla(self) -> None:
        self.topologia["red"]["nat_gateways"] = 1
        self.assertIn("NAT_PRESENTE: el diseño debe evitar NAT", requisitos.validar_conectividad_privada(self.topologia))

    def test_faltan_gateway_endpoints(self) -> None:
        self.topologia["red"]["gateway_endpoints"] = ["s3"]
        violaciones = requisitos.validar_conectividad_privada(self.topologia)
        self.assertTrue(any("GATEWAY_ENDPOINTS_FALTANTES" in v for v in violaciones))

    def test_ipv4_publica_falla(self) -> None:
        self.topologia["red"]["ipv4_publicas"] = 2
        self.assertIn("IPV4_PUBLICAS: el diseño C no debe asignar IPv4 pública", requisitos.validar_conectividad_privada(self.topologia))

    def test_tls_navegador_insuficiente(self) -> None:
        self.topologia["tls"]["navegador_a_cloudfront"] = "http"
        self.assertIn("TLS_NAVEGADOR_INSUFICIENTE", requisitos.validar_tls(self.topologia))

    def test_resultado_declara_runtime_no_ejecutado(self) -> None:
        resultado = requisitos.validar(self.topologia)
        self.assertEqual(resultado["verificacion_runtime"], "NOT_RUN")


if __name__ == "__main__":
    unittest.main()
