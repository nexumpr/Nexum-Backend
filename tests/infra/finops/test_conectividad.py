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
        cubiertas = {d["dependencia"] for d in self.analisis["dependencias"] if d["mecanismo"] == "GATEWAY_ENDPOINT"}
        self.assertEqual(cubiertas, {"DynamoDB", "S3 (artefactos, respaldos, terraform state)"})

    def test_secrets_y_logs_requieren_interface_endpoint(self) -> None:
        por_nombre = {d["dependencia"]: d for d in self.analisis["dependencias"]}
        self.assertEqual(por_nombre["Secrets Manager (credenciales JWT, claves)"]["mecanismo"], "INTERFACE_ENDPOINT")
        self.assertEqual(por_nombre["CloudWatch Logs (observabilidad del backend)"]["mecanismo"], "INTERFACE_ENDPOINT")

    def test_imds_y_ntp_no_requieren_egreso(self) -> None:
        sin_red = {d["dependencia"] for d in self.analisis["dependencias"] if not d["requiere_red"]}
        self.assertIn("Credenciales de instancia (IAM role)", sin_red)
        self.assertIn("Sincronización horaria", sin_red)

    def test_todas_las_interface_endpoints_tienen_fuente(self) -> None:
        for dependencia in self.analisis["dependencias"]:
            self.assertTrue(dependencia["fuente"].startswith("https://"), dependencia["dependencia"])


class CosteDeEndpoints(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.catalogo = json.loads(PRECIOS.read_text(encoding="utf-8"))

    def test_precio_verificado(self) -> None:
        self.assertEqual(conectividad.precio_endpoint(self.catalogo), Decimal("0.01"))

    def test_dos_servicios_dos_az(self) -> None:
        coste = conectividad.costo_endpoints(("secretsmanager", "logs"), self.catalogo)
        self.assertEqual(coste["enis_totales"], 4)
        self.assertEqual(coste["costo_45d_usd"], "43.2000")  # 4 ENI x 0.01 x 1080 h

    def test_seis_servicios_cuestan_mas(self) -> None:
        coste = conectividad.costo_endpoints(conectividad.ENDPOINTS_OBLIGATORIOS_SDD, self.catalogo)
        self.assertEqual(coste["enis_totales"], 12)
        self.assertEqual(coste["costo_45d_usd"], "129.6000")

    def test_catalogo_sin_verificar_falla(self) -> None:
        catalogo = json.loads(json.dumps(self.catalogo))
        catalogo["items"]["VPC_INTERFACE_ENDPOINT_HORA"]["estado"] = "DOC_ONLY"
        with self.assertRaises(ValueError):
            conectividad.precio_endpoint(catalogo)


class Recotizacion(unittest.TestCase):
    def test_minimo_sin_ssm_sigue_sin_cobertura(self) -> None:
        recoste = conectividad.recotizar("79.0430", "120", incluir_ssm=False)
        self.assertEqual(recoste["costo_conectividad_usd"], "43.2000")
        self.assertEqual(recoste["bruto_total_usd"], "122.2430")
        self.assertEqual(recoste["creditos_requeridos_usd"], "134.4673")
        self.assertFalse(recoste["cubierto"])
        self.assertEqual(recoste["importe_sin_cobertura_usd"], "14.4673")

    def test_con_ssm_el_deficit_crece(self) -> None:
        minimo = conectividad.recotizar("79.0430", "120", incluir_ssm=False)
        completo = conectividad.recotizar("79.0430", "120", incluir_ssm=True)
        self.assertEqual(completo["costo_conectividad_usd"], "129.6000")
        self.assertGreater(
            dinero.dec(completo["creditos_requeridos_usd"]),
            dinero.dec(minimo["creditos_requeridos_usd"]),
        )
        self.assertFalse(completo["cubierto"])

    def test_sin_conectividad_habria_cobertura(self) -> None:
        base = dinero.dec("79.0430") * Decimal("1.10")
        self.assertLessEqual(base, dinero.dec("120"))


if __name__ == "__main__":
    unittest.main()
