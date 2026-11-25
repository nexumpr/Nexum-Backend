"""Contrato del módulo `egreso_fcm` (NAT Gateway 1 AZ, deshabilitado).

Comprueba que la preparación no crea recursos por defecto, que la única
topología es un NAT en una AZ, que la subred pública es nueva y dedicada, que
las EC2 siguen privadas, que no se abre SSH y que el egreso 443 no inventa una
prefix list de Google inexistente.
"""

from __future__ import annotations

import re
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
MODULO = RAIZ / "infra" / "terraform" / "modules" / "egreso_fcm"
ENVS = RAIZ / "infra" / "terraform" / "envs" / "h08i"
RED = RAIZ / "infra" / "terraform" / "modules" / "red"


def _leer(p: Path) -> str:
    return p.read_text(encoding="utf-8")


def _bloque(texto: str, cabecera: str) -> str:
    inicio = texto.find(cabecera)
    if inicio < 0:
        raise AssertionError(f"no se encontró {cabecera!r}")
    apertura = texto.index("{", inicio)
    nivel = 0
    for pos in range(apertura, len(texto)):
        if texto[pos] == "{":
            nivel += 1
        elif texto[pos] == "}":
            nivel -= 1
            if nivel == 0:
                return texto[apertura + 1 : pos]
    raise AssertionError(f"{cabecera!r} sin cierre")


class PorDefectoDeshabilitado(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(MODULO / "main.tf")
        cls.vars = _leer(MODULO / "variables.tf")
        cls.outputs = _leer(MODULO / "outputs.tf")
        cls.env_main = _leer(ENVS / "main.tf")
        cls.env_vars = _leer(ENVS / "variables.tf")

    def test_habilitar_false_por_defecto(self) -> None:
        bloque = self.vars.split('variable "habilitar"', 1)[1].split("\n}", 1)[0]
        self.assertIn("default     = false", bloque)

    def test_modulo_en_el_entorno_nace_con_count_cero(self) -> None:
        bloque = _bloque(self.env_main, 'module "egreso_fcm"')
        self.assertIn("count = var.habilitar_egreso_fcm ? 1 : 0", bloque)
        self.assertIn("habilitar = var.habilitar_egreso_fcm", bloque)
        self.assertIn("source = \"../../modules/egreso_fcm\"", bloque)

    def test_variables_del_entorno_deshabilitadas(self) -> None:
        for nombre in ("habilitar_egreso_fcm", "egreso_fcm_az",
                       "egreso_fcm_cidr_publica", "egreso_fcm_cidr_https",
                       "egreso_fcm_prefix_list_id"):
            self.assertIn(f'variable "{nombre}"', self.env_vars, nombre)
        bloque = self.env_vars.split('variable "habilitar_egreso_fcm"', 1)[1].split("\n}", 1)[0]
        self.assertIn("default     = false", bloque)
        bloque = self.env_vars.split('variable "egreso_fcm_az"', 1)[1].split("\n}", 1)[0]
        self.assertIn('default     = "us-east-2a"', bloque)
        bloque = self.env_vars.split('variable "egreso_fcm_cidr_publica"', 1)[1].split("\n}", 1)[0]
        self.assertIn('default     = "10.42.16.0/24"', bloque)

    def test_todos_los_recursos_tienen_count(self) -> None:
        for recurso in re.findall(r'resource "([^"]+)" "([^"]+)"', self.main):
            tipo, nombre = recurso
            bloque = _bloque(self.main, f'resource "{tipo}" "{nombre}"')
            self.assertIn("count =", bloque, f"{tipo}.{nombre} sin count")
            self.assertIn("local.activo", bloque, f"{tipo}.{nombre} no depende de local.activo")

    def test_no_crea_ec2_ni_security_group_nuevo(self) -> None:
        self.assertNotIn('resource "aws_instance"', self.main)
        self.assertNotIn('resource "aws_security_group"', self.main)
        self.assertNotIn("from_port         = 22", self.main)
        self.assertNotIn("to_port           = 22", self.main)


class UnaSolaAzYSubredNueva(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(MODULO / "main.tf")
        cls.vars = _leer(MODULO / "variables.tf")

    def test_un_unico_nat(self) -> None:
        bloque = _bloque(self.main, 'resource "aws_nat_gateway" "nat"')
        self.assertIn("count = local.activo ? 1 : 0", bloque)
        self.assertIn("connectivity_type = \"public\"", bloque)
        self.assertNotIn("numero_nat_gateways", self.main)
        self.assertNotIn("numero_nat_gateways", self.vars)

    def test_subred_publica_nueva_sin_ip_publica_al_lanzar(self) -> None:
        bloque = _bloque(self.main, 'resource "aws_subnet" "publica_nat"')
        self.assertIn("cidr_block              = var.subred_publica_cidr", bloque)
        self.assertIn("availability_zone       = var.zona", bloque)
        self.assertIn("map_public_ip_on_launch = false", bloque)
        self.assertNotIn("map_public_ip_on_launch = true", self.main)

    def test_ruta_publica_al_igw_y_privada_al_nat(self) -> None:
        publica = _bloque(self.main, 'resource "aws_route" "publica_internet"')
        self.assertIn('destination_cidr_block = "0.0.0.0/0"', publica)
        self.assertIn("gateway_id             = var.internet_gateway_id", publica)
        self.assertNotIn("nat_gateway_id", publica)
        privada = _bloque(self.main, 'resource "aws_route" "privada_nat"')
        self.assertIn("nat_gateway_id         = aws_nat_gateway.nat[0].id", privada)
        self.assertIn("var.private_route_table_ids", privada)
        self.assertIn("length(var.private_route_table_ids)", privada)
        # Una ruta por cada tabla privada real de I-010, sin inventar una lista.
        self.assertIn("var.private_route_table_ids[count.index]", privada)

    def test_eip_es_para_nat_y_no_para_ec2(self) -> None:
        bloque = _bloque(self.main, 'resource "aws_eip" "nat"')
        self.assertIn('domain = "vpc"', bloque)
        nat = _bloque(self.main, 'resource "aws_nat_gateway" "nat"')
        self.assertIn("allocation_id     = aws_eip.nat[0].id", nat)
        self.assertIn("subnet_id         = aws_subnet.publica_nat[0].id", nat)

    def test_cidr_publico_es_slash_27_o_mayor(self) -> None:
        self.assertIn('tonumber(split("/", var.subred_publica_cidr)[1]) <= 27', self.main)

    def test_az_pertenece_a_region(self) -> None:
        self.assertIn('startswith(var.zona, "${var.region}")', self.main)


class Egreso443SinPrefixListInventada(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(MODULO / "main.tf")
        cls.vars = _leer(MODULO / "variables.tf")

    def test_regla_cidr_443(self) -> None:
        bloque = _bloque(self.main, 'resource "aws_vpc_security_group_egress_rule" "backend_https_cidr"')
        self.assertIn('from_port         = 443', bloque)
        self.assertIn('to_port           = 443', bloque)
        self.assertIn("cidr_ipv4         = var.cidr_egreso_https", bloque)
        self.assertIn("prefix_list_egreso_https_id == \"\"", bloque)

    def test_regla_prefix_list_es_opcional_y_real(self) -> None:
        bloque = _bloque(self.main, 'resource "aws_vpc_security_group_egress_rule" "backend_https_prefix_list"')
        self.assertIn("prefix_list_id    = var.prefix_list_egreso_https_id", bloque)
        self.assertIn("var.prefix_list_egreso_https_id != \"\"", bloque)
        self.assertIn('can(regex("^pl-[0-9a-f]+$", var.prefix_list_egreso_https_id))', self.main)

    def test_no_se_declara_una_prefix_list_inexistente(self) -> None:
        self.assertNotIn("data \"aws_ec2_managed_prefix_list\"", self.main)
        # No se escribe ningún ID `pl-*` concreto: el único `pl-` admitido es el
        # patrón de validación `^pl-[0-9a-f]+$`. Antes se prohibía la palabra
        # `google`, pero los comentarios que explican qué endpoints se alcanzan
        # la mencionan legítimamente; la comprobación real es que no se inventa
        # un recurso ni un identificador.
        self.assertNotRegex(self.main, r"pl-[0-9a-f]{6,}")
        self.assertIn('default     = ""', self.vars.split('variable "prefix_list_egreso_https_id"', 1)[1].split("\n}", 1)[0])
        self.assertIn('default     = "0.0.0.0/0"', self.vars.split('variable "cidr_egreso_https"', 1)[1].split("\n}", 1)[0])

    def test_una_sola_regla_de_egreso_443(self) -> None:
        self.assertEqual(self.main.count("aws_vpc_security_group_egress_rule\" \"backend_https"), 2)


class DnsYVecindad(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.red = _leer(RED / "main.tf")
        cls.main = _leer(MODULO / "main.tf")
        cls.outputs = _leer(MODULO / "outputs.tf")
        cls.readme = _leer(MODULO / "README.md")

    def test_vpc_tiene_dns_support_y_hostnames(self) -> None:
        self.assertIn("enable_dns_support   = true", self.red)
        self.assertIn("enable_dns_hostnames = true", self.red)

    def test_entorno_pasa_igw_y_route_table_reales(self) -> None:
        env = _leer(ENVS / "main.tf")
        # `terraform fmt` alinea los `=` con espacios variables; se compara por
        # expresión y no por columnas exactas para que el formateo no rompa la
        # prueba.
        for patron, descripcion in (
            (r"internet_gateway_id\s*=\s*module\.red\.internet_gateway_id", "IGW real"),
            (r"private_route_table_ids\s*=\s*\[module\.red\.route_table_privada\]", "route table privada"),
            (r"backend_security_group_id\s*=\s*module\.red\.security_groups\[\"backend\"\]", "SG del backend"),
        ):
            self.assertRegex(env, patron, descripcion)

    def test_outputs_requeridos(self) -> None:
        for salida in ("nat_gateway_id", "eip", "private_route_table_ids", "public_subnet_id",
                       "regla_egreso_id", "estado"):
            self.assertIn(f'output "{salida}"', self.outputs, salida)
        bloque = _bloque(self.outputs, 'output "eip"')
        self.assertIn("public_ip", bloque)
        self.assertIn("allocation_id", bloque)

    def test_documenta_spof_y_topologia_privada(self) -> None:
        readme = self.readme.lower()
        self.assertIn("punto único de fallo", readme)
        self.assertIn("no crea ec2", readme)
        self.assertIn("alb sigue interno", readme)
        self.assertIn("map_public_ip_on_launch = false", self.main)

    def test_sin_secretos_ni_estado(self) -> None:
        texto = "\n".join(_leer(MODULO / n) for n in ("main.tf", "variables.tf", "outputs.tf", "README.md"))
        for prohibido in ("BEGIN RSA", "AWS_SECRET", "password", "tfstate", "client_secret"):
            self.assertNotIn(prohibido, texto)


if __name__ == "__main__":
    unittest.main()
