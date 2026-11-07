"""Alineación FinOps de I-030 con el escenario C ratificado en I-000.

I-030 no introduce precios nuevos: debe limitarse a los componentes ya
presupuestados (ALB interno + LCU, CloudFront, VPC origin sin cargo, gateway
endpoints de I-010) y no crear servicios facturables fuera de ese escenario.
"""

from __future__ import annotations

import re
import sys
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
MODULO = RAIZ / "infra" / "terraform" / "modules" / "balanceo"

sys.path.insert(0, str(RAIZ / "infra" / "finops"))

from escenarios import escenarios  # noqa: E402

RECURSOS_PERMITIDOS = {
    "terraform_data",  # precondiciones, sin coste
    "aws_lb",
    "aws_lb_target_group",
    "aws_lb_listener",
    "aws_autoscaling_attachment",
    "aws_cloudfront_vpc_origin",
    "aws_cloudfront_distribution",
}


class EscenarioCSigueSiendoLaBase(unittest.TestCase):
    def setUp(self) -> None:
        self.c = escenarios()["C_CLOUDFRONT_VPC_ORIGIN_PRIVADO"]
        self.por_clave = {p.clave: p for p in self.c.partidas}

    def test_partidas_de_balanceo_presupuestadas(self) -> None:
        for clave in ("ALB_FIJO", "ALB_LCU", "CERT_CF", "VPC_ORIGIN", "GW_ENDPOINTS"):
            self.assertIn(clave, self.por_clave)

    def test_sin_partidas_nuevas_habilitadas(self) -> None:
        """Lo que I-030 no crea debe seguir en cantidad cero en el escenario."""
        for clave in ("IPV4_ALB", "IPV4_EC2", "ROUTE53", "NAT"):
            self.assertEqual(self.por_clave[clave].cantidad.strip(), "0", clave)

    def test_condicion_tls_sigue_declarada(self) -> None:
        condiciones = " ".join(self.c.condiciones)
        self.assertIn("TLS extremo a extremo", condiciones)

    def test_escenario_cumple_dos_instancias_y_24x7(self) -> None:
        self.assertTrue(self.c.cumple_24x7)
        self.assertTrue(self.c.cumple_dos_instancias)


class ModuloNoIntroduceCostes(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.texto = (MODULO / "main.tf").read_text(encoding="utf-8")
        cls.recursos = set(re.findall(r'resource\s+"([a-z0-9_]+)"', cls.texto))

    def test_solo_recursos_presupuestados(self) -> None:
        self.assertEqual(self.recursos, RECURSOS_PERMITIDOS)

    def test_sin_servicios_facturables_ajenos(self) -> None:
        for prohibido in (
            "aws_nat_gateway",
            "aws_eip",
            "aws_wafv2_web_acl",
            "aws_acm_certificate",
            "aws_route53_zone",
            "aws_cloudwatch_log_group",
            "aws_cloudfront_response_headers_policy",
            "aws_lb_listener_rule",
            "aws_lb_target_group_attachment",
        ):
            self.assertNotIn(prohibido, self.recursos)

    def test_un_solo_alb_y_un_solo_vpc_origin(self) -> None:
        self.assertEqual(self.texto.count('resource "aws_lb" "'), 1)
        self.assertEqual(self.texto.count('resource "aws_cloudfront_vpc_origin" "'), 1)
        self.assertEqual(self.texto.count('resource "aws_cloudfront_distribution" "'), 1)

    def test_deletion_protection_desactivada_para_teardown(self) -> None:
        """El plan Free debe poder destruir; destroy sigue exigiendo Gate humano."""
        self.assertIn("enable_deletion_protection = false", self.texto)


if __name__ == "__main__":
    unittest.main()
