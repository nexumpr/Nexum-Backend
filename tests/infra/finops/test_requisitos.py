"""Pruebas de los requisitos de topología (Corrección D).

Un JSON de diseño con ``"real": true`` no acredita infraestructura desplegada:
se distingue ``DESIGN_VALIDATED`` de ``AWS_DEPLOYMENT_VERIFIED``.
"""

from __future__ import annotations

import json
import sys
import tempfile
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(RAIZ / "infra" / "finops"))

import requisitos  # noqa: E402

TOPOLOGIA = RAIZ / "infra" / "finops" / "evidencia" / "topologia_c_2026-11-06.json"
EVIDENCIA = RAIZ / "infra" / "finops" / "evidencia"


class RequisitosTopologia(unittest.TestCase):
    def setUp(self) -> None:
        self.topologia = json.loads(TOPOLOGIA.read_text(encoding="utf-8"))

    def test_diseno_actual_es_valido(self) -> None:
        self.assertEqual(requisitos.validar_dos_instancias(self.topologia), [])
        self.assertEqual(requisitos.validar_conectividad_privada(self.topologia), [])

    def test_estado_de_diseno_explicito(self) -> None:
        resultado = requisitos.validar(self.topologia)
        self.assertEqual(resultado["estado_diseno"], "DESIGN_VALIDATED")
        self.assertTrue(resultado["diseno_valido"])

    def test_una_sola_instancia_falla(self) -> None:
        self.topologia["instancias"] = self.topologia["instancias"][:1]
        violaciones = requisitos.validar_dos_instancias(self.topologia)
        self.assertTrue(any("exactamente 2" in v for v in violaciones))

    def test_instancias_en_misma_az_falla(self) -> None:
        self.topologia["instancias"][1]["az"] = self.topologia["instancias"][0]["az"]
        self.assertIn(
            "LAS_DOS_INSTANCIAS_COMPARTEN_AZ",
            requisitos.validar_dos_instancias(self.topologia),
        )

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
        self.assertIn(
            "BALANCEADOR_NO_DECLARADO_REAL",
            requisitos.validar_dos_instancias(self.topologia),
        )

    def test_internet_gateway_requerido_por_vpc_origin(self) -> None:
        self.topologia["red"]["internet_gateway"] = False
        violaciones = requisitos.validar_conectividad_privada(self.topologia)
        self.assertTrue(any("INTERNET_GATEWAY_AUSENTE" in v for v in violaciones))

    def test_rutas_publicas_prohibidas(self) -> None:
        self.topologia["red"]["rutas_publicas"] = True
        violaciones = requisitos.validar_conectividad_privada(self.topologia)
        self.assertTrue(any("RUTAS_PUBLICAS_PRESENTES" in v for v in violaciones))

    def test_dos_instancias_requieren_cuatro_vcpu(self) -> None:
        capacidad = self.topologia["capacidad"]
        self.assertEqual(capacidad["vcpu_por_instancia"], 2)
        self.assertEqual(capacidad["vcpu_totales_requeridos"], 4)
        self.assertGreaterEqual(
            capacidad["cuota_vcpu_on_demand_observada"],
            capacidad["vcpu_totales_requeridos"],
        )

    def test_nat_presente_falla(self) -> None:
        self.topologia["red"]["nat_gateways"] = 1
        self.assertIn(
            "NAT_PRESENTE: el diseño debe evitar NAT",
            requisitos.validar_conectividad_privada(self.topologia),
        )

    def test_faltan_gateway_endpoints(self) -> None:
        self.topologia["red"]["gateway_endpoints"] = ["s3"]
        violaciones = requisitos.validar_conectividad_privada(self.topologia)
        self.assertTrue(any("GATEWAY_ENDPOINTS_FALTANTES" in v for v in violaciones))

    def test_ipv4_publica_falla(self) -> None:
        self.topologia["red"]["ipv4_publicas"] = 2
        self.assertIn(
            "IPV4_PUBLICAS: el diseño C no debe asignar IPv4 pública",
            requisitos.validar_conectividad_privada(self.topologia),
        )

    def test_tls_navegador_insuficiente(self) -> None:
        self.topologia["tls"]["navegador_a_cloudfront"] = "http"
        self.assertIn("TLS_NAVEGADOR_INSUFICIENTE", requisitos.validar_tls(self.topologia))


class DisenoNoEsDespliegue(unittest.TestCase):
    def setUp(self) -> None:
        self.topologia = json.loads(TOPOLOGIA.read_text(encoding="utf-8"))

    def test_real_true_no_acredita_despliegue(self) -> None:
        self.assertTrue(all(i["detras_de_balanceador"] for i in self.topologia["instancias"]))
        resultado = requisitos.validar(self.topologia)
        self.assertEqual(resultado["estado_despliegue"], "NOT_RUN")
        self.assertEqual(resultado["verificacion_runtime"], "NOT_RUN")
        self.assertIn("no acredita", resultado["nota"])

    def test_autocertificacion_de_runtime_se_rechaza(self) -> None:
        self.topologia["estado_verificacion"]["runtime"] = "AWS_DEPLOYMENT_VERIFIED"
        resultado = requisitos.validar(self.topologia)
        self.assertFalse(resultado["diseno_valido"])
        self.assertEqual(resultado["estado_diseno"], "DESIGN_INVALID")
        self.assertTrue(
            any("AUTOCERTIFICAR" in p for p in resultado["autocertificacion"])
        )

    def test_declarar_recursos_creados_se_rechaza(self) -> None:
        self.topologia["estado_verificacion"]["recursos_creados"] = True
        resultado = requisitos.validar(self.topologia)
        self.assertFalse(resultado["diseno_valido"])

    def test_sin_evidencia_de_runtime_es_not_run(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            resultado = requisitos.verificar_despliegue(Path(tmp))
        self.assertEqual(resultado["estado"], "NOT_RUN")
        self.assertIsNone(resultado["evidencia"])

    def _escribir_evidencia(self, directorio: Path, contenido: dict) -> None:
        (directorio / "despliegue_runtime_2026-11-06.json").write_text(
            json.dumps(contenido), encoding="utf-8"
        )

    def _evidencia_completa(self) -> dict:
        return {
            "origen": "aws_ec2_describe_instances",
            "estado": "VERIFIED_AWS",
            "verificado_por_humano": True,
            "huella_sha256": "a" * 64,
            "instancias": [
                {"id": "i-0abc1234567890def", "estado": "running", "az": "us-east-2a"},
                {"id": "i-0fedcba9876543210", "estado": "running", "az": "us-east-2b"},
            ],
        }

    def _control(self, directorio: Path, contenido: str) -> None:
        (directorio / requisitos.NOMBRE_APROBACION).write_text(contenido, encoding="utf-8")

    def test_json_fabricado_con_humano_true_no_acredita(self) -> None:
        """Un JSON escrito a mano no puede autocertificar el despliegue."""

        with tempfile.TemporaryDirectory() as tmp:
            dir_ev = Path(tmp) / "ev"
            dir_ctl = Path(tmp) / "ctl"
            dir_ev.mkdir()
            dir_ctl.mkdir()
            self._escribir_evidencia(dir_ev, self._evidencia_completa())
            resultado = requisitos.verificar_despliegue(dir_ev, dir_ctl)
        self.assertEqual(resultado["estado"], "NOT_RUN")
        self.assertTrue(any("APROBACION_DESPLIEGUE_VERIFICADO" in d for d in resultado["detalle"]))

    def test_ids_sin_formato_aws_no_acreditan(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            dir_ev = Path(tmp) / "ev"
            dir_ctl = Path(tmp) / "ctl"
            dir_ev.mkdir()
            dir_ctl.mkdir()
            datos = self._evidencia_completa()
            datos["instancias"] = [
                {"id": "i-1", "estado": "running", "az": "us-east-2a"},
                {"id": "i-2", "estado": "running", "az": "us-east-2b"},
            ]
            self._escribir_evidencia(dir_ev, datos)
            self._control(
                dir_ctl,
                f"{requisitos.FRASE_APROBACION_DESPLEGUE}\nver despliegue_runtime_2026-11-06.json",
            )
            resultado = requisitos.verificar_despliegue(dir_ev, dir_ctl)
        self.assertEqual(resultado["estado"], "NOT_RUN")

    def test_sin_huella_no_acredita(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            dir_ev = Path(tmp) / "ev"
            dir_ctl = Path(tmp) / "ctl"
            dir_ev.mkdir()
            dir_ctl.mkdir()
            datos = self._evidencia_completa()
            del datos["huella_sha256"]
            self._escribir_evidencia(dir_ev, datos)
            self._control(
                dir_ctl,
                f"{requisitos.FRASE_APROBACION_DESPLEGUE}\ndespliegue_runtime_2026-11-06.json",
            )
            resultado = requisitos.verificar_despliegue(dir_ev, dir_ctl)
        self.assertEqual(resultado["estado"], "NOT_RUN")

    def test_misma_az_no_acredita(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            dir_ev = Path(tmp) / "ev"
            dir_ctl = Path(tmp) / "ctl"
            dir_ev.mkdir()
            dir_ctl.mkdir()
            datos = self._evidencia_completa()
            datos["instancias"][1]["az"] = "us-east-2a"
            self._escribir_evidencia(dir_ev, datos)
            self._control(
                dir_ctl,
                f"{requisitos.FRASE_APROBACION_DESPLEGUE}\ndespliegue_runtime_2026-11-06.json",
            )
            resultado = requisitos.verificar_despliegue(dir_ev, dir_ctl)
        self.assertEqual(resultado["estado"], "NOT_RUN")

    def test_evidencia_completa_con_control_acredita(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            dir_ev = Path(tmp) / "ev"
            dir_ctl = Path(tmp) / "ctl"
            dir_ev.mkdir()
            dir_ctl.mkdir()
            self._escribir_evidencia(dir_ev, self._evidencia_completa())
            self._control(
                dir_ctl,
                f"{requisitos.FRASE_APROBACION_DESPLEGUE}\nEvidencia: despliegue_runtime_2026-11-06.json",
            )
            resultado = requisitos.verificar_despliegue(dir_ev, dir_ctl)
        self.assertEqual(resultado["estado"], "AWS_DEPLOYMENT_VERIFIED")
        self.assertEqual(resultado["evidencia"], "despliegue_runtime_2026-11-06.json")

    def test_evidencia_real_del_repositorio_no_acredita_despliegue(self) -> None:
        resultado = requisitos.verificar_despliegue(EVIDENCIA)
        self.assertEqual(resultado["estado"], "NOT_RUN")


if __name__ == "__main__":
    unittest.main()
