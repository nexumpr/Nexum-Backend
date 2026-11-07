"""Alineación FinOps de I-050 con la partida Redis de I-000.

I-050 no introduce precios nuevos: usa el SKU ya verificado (`cache.t4g.micro`,
0.016 USD/nodo-hora, 1080 nodo-hora = 17.2800 USD/45 d) y no añade réplicas,
backups ni transferencia inter-AZ extra.
"""

from __future__ import annotations

import json
import re
import sys
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
MODULO = RAIZ / "infra" / "terraform" / "modules" / "redis"

sys.path.insert(0, str(RAIZ / "infra" / "finops"))

from escenarios import escenarios  # noqa: E402


class PartidaRedisPresupuestada(unittest.TestCase):
    def setUp(self) -> None:
        self.c = escenarios()["C_CLOUDFRONT_VPC_ORIGIN_PRIVADO"]
        self.por_clave = {p.clave: p for p in self.c.partidas}

    def test_partida_presente_y_cuantificada(self) -> None:
        redis = self.por_clave["REDIS_OSS"]
        self.assertEqual(redis.cantidad, "1080")
        self.assertEqual(redis.unidad, "nodo-hora")
        self.assertEqual(redis.sku, "ELASTICACHE_REDIS_T4G_MICRO_HORA")
        self.assertTrue(redis.requerida)

    def test_precio_del_catalogo_verificado(self) -> None:
        catalogo = json.loads((RAIZ / "infra" / "finops" / "precios_us-east-2.json").read_text(encoding="utf-8"))
        item = catalogo["items"]["ELASTICACHE_REDIS_T4G_MICRO_HORA"]
        self.assertEqual(item["estado"], "VERIFIED_AWS")
        self.assertEqual(item["precio_usd"], "0.016")
        self.assertNotIn("VALKEY", item["sku"].upper())

    def test_transferencia_inter_az_presupuestada(self) -> None:
        self.assertIn("REDIS_INTERAZ", self.por_clave)
        self.assertEqual(self.por_clave["REDIS_INTERAZ"].cantidad, "10")


class ModuloNoIntroduceCostes(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.texto = (MODULO / "main.tf").read_text(encoding="utf-8")
        cls.vars = (MODULO / "variables.tf").read_text(encoding="utf-8")
        cls.recursos = set(re.findall(r'resource\s+"([a-z0-9_]+)"', cls.texto))

    def test_solo_recursos_sin_coste_extra(self) -> None:
        self.assertEqual(
            self.recursos,
            {
                "terraform_data",
                "aws_elasticache_subnet_group",
                "aws_elasticache_parameter_group",
                "aws_elasticache_user",
                "aws_elasticache_user_group",
                "aws_elasticache_replication_group",
            },
        )

    def test_un_solo_nodo_sin_replicas(self) -> None:
        self.assertIn("num_cache_clusters         = var.num_nodos", self.texto)
        self.assertRegex(self.vars, r'variable "num_nodos"[\s\S]*?default     = 1')
        self.assertIn("var.num_nodos == 1", self.vars)

    def test_sin_backups_ni_retencion(self) -> None:
        self.assertIn("snapshot_retention_limit = 0", self.texto)
        self.assertNotIn("final_snapshot_identifier", self.texto)

    def test_sin_serverless_ni_tipo_mayor(self) -> None:
        """El modelo de coste de I-000 asume node-based t4g.micro."""
        self.assertNotIn("aws_elasticache_serverless_cache", self.recursos)
        for caro in ("cache.t4g.medium", "cache.r7g", "cache.m7g"):
            self.assertNotIn(caro, self.texto)

    def test_sin_kms_propio_ni_transferencia_extra(self) -> None:
        for prohibido in ("aws_kms_key", "aws_nat_gateway", "aws_eip"):
            self.assertNotIn(prohibido, self.recursos)


if __name__ == "__main__":
    unittest.main()
