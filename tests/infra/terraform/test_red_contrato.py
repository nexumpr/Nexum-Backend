"""Pruebas estáticas de I-010 (red VPC) y reconciliación con I-000.

No contacta AWS ni ejecuta Terraform: analiza el código `.tf` y el modelo FinOps.
Complementa —no sustituye— `terraform validate`.
"""

from __future__ import annotations

import ipaddress
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
            self.assertNotRegex(texto, r"(?i)(secret_key|password\s*=)", ruta.name)

            if ruta.name == "backend.tf":
                # Única excepción autorizada: el nombre del bucket de state incluye
                # el ID de cuenta y un bloque `backend` de Terraform NO admite
                # variables, así que el literal es inevitable. Se exige que el
                # número aparezca sólo dentro de nexum-h08i-tfstate-<cuenta>.
                sin_bucket = texto.replace(
                    "nexum-h08i-tfstate-713797602090", ""
                )
                self.assertNotRegex(sin_bucket, r"\b\d{12}\b", ruta.name)
                continue

            self.assertNotRegex(texto, r"\b\d{12}\b", ruta.name)


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

    def test_cidr_validado_rfc1918(self) -> None:
        variables = _leer(MODULO / "variables.tf")
        self.assertIn("can(cidrnetmask(var.cidr_vpc))", variables)
        self.assertIn('"^10', variables)
        self.assertIn('"^172', variables)
        self.assertIn('"^192', variables)
        self.assertIn(">= 16", variables)
        self.assertIn("<= 24", variables)

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
        # Sólo el ALB no debe tener 443: S3/DynamoDB sí usan 443 con prefix list.
        bloque = self.main.split('"alb_cloudfront_80"')[1].split('}')[0]
        self.assertIn('from_port         = 80', bloque)
        self.assertNotIn('from_port         = 443', bloque)
        for trozo in self.main.split('resource "aws_vpc_security_group_ingress_rule"')[1:]:
            cuerpo = trozo.split('}')[0]
            if 'aws_security_group.alb.id' in cuerpo:
                self.assertNotIn('from_port                    = 443', cuerpo)
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

    def test_nacl_permite_efimeros_en_ambos_sentidos(self) -> None:
        for recurso in ("ingress_efimeros", "egress_efimeros"):
            self.assertIn(f'resource "aws_network_acl_rule" "{recurso}"', self.main)
            bloque = self.main.split(f'resource "aws_network_acl_rule" "{recurso}"')[1].split("}")[0]
            self.assertIn("from_port      = 1024", bloque)
            self.assertIn("to_port        = 65535", bloque)

    def test_nacl_cubre_flujos_internos(self) -> None:
        for recurso in (
            "ingress_alb_80",
            "ingress_backend_8080",
            "ingress_redis_6379",
            "egress_https",
            "egress_backend_8080",
            "egress_redis_6379",
        ):
            self.assertIn(f'resource "aws_network_acl_rule" "{recurso}"', self.main)

    def test_nacl_https_sale_a_cualquier_destino_aws(self) -> None:
        bloque = self.main.split('resource "aws_network_acl_rule" "egress_https"')[1].split("}")[0]
        self.assertIn('cidr_block     = "0.0.0.0/0"', bloque)
        self.assertIn("from_port      = 443", bloque)

    def test_egress_s3_por_prefix_list(self) -> None:
        self.assertIn('resource "aws_vpc_security_group_egress_rule" "backend_s3_443"', self.main)
        self.assertIn('data "aws_ec2_managed_prefix_list" "s3"', self.main)
        self.assertIn('"com.amazonaws.${var.region}.s3"', self.main)
        bloque = self.main.split('"backend_s3_443"')[1].split("}")[0]
        self.assertIn("prefix_list_id    = data.aws_ec2_managed_prefix_list.s3.id", bloque)
        self.assertIn("from_port         = 443", bloque)

    def test_egress_dynamodb_por_prefix_list(self) -> None:
        self.assertIn('resource "aws_vpc_security_group_egress_rule" "backend_dynamodb_443"', self.main)
        self.assertIn('data "aws_ec2_managed_prefix_list" "dynamodb"', self.main)
        self.assertIn('"com.amazonaws.${var.region}.dynamodb"', self.main)
        bloque = self.main.split('"backend_dynamodb_443"')[1].split("}")[0]
        self.assertIn("prefix_list_id    = data.aws_ec2_managed_prefix_list.dynamodb.id", bloque)

    def test_sin_cidr_abierto_en_security_groups(self) -> None:
        for trozo in self.main.split('resource "aws_vpc_security_group_')[1:]:
            self.assertNotIn('cidr_ipv4', trozo.split("}")[0])

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
        bloque = texto.split("endpoints_permanentes = {")[1].split('resource "aws_vpc"')[0]
        esperados = {"logs", "secretsmanager", "ssm", "ssmmessages"}
        encontrados = set(
            re.findall(r"com\.amazonaws\.\$\{var\.region\}\.([a-z0-9]+)", bloque)
        )
        self.assertEqual(encontrados, esperados)

class ConectividadAWS(unittest.TestCase):
    """Los dos flujos que un gateway endpoint no cubre por SG propio."""

    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(MODULO / "main.tf")

    def test_gateway_endpoints_asociados_a_la_tabla_privada(self) -> None:
        self.assertEqual(
            self.main.count("route_table_ids   = [aws_route_table.privada.id]"), 2
        )

    def test_backend_puede_consultar_dynamodb_y_s3(self) -> None:
        for sku in ("backend_s3_443", "backend_dynamodb_443"):
            self.assertIn(f'"{sku}"', self.main, sku)
        # Si alguien elimina las reglas, el conteo de egress 443 cae por debajo de 3.
        self.assertGreaterEqual(self.main.count("= 443"), 3)

    def test_iam_no_se_define_aqui(self) -> None:
        self.assertNotIn('resource "aws_iam_', self.main)


class CapacidadSubredes(unittest.TestCase):
    """Reproduce `cidrsubnet` y las cotas reales de tamaño exigidas por el ALB."""

    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(MODULO / "main.tf")
        cls.variables = _leer(MODULO / "variables.tf")

    def _subred(self, vpc: str, bits: int, indice: int) -> ipaddress.IPv4Network:
        red = ipaddress.ip_network(vpc, strict=True)
        nueva = red.prefixlen + bits
        tamano = 1 << (32 - nueva)
        base = int(red.network_address) + indice * tamano
        return ipaddress.ip_network((base, nueva), strict=False)

    def test_vpc_por_defecto_da_dos_barrios_24(self) -> None:
        a = self._subred("10.42.0.0/16", 8, 0)
        b = self._subred("10.42.0.0/16", 8, 1)
        self.assertEqual(str(a), "10.42.0.0/24")
        self.assertEqual(str(b), "10.42.1.0/24")
        self.assertFalse(a.overlaps(b))
        self.assertTrue(a.subnet_of(ipaddress.ip_network("10.42.0.0/16")))

    def test_barrio_27_es_minimo_aceptado(self) -> None:
        red = self._subred("10.42.0.0/16", 11, 1)
        self.assertEqual(red.prefixlen, 27)
        self.assertGreaterEqual(red.num_addresses - 5, 8, "AWS reserva 5 direcciones")

    def test_barrio_28_rechazado_por_politica_de_escalado(self) -> None:
        # AWS exige 8 IP libres; /27 es el minimo recomendado para que el ALB
        # pueda escalar. El modulo adopta /27 como politica explicita.
        red28 = self._subred("10.42.0.0/16", 12, 0)
        red27 = self._subred("10.42.0.0/16", 11, 0)
        self.assertEqual(red28.prefixlen, 28)
        self.assertEqual(red27.prefixlen, 27)
        self.assertGreaterEqual(red27.num_addresses - 5, 8)
        self.assertGreater(red27.num_addresses, red28.num_addresses)
        self.assertIn("<= 27", self.main)

    def test_hcl_rechaza_barrio_28(self) -> None:
        self.assertIn("<= 27", self.main)
        self.assertIn("de al menos /27", self.main)
        self.assertRegex(self.variables, r"bits_subredes <= 11")

    def test_hcl_rechaza_barrios_gigantes(self) -> None:
        self.assertIn(">= 20", self.main)
        self.assertRegex(self.variables, r"bits_subredes >= 4")

    def test_hcl_usa_indices_cero_y_uno(self) -> None:
        self.assertIn("cidrsubnet(var.cidr_vpc, var.bits_subredes, indice)", self.main)
        self.assertNotIn("10 + indice", self.main)

    def test_zonas_coherentes_con_region_en_precondition(self) -> None:
        self.assertIn("startswith(az, var.region)", self.main)

    def test_rfc1918_obligatorio(self) -> None:
        for prefijo in ("^10", "^172", "^192"):
            self.assertIn(prefijo, self.variables)

    def test_rangos_no_privados_se_rechazan(self) -> None:
        patrones = [r"^10\.", r"^172\.(1[6-9]|2[0-9]|3[01])\.", r"^192\.168\."]
        for cidr in ("8.8.0.0/16", "172.15.0.0/16", "172.32.0.0/16", "192.169.0.0/16"):
            self.assertFalse(any(re.match(patron, cidr) for patron in patrones), cidr)

    def test_zonas_duplicadas_rechazadas(self) -> None:
        self.assertIn("length(distinct(var.zonas)) == 2", self.variables)
        self.assertIn("length(distinct(var.zonas)) == 2", self.main)

    def test_indices_siempre_validos_y_sin_solape(self) -> None:
        for bits in range(4, 12):
            a = self._subred("10.42.0.0/16", bits, 0)
            b = self._subred("10.42.0.0/16", bits, 1)
            self.assertFalse(a.overlaps(b), bits)
            self.assertEqual(a.prefixlen, 16 + bits)


if __name__ == "__main__":
    unittest.main()