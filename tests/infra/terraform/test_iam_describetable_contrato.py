"""Contrato I-060: DescribeTable de solo lectura para readiness (sin AWS).

Comprueba el modulo IAM completo con normalizacion de espacios, sin depender del
formato que impone `terraform fmt` ni del orden de las sentencias.
"""

from __future__ import annotations

import re
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
IAM = RAIZ / "infra" / "terraform" / "modules" / "iam"


class DescribeTableParaReadiness(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        crudo = (IAM / "main.tf").read_text(encoding="utf-8")
        cls.crudo = crudo
        # Espacios colapsados: las aserciones no dependen del alineado de fmt.
        cls.texto = re.sub(r"[ \t]+", " ", crudo)

    def test_accion_exacta_una_sola_vez(self) -> None:
        self.assertEqual(self.crudo.count("dynamodb:DescribeTable"), 1)
        self.assertIn('Action = ["dynamodb:DescribeTable"]', self.texto)

    def test_recurso_exacto_sin_wildcard(self) -> None:
        self.assertIn("Resource = var.dynamodb_table_arn", self.texto)
        self.assertNotIn('Resource = "*"', self.texto)

    def test_sin_acciones_administrativas_ni_de_escritura(self) -> None:
        for prohibido in (
            "dynamodb:ListTables",
            "dynamodb:Scan",
            "dynamodb:CreateTable",
            "dynamodb:DeleteTable",
            "dynamodb:UpdateTable",
            "dynamodb:DescribeTimeToLive",
        ):
            self.assertNotIn(prohibido, self.crudo, prohibido)

    def test_sentencia_propia_sin_leading_keys(self) -> None:
        # DescribeTable vive en su propia sentencia: el Sid existe y la condicion
        # LeadingKeys solo aparece en las sentencias de datos.
        self.assertIn('Sid = "DescribeTablaParaReadiness"', self.texto)
        self.assertGreaterEqual(self.crudo.count("LeadingKeys"), 1)

    def test_se_incluye_en_la_politica_runtime(self) -> None:
        self.assertIn("ddb_statement_describe", self.texto)
        indice = self.texto.index("runtime_policy = {")
        self.assertIn("local.ddb_statement_describe", self.texto[indice : indice + 300])

    def test_politicas_de_datos_intactas(self) -> None:
        for sentencia in ("ddb_statement_table", "ddb_statement_indices"):
            self.assertIn(sentencia, self.texto)
        self.assertIn("dynamodb:Query", self.crudo)
        self.assertIn("dynamodb:GetItem", self.crudo)


class ContratoExistenteActualizado(unittest.TestCase):
    """El contrato I-060 previo sigue protegiendo lo importante."""

    @classmethod
    def setUpClass(cls) -> None:
        cls.contrato = (RAIZ / "tests" / "infra" / "terraform" / "test_iam_contrato.py").read_text(
            encoding="utf-8"
        )

    def test_ya_no_prohibe_describetable(self) -> None:
        self.assertNotIn('"dynamodb:DescribeTable"', self.contrato)

    def test_sigue_prohibiendo_administracion(self) -> None:
        for prohibido in ("dynamodb:ListTables", "dynamodb:CreateTable", "dynamodb:*"):
            self.assertIn(prohibido, self.contrato, prohibido)


if __name__ == "__main__":
    unittest.main()
