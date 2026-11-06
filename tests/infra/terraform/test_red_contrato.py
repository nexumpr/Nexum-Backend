"""Pruebas estáticas de I-010 (red VPC) y reconciliación con I-000.

No contacta AWS ni ejecuta Terraform: analiza el código `.tf` y el modelo FinOps.
Complementa —no sustituye— `terraform validate`.
"""

from __future__ import annotations

import json
import re
import sys
import unittest
from decimal import Decimal
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(RAIZ / "infra" / "finops"))

import conectividad  # noqa: E402
import dinero  # noqa: E402

MODULO = RAIZ / "infra" / "terraform" / "modules" / "red"
ENV = RAIZ / "infra" / "terraform" / "envs" / "h08i"
TODOS_TF = list(MODULO.glob("*.tf")) + list(ENV.glob("*.tf"))


def _leer(ruta: Path) -> str:
    return ruta.read_text(encoding="utf-8")


def _todo() -> str:
    return "\n".join(_leer(p) for p in TODOS_TF)


class Estructura(unittest.TestCase):
    def test_hay_ficheros_esperados(self) -> None:
        for nombre in ("main.tf", "variables.tf", "outputs.tf"):
            self.assertTrue((MODULO / nombre).is_file(), nombre)
        for nombre in ("main.tf", "variables.tf"):
            self.assertTrue((ENV / nombre).is_file(), nombre)

    def test_provider_versionado(self) -> None:
        env = _leer(ENV / "main.tf")
        self.assertIn("required_version", env)
        self.assertRegex(env, r"source\s*=\s*\"hashicorp/aws\"")
        self.assertRegex(env, r"version\s*=\s*\"~>")

    def test_sin_backend_remoto(self) -> None:
        env = _leer(ENV / "main.tf")
        self.assertNotRegex(env, r"(?m)^\s*backend\s+\"s3\"")

    def test_sin_credenciales_ni_cuenta(self) -> None:
        for ruta in TODOS_TF:
            texto = _leer(ruta)
            self.assertNotRegex(texto, r"AKIA[0-9A-Z]{16}", ruta.name)
            self.assertNotRegex(texto, r"\b\d{12}\b", ruta.name)
            self.assertNotRegex(texto, r"(?i)(secret_key|password\s*=)", ruta.name)


class Red(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(MODULO / "main.tf")

    def test_dos_subredes_privadas_en_dos_az(self) -> None:
        self.assertIn('resource "aws_subnet" "privada"', self.main)
        self.assertIn("for_each = local.subredes", self.main)
        self.assertIn("map_public_ip_on_launch = false", self.main)
        variables = _leer(MODULO / "variables.tf")
        self.assertIn('default     = ["us-east-2a", "us-east-2b"]', variables)
        self.assertIn("length(distinct(var.zonas)) == 2", variables)

    def test_cidr_validado(self) -> None:
        variables = _leer(MODULO / "variables.tf")
        self.assertIn("can(cidrhost(var.cidr_vpc, 0))", variables)
        self.assertIn("^(10|172|192)\\\\.", variables)

    def test_internet_gateway_para_vpc_origins(self) -> None:
        self.assertIn('resource "aws_internet_gateway" "principal"', self.main)

    def test_sin_nat_y_sin_ruta_publica(self) -> None:
        self.assertNotIn('resource "aws_nat_gateway"', self.main)
        self.assertNotIn('resource "aws_route" "', self.main)
        self.assertNotIn("gateway_id = aws_internet_gateway", self.main)

    def test_subredes_asociadas_a_tabla_privada(self) -> None:
        self.assertIn('resource "aws_route_table_association" "privada"', self.main)
        self.assertIn("route_table_id = aws_route_table.privada.id", self.main)


class Endpoints(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(MODULO / "main.tf")

    def test_gateway_endpoints_s3_y_dynamodb(self) -> None:
        self.assertIn('resource "aws_vpc_endpoint" "s3"', self.main)
        self.assertIn('resource "aws_vpc_endpoint" "dynamodb"', self.main)
        self.assertEqual(self.main.count('vpc_endpoint_type = "Gateway"'), 2)

    def test_logs_es_el_unico_permanente(self) -> None:
        self.assertIn("endpoints_permanentes = {", self.main)
        bloque = self.main.split("endpoints_permanentes = {")[1].split("}")[0]
        self.assertIn("logs", bloque)
        self.assertNotIn("secretsmanager", bloque)

    def test_auxiliares_condicionados_a_ventana(self) -> None:
        self.assertIn("endpoints_ventana = {", self.main)
        self.assertIn(
            "var.ventana_mantenimiento_activa ? local.endpoints_ventana : {}", self.main
        )
        for servicio in ("secretsmanager", "ssm", "ssmmessages"):
            self.assertIn(f"{servicio}", self.main)
        self.assertIn('resource "aws_vpc_endpoint" "ventana"', self.main)

    def test_no_se_presupuesta_monitoring_ni_ec2messages(self) -> None:
        self.assertNotIn("monitoring", self.main)
        self.assertNotIn("ec2messages", self.main)


class Seguridad(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(MODULO / "main.tf")

    def test_alb_solo_desde_cloudfront_en_80(self) -> None:
        self.assertIn("prefix_list_id    = data.aws_ec2_managed_prefix_list.cloudfront_origen.id", self.main)
        self.assertIn('name = "com.amazonaws.global.cloudfront.origin-facing"', self.main)
        self.assertIn("from_port         = 80", self.main)

    def test_no_hay_https_en_alb_por_decision_d2(self) -> None:
        self.assertNotIn("from_port         = 443\n  to_port           = 443\n  prefix_list_id", self.main)
        self.assertNotIn('resource "aws_lb_listener"', self.main)

    def test_backend_recibe_solo_del_alb(self) -> None:
        bloque = self.main.split('resource "aws_vpc_security_group_ingress_rule" "backend_desde_alb"')[1]
        bloque = bloque.split("}")[0]
        self.assertIn("referenced_security_group_id = aws_security_group.alb.id", bloque)
        self.assertIn("var.puerto_backend", bloque)

    def test_redis_no_expuesto(self) -> None:
        bloque = self.main.split('resource "aws_vpc_security_group_ingress_rule" "redis_desde_backend"')[1]
        bloque = bloque.split("}")[0]
        self.assertIn("from_port                    = 6379", bloque)
        self.assertIn("referenced_security_group_id = aws_security_group.backend.id", bloque)

    def test_ningun_ingress_abierto_a_internet(self) -> None:
        for trozo in self.main.split('resource "aws_vpc_security_group_ingress_rule"')[1:]:
            self.assertNotIn('cidr_ipv4', trozo.split("}")[0])

    def test_sin_ssh(self) -> None:
        self.assertNotIn("from_port         = 22", self.main)
        self.assertNotIn("to_port           = 22", self.main)

    def test_nacl_permite_efimeros_de_retorno(self) -> None:
        self.assertIn('resource "aws_network_acl_rule" "salida_efimeros"', self.main)
        bloque = self.main.split('resource "aws_network_acl_rule" "salida_efimeros"')[1].split("}")[0]
        self.assertIn("from_port      = 1024", bloque)
        self.assertIn("to_port        = 65535", bloque)
        self.assertIn("egress         = true", bloque)

    def test_sin_sg_administrado_por_cloudfront_creado(self) -> None:
        self.assertNotIn("CloudFront-VPCOrigins-Service-SG", self.main)


class Presupuesto(unittest.TestCase):
    def test_escenario_a_y_b_con_margen(self) -> None:
        conectividad_usd = dinero.dec(conectividad.costo_conectividad()["costo_total_45d_usd"])
        self.assertEqual(conectividad_usd, Decimal("23.7400"))

        a = dinero.cuantizar((dinero.dec("79.0430") + conectividad_usd) * Decimal("1.10"))
        b = dinero.cuantizar((dinero.dec("84.0430") + conectividad_usd) * Decimal("1.10"))
        self.assertEqual(dinero.formatear(a), "113.0613")
        self.assertEqual(dinero.formatear(b), "118.5613")
        for total in (a, b):
            self.assertLessEqual(total, dinero.dec("120"))

    def test_ningun_servicio_duplicado_en_el_presupuesto(self) -> None:
        conectividad_servicios = list(conectividad.PERMANENTES) + list(conectividad.TEMPORALES)
        self.assertEqual(len(conectividad_servicios), len(set(conectividad_servicios)))

    def test_no_hay_endpoints_fuera_del_diseno(self) -> None:
        texto = _leer(MODULO / "main.tf")
        locales = texto.split("locals {")[1].split("subredes =")[0]
        esperados = {"logs", "secretsmanager", "ssm", "ssmmessages"}
        encontrados = set(
            re.findall(r"com\.amazonaws\.\$\{var\.region\}\.([a-z0-9]+)", locales)
        )
        self.assertEqual(encontrados, esperados)


if __name__ == "__main__":
    unittest.main()
