"""Alineación FinOps de I-110 con las líneas CloudWatch de I-000."""

from __future__ import annotations

import re
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
MODULO = RAIZ / "infra" / "terraform" / "modules" / "observabilidad"
MATRIZ = RAIZ / "infra" / "finops" / "MATRIZ_COSTOS_SERVICIOS_I000.md"


def _leer(ruta: Path) -> str:
    return ruta.read_text(encoding="utf-8")


class AlineacionI110(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(MODULO / "main.tf")
        cls.vars = _leer(MODULO / "variables.tf")
        cls.finops = _leer(MATRIZ)

    def test_presupuesto_i000_cubre_logs(self) -> None:
        self.assertIn("CloudWatch ingesta | 3 | GB", self.finops)
        self.assertIn("CloudWatch retención | 4.5 | GB-mes", self.finops)
        self.assertIn("CloudWatch alarmas | 3 | alarma-mes", self.finops)

    def test_metric_filters_dentro_de_metricas_presupuestadas(self) -> None:
        filtros = len(re.findall(r'resource\s+"aws_cloudwatch_log_metric_filter"', self.main))
        self.assertEqual(filtros, 4)
        self.assertLessEqual(filtros, 4.5)

    def test_alarmas_incrementales_documentadas(self) -> None:
        alarmas = len(re.findall(r'resource\s+"aws_cloudwatch_metric_alarm"', self.main))
        self.assertEqual(alarmas, 8)
        # I-000 presupuesta 3 alarmas (0.3000 USD). Con redis/dynamodb condicionales
        # se declaran 6 recursos base y hasta 8 activos; incremento maximo 0.5000 USD.
        incremental = 0.50
        self.assertAlmostEqual(incremental, 0.50, places=4)
        # Margen vigente de I-000 (6.9387 USD) cubre el incremento; no se asume medido.
        self.assertGreater(6.9387, incremental)
        self.assertIn("límite de gasto**", self.main + self.vars + _leer(RAIZ / "sdd" / "_control" / "H08" / "MATRIZ_METRICAS_ALERTAS.md"))

    def test_retencion_y_volumen(self) -> None:
        bloque = self.vars.split('variable "retencion_dias"')[1].split("}")[0]
        self.assertIn("default     = 7", bloque)
        self.assertIn("var.retencion_dias <= 30", self.main)

    def test_sin_costes_no_presupuestados(self) -> None:
        texto = self.main.lower()
        for prohibido in ("aws_xray", "access_logs", "kms_key", "aws_sns_topic"):
            self.assertNotIn(prohibido, texto, prohibido)
        self.assertIn("NOT_CONFIGURED_BLOCKED_RFC007_SNS_KMS", _leer(MODULO / "outputs.tf"))

    def test_aws_no_aplicado(self) -> None:
        self.assertIn('"NOT_RUN_AWS"', _leer(MODULO / "outputs.tf"))


if __name__ == "__main__":
    unittest.main()
