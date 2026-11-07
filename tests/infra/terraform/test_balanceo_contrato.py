"""Pruebas estáticas de I-030 (balanceo HTTPS con ALB interno y CloudFront).

Comprueban invariantes de seguridad y disponibilidad sobre el código `.tf`.
`terraform validate` no evalúa preconditions ni relaciones entre recursos, por lo
que estas pruebas leen los archivos. La verificación real en AWS es
`NOT_RUN_AWS`: no se ha ejecutado plan ni apply.
"""

from __future__ import annotations

import re
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
MODULO = RAIZ / "infra" / "terraform" / "modules" / "balanceo"
ENV = RAIZ / "infra" / "terraform" / "envs" / "h08i"
RED = RAIZ / "infra" / "terraform" / "modules" / "red"


def _leer(p: Path) -> str:
    return p.read_text(encoding="utf-8")


class BalanceadorInterno(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(MODULO / "main.tf")
        cls.vars = _leer(MODULO / "variables.tf")
        cls.outs = _leer(MODULO / "outputs.tf")

    def test_alb_es_interno(self) -> None:
        self.assertIn("internal           = true", self.main)
        self.assertNotIn("internal           = false", self.main)
        self.assertNotIn("internal = false", self.main)

    def test_sin_acm_sin_route53_sin_listener_443(self) -> None:
        for prohibido in ("aws_acm_certificate", "aws_route53", "aws_lb_certificate"):
            self.assertNotIn(prohibido, self.main)
        self.assertNotIn("port              = 443", self.main)
        self.assertNotIn("protocol          = \"HTTPS\"", self.main)

    def test_listener_http_80_privado(self) -> None:
        bloque = self.main.split('resource "aws_lb_listener" "http_privado"')[1].split("}")[0]
        self.assertIn("port              = var.puerto_alb", bloque)
        self.assertIn('protocol          = "HTTP"', bloque)
        self.assertIn("puerto_alb", self.vars)
        self.assertIn("var.puerto_alb == 80", self.vars)

    def test_target_group_8080_matcher_200(self) -> None:
        bloque = self.main.split('resource "aws_lb_target_group" "backend"')[1].split("\nresource ")[0]
        self.assertIn("port        = var.puerto_backend", bloque)
        self.assertIn('protocol    = "HTTP"', bloque)
        self.assertIn('target_type = "instance"', bloque)
        self.assertIn('matcher             = "200"', bloque)

    def test_health_check_no_usa_readiness(self) -> None:
        self.assertIn('default     = "/api/v1/health/live"', self.vars)
        self.assertIn("endswith(var.health_check_path, \"/ready\")", self.vars)
        self.assertIn("!endswith(var.health_check_path, \"/ready\")", self.main)
        self.assertNotIn('default     = "/api/v1/health/ready"', self.vars)

    def test_deregistration_delay_acotado(self) -> None:
        self.assertIn("default     = 30", self.vars)
        self.assertIn("var.deregistration_delay <= 60", self.vars)
        self.assertIn("deregistration_delay = var.deregistration_delay", self.main)

    def test_sin_stickiness(self) -> None:
        """Backend stateless: no debe declararse afinidad de sesión."""
        bloque = self.main.split('resource "aws_lb_target_group" "backend"')[1].split("\nresource ")[0]
        self.assertNotIn("stickiness", bloque)
        self.assertNotIn("lb_cookie", self.main)

    def test_asociacion_al_asg_sin_servidores_extra(self) -> None:
        self.assertIn('resource "aws_autoscaling_attachment" "backend"', self.main)
        self.assertIn("autoscaling_group_name = var.asg_name", self.main)
        for prohibido in ("aws_instance", "aws_launch_template", "aws_autoscaling_group"):
            self.assertNotIn(f'resource "{prohibido}"', self.main)

    def test_dos_zonas_distintas(self) -> None:
        self.assertIn("local.zonas_count == 2", self.main)
        self.assertIn("length(distinct(values(var.subnet_ids))) == 2", self.vars)
        self.assertIn("length(var.subnet_ids) == 2", self.vars)

    def test_sg_alb_reutilizado_de_i010(self) -> None:
        self.assertIn("security_groups    = [var.sg_alb_id]", self.main)
        self.assertNotIn('resource "aws_security_group"', self.main)
        env = _leer(ENV / "main.tf")
        self.assertIn('sg_alb_id     = module.red.security_groups["alb"]', env)
        self.assertIn("var.sg_alb_id != var.sg_backend_id", self.main)


class RedDeI010Compatible(unittest.TestCase):
    """El ALB depende de reglas que pertenecen a I-010: deben seguir existiendo."""

    @classmethod
    def setUpClass(cls) -> None:
        cls.red = _leer(RED / "main.tf")

    def test_ingress_80_desde_prefix_list_cloudfront(self) -> None:
        self.assertIn('name = "com.amazonaws.global.cloudfront.origin-facing"', self.red)
        self.assertIn('resource "aws_vpc_security_group_ingress_rule" "alb_cloudfront_80"', self.red)
        self.assertIn("from_port         = 80", self.red)

    def test_egress_alb_al_backend_8080(self) -> None:
        self.assertIn('resource "aws_vpc_security_group_egress_rule" "alb_backend_8080"', self.red)
        self.assertIn("referenced_security_group_id = aws_security_group.backend.id", self.red)

    def test_igw_sin_rutas_publicas(self) -> None:
        self.assertIn('resource "aws_internet_gateway" "principal"', self.red)
        self.assertNotIn("gateway_id = aws_internet_gateway.principal.id", self.red)

    def test_nacl_egress_efimeros_para_vpc_origins(self) -> None:
        """VPC origins no evalúa ingress NACL, pero sí el egress de retorno."""
        self.assertIn('resource "aws_network_acl_rule" "egress_efimeros"', self.red)
        self.assertIn("from_port      = 1024", self.red)
        self.assertIn("to_port        = 65535", self.red)


class CloudFront(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(MODULO / "main.tf")
        cls.vars = _leer(MODULO / "variables.tf")
        cls.outs = _leer(MODULO / "outputs.tf")

    def test_vpc_origin_http_only(self) -> None:
        self.assertIn('resource "aws_cloudfront_vpc_origin" "alb"', self.main)
        self.assertIn('origin_protocol_policy = "http-only"', self.main)
        self.assertIn("arn                    = aws_lb.principal.arn", self.main)
        self.assertIn("http_port              = var.puerto_alb", self.main)

    def test_origen_usa_vpc_origin_config_no_custom_origin(self) -> None:
        self.assertIn("vpc_origin_config {", self.main)
        self.assertIn("vpc_origin_id = aws_cloudfront_vpc_origin.alb.id", self.main)
        self.assertIn("domain_name = aws_cloudfront_vpc_origin.alb.arn", self.main)
        self.assertNotIn("custom_origin_config", self.main)

    def test_redireccion_https_en_el_borde(self) -> None:
        self.assertIn('viewer_protocol_policy = "redirect-to-https"', self.main)
        self.assertNotIn('viewer_protocol_policy = "allow-all"', self.main)

    def test_sin_cache_para_api_financiera(self) -> None:
        self.assertIn('name = "Managed-CachingDisabled"', self.main)
        self.assertIn('name = "Managed-AllViewer"', self.main)
        self.assertIn("cache_policy_id          = data.aws_cloudfront_cache_policy.sin_cache.id", self.main)
        self.assertIn("origin_request_policy_id = data.aws_cloudfront_origin_request_policy.api.id", self.main)
        self.assertNotIn("forwarded_values", self.main)
        self.assertNotIn("CacheEverything", self.main)
        self.assertNotIn("cache_policy_id = \"658327ea-f89d-4fab-a63d-7e88639e58f6\"", self.main)

    def test_metodos_rest_completos(self) -> None:
        bloque = self.main.split("default_cache_behavior {")[1].split("}")[0]
        for metodo in ("GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"):
            self.assertIn(f'"{metodo}"', bloque)
        self.assertNotIn('allowed_methods = ["GET", "HEAD"]', bloque)

    def test_certificado_por_defecto_sin_sobreafirmar_tls(self) -> None:
        """AWS fija TLSv1 con el certificado por defecto: no se puede prometer TLS 1.2."""
        self.assertIn("cloudfront_default_certificate = true", self.main)
        self.assertNotIn("minimum_protocol_version", self.main)
        self.assertNotIn("acm_certificate_arn", self.main)
        self.assertIn("TLSv1 (fijada por AWS con certificado por defecto, no configurable)", self.outs)
        self.assertIn("tls_app_hasta_alb       = false", self.outs)

    def test_sin_waf_ni_logging_de_pago(self) -> None:
        self.assertNotIn("web_acl_id", self.main)
        self.assertNotIn("logging_config", self.main)

    def test_price_class_barata_por_defecto(self) -> None:
        self.assertIn('default     = "PriceClass_100"', self.vars)

    def test_estado_no_se_declara_desplegado(self) -> None:
        self.assertIn('value       = "NOT_RUN_AWS"', self.outs)


class ComposicionDelEntorno(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.env = _leer(ENV / "main.tf")
        cls.env_vars = _leer(ENV / "variables.tf")

    def test_modulo_balanceo_compuesto(self) -> None:
        self.assertIn('module "balanceo"', self.env)
        self.assertIn('source = "../../modules/balanceo"', self.env)
        self.assertIn("subnet_ids    = module.red.subredes_privadas", self.env)
        self.assertIn("asg_name      = module.computo.asg_name", self.env)
        self.assertIn("vpc_id   = module.red.vpc_id", self.env)

    def test_provider_soporta_vpc_origin(self) -> None:
        self.assertIn('version = "~> 5.72"', self.env)
        self.assertNotIn('version = "~> 5.60"', self.env)

    def test_salidas_clave(self) -> None:
        for salida in (
            "alb_es_interno",
            "cloudfront_domain",
            "vpc_origin_id",
            "health_check",
            "estado_despliegue",
        ):
            self.assertIn(salida, self.env)

    def test_sin_recursos_prohibidos_en_el_repo_terraform(self) -> None:
        texto = "\n".join(p.read_text(encoding="utf-8") for p in (RAIZ / "infra" / "terraform").rglob("*.tf"))
        for prohibido in (
            "aws_acm_certificate",
            "aws_route53",
            "aws_wafv2",
            "aws_nat_gateway",
            "aws_eip",
            "map_public_ip_on_launch   = true",
        ):
            self.assertNotIn(prohibido, texto)


if __name__ == "__main__":
    unittest.main()
