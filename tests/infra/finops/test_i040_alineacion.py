"""Alineación FinOps de I-040 con las partidas DynamoDB de I-000.

I-040 no introduce precios nuevos: debe usar sólo los conceptos ya
presupuestados (almacenamiento, lecturas/escrituras on-demand, PITR) y no crear
servicios facturables añadidos (streams, capacidad provisionada ociosa, copias
programadas fuera de I-130, claves KMS propias).
"""

from __future__ import annotations

import re
import sys
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
MODULO = RAIZ / "infra" / "terraform" / "modules" / "dynamodb"

sys.path.insert(0, str(RAIZ / "infra" / "finops"))

from escenarios import escenarios  # noqa: E402

PARTIDAS_DYNAMODB = ("DDB_STORAGE", "DDB_LECTURAS", "DDB_ESCRITURAS", "DDB_PITR", "DDB_BACKUP")


class EscenarioCPresupuestaDynamoDB(unittest.TestCase):
    def setUp(self) -> None:
        self.c = escenarios()["C_CLOUDFRONT_VPC_ORIGIN_PRIVADO"]
        self.por_clave = {p.clave: p for p in self.c.partidas}

    def test_partidas_dynamodb_presentes(self) -> None:
        for clave in PARTIDAS_DYNAMODB:
            self.assertIn(clave, self.por_clave)

    def test_pitr_presupuestado_y_requerido(self) -> None:
        pitr = self.por_clave["DDB_PITR"]
        self.assertTrue(pitr.requerida)
        self.assertEqual(pitr.cantidad, "3")

    def test_almacenamiento_dentro_del_plan_free(self) -> None:
        """El plan Free cubre 25 GB; el modelo usa 3 GB más copias."""
        self.assertEqual(self.por_clave["DDB_STORAGE"].cantidad, "3")
        self.assertEqual(self.por_clave["DDB_BACKUP"].cantidad, "3")

    def test_on_demand_no_deja_capacidad_ociosa(self) -> None:
        """Sin capacidad provisionada no hay coste fijo con la tabla vacía."""
        self.assertEqual(self.por_clave["DDB_LECTURAS"].unidad, "millon-RRU")
        self.assertEqual(self.por_clave["DDB_ESCRITURAS"].unidad, "millon-WRU")


class ModuloNoIntroduceCostes(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.texto = (MODULO / "main.tf").read_text(encoding="utf-8")
        cls.vars = (MODULO / "variables.tf").read_text(encoding="utf-8")
        cls.recursos = set(re.findall(r'resource\s+"([a-z0-9_]+)"', cls.texto))

    def test_solo_tabla_y_validacion(self) -> None:
        self.assertEqual(self.recursos, {"terraform_data", "aws_dynamodb_table"})

    def test_sin_streams(self) -> None:
        self.assertIn("stream_enabled = false", self.texto)
        self.assertNotIn("stream_view_type", self.texto)

    def test_sin_capacidad_provisionada(self) -> None:
        self.assertIn('billing_mode = "PAY_PER_REQUEST"', self.texto)
        self.assertNotIn("read_capacity", self.texto)
        self.assertNotIn("write_capacity", self.texto)

    def test_sin_backups_programados_ni_kms_propio(self) -> None:
        """Las copias programadas son de I-130; KMS propio tendría coste fijo."""
        for prohibido in ("aws_backup_plan", "aws_backup_vault", "aws_kms_key", "aws_kms_alias"):
            self.assertNotIn(prohibido, self.recursos)

    def test_un_solo_gsi_por_defecto(self) -> None:
        """Cada GSI replica escrituras: el segundo es opt-in y no se factura por defecto."""
        self.assertEqual(self.texto.count("name               = \"GSI1\"") + self.texto.count('name = "GSI1"'), 1)
        self.assertIn("var.habilitar_gsi2 ? [", self.texto)
        self.assertRegex(self.vars, r'variable "habilitar_gsi2"[\s\S]*?default     = false')

    def test_pitr_configurable(self) -> None:
        self.assertIn("pitr_habilitado", self.vars)
        self.assertRegex(self.vars, r'variable "pitr_habilitado"[\s\S]*?default     = true')


if __name__ == "__main__":
    unittest.main()
