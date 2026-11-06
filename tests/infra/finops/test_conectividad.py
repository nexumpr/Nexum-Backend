"""Pruebas de conectividad privada sin NAT (I-000, revisión 5).

Verifican que los gateway endpoints sólo cubren S3/DynamoDB, que el resto de
dependencias exige interface endpoints con costo por AZ, y que la recotización
completa deja el presupuesto sin cobertura.
"""

from __future__ import annotations

import json
import sys
import unittest
from decimal import Decimal
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(RAIZ / "infra" / "finops"))

import conectividad  # noqa: E402
import dinero  # noqa: E402

PRECIOS = RAIZ / "infra" / "finops" / "precios_us-east-2.json"


class Dependencias(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.catalogo = json.loads(PRECIOS.read_text(encoding="utf-8"))
        cls.analisis = conectividad.evaluar(cls.catalogo)

    def test_gateway_endpoints_solo_cubren_s3_y_dynamodb(self) -> None:
        cubiertas = {
            d["dependencia"]
            for d in self.analisis["sin_red"]
            if "GATEWAY_ENDPOINT" in d["mecanismo"]
        }
        self.assertEqual(cubiertas, {"DynamoDB", "S3 (artefactos, respaldos, terraform state)"})

    def test_solo_logs_es_permanente(self) -> None:
        self.assertEqual(conectividad.PERMANENTES, ("logs",))

    def test_auxiliares_son_temporales(self) -> None:
        self.assertEqual(
            set(conectividad.TEMPORALES), {"secretsmanager", "ssm", "ssmmessages"}
        )

    def test_monitoring_y_ec2messages_no_se_presupuestan(self) -> None:
        modos = {s["servicio"]: s["modo"] for s in conectividad.SERVICIOS}
        self.assertEqual(modos["monitoring"], "NO_REQUERIDO")
        self.assertEqual(modos["ec2messages"], "NO_REQUERIDO")

    def test_imds_y_ntp_sin_egreso(self) -> None:
        sin_red = {d["dependencia"] for d in self.analisis["sin_red"]}
        self.assertIn("Credenciales de instancia (IAM role)", sin_red)
        self.assertIn("Sincronización horaria", sin_red)

    def test_gateway_endpoints_no_cuestan(self) -> None:
        for dependencia in self.analisis["sin_red"]:
            self.assertEqual(dependencia["costo"], "0")
            self.assertTrue(dependencia["fuente"].startswith("https://"))


class CosteDeEndpoints(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.catalogo = json.loads(PRECIOS.read_text(encoding="utf-8"))

    def test_precio_verificado(self) -> None:
        self.assertEqual(conectividad.precio_endpoint(self.catalogo), Decimal("0.01"))

    def test_un_permanente_dos_az(self) -> None:
        coste = conectividad.costo_servicios(("logs",), 1080, self.catalogo)
        self.assertEqual(coste["enis"], 2)
        self.assertEqual(coste["costo_usd"], "21.6000")

    def test_tres_temporales_en_ventanas(self) -> None:
        coste = conectividad.costo_servicios(
            ("secretsmanager", "ssm", "ssmmessages"), 24, self.catalogo
        )
        self.assertEqual(coste["enis"], 6)
        self.assertEqual(coste["costo_usd"], "1.4400")

    def test_catalogo_sin_verificar_falla(self) -> None:
        catalogo = json.loads(json.dumps(self.catalogo))
        catalogo["items"]["VPC_INTERFACE_ENDPOINT_HORA"]["estado"] = "DOC_ONLY"
        with self.assertRaises(ValueError):
            conectividad.precio_endpoint(catalogo)


class Recotizacion(unittest.TestCase):
    def test_diseno_e_cubierto(self) -> None:
        recoste = conectividad.recotizar("79.0430", "120")
        self.assertEqual(recoste["costo_conectividad_usd"], "23.7400")
        self.assertEqual(recoste["bruto_total_usd"], "102.7830")
        self.assertEqual(recoste["creditos_requeridos_usd"], "113.0613")
        self.assertTrue(recoste["cubierto"])
        self.assertEqual(recoste["importe_sin_cobertura_usd"], "0.0000")
        self.assertEqual(recoste["sobrante_usd"], "6.9387")

    def test_dos_permanentes_no_caben(self) -> None:
        dos = conectividad.costo_servicios(("logs", "secretsmanager"), 1080, None)
        self.assertEqual(dos["costo_usd"], "43.2000")
        base = Decimal("79.0430") + Decimal("43.2000")
        self.assertGreater(base * Decimal("1.10"), Decimal("120"))

    def test_escenarios(self) -> None:
        e = conectividad.escenarios()
        self.assertEqual(e["A_todos_permanentes"]["costo_total_usd"], "86.4000")
        self.assertEqual(e["B_solo_operacion"]["costo_total_usd"], "21.6000")
        self.assertEqual(e["C_operacion_mas_ventanas"]["costo_total_usd"], "23.0400")
        self.assertEqual(e["E_optimizado"]["costo_total_usd"], "23.0400")


if __name__ == "__main__":
    unittest.main()
