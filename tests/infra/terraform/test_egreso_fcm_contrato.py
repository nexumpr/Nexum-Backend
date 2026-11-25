"""Contrato del módulo `egreso_fcm` (propuesta, deshabilitado por defecto).

Comprueba que la preparación no crea recursos, no abre SSH, no asigna IPv4
pública al lanzar instancias y no enruta Internet salvo por el NAT opcional.
"""

from __future__ import annotations

import re
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
MODULO = RAIZ / "infra" / "terraform" / "modules" / "egreso_fcm"
ENVS = RAIZ / "infra" / "terraform" / "envs" / "h08i"


def _leer(p: Path) -> str:
    return p.read_text(encoding="utf-8")


class PorDefectoDeshabilitado(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(MODULO / "main.tf")
        cls.vars = _leer(MODULO / "variables.tf")
        cls.outputs = _leer(MODULO / "outputs.tf")

    def test_habilitar_false_por_defecto(self) -> None:
        bloque = self.vars.split('variable "habilitar"', 1)[1].split("}", 1)[0]
        self.assertIn("default     = false", bloque)

    def test_no_instanciado_en_el_entorno(self) -> None:
        for archivo in ENVS.glob("*.tf"):
            self.assertNotIn('module "egreso_fcm"', _leer(archivo), archivo.name)

    def test_recursos_condicionados_a_habilitar(self) -> None:
        for recurso in re.findall(r'resource "([^"]+)" "([^"]+)"', self.main):
            tipo, nombre = recurso
            if tipo == "terraform_data":
                continue
            bloque = self.main.split(f'resource "{tipo}" "{nombre}"', 1)[1].split("\n}", 1)[0]
            self.assertIn("count =", bloque, f"{tipo}.{nombre} sin count")
            self.assertIn("local.activo", bloque, f"{tipo}.{nombre} no depende de local.activo")

    def test_no_crea_ec2_ni_security_group_nuevo(self) -> None:
        self.assertNotIn('resource "aws_instance"', self.main)
        self.assertNotIn('resource "aws_security_group"', self.main)
        self.assertNotIn("from_port         = 22", self.main)
        self.assertNotIn("to_port           = 22", self.main)

    def test_no_asigna_ip_publica_al_lanzar(self) -> None:
        self.assertIn("map_public_ip_on_launch = false", self.main)
        self.assertNotIn("map_public_ip_on_launch = true", self.main)

    def test_eip_es_para_nat_y_no_para_ec2(self) -> None:
        self.assertIn('resource "aws_eip" "nat"', self.main)
        self.assertIn('domain = "vpc"', self.main)


class RutaYReglaOpcional(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(MODULO / "main.tf")
        cls.vars = _leer(MODULO / "variables.tf")

    def test_ruta_publica_solo_al_igw(self) -> None:
        bloque = self.main.split('resource "aws_route" "publica_internet"', 1)[1].split("\n}", 1)[0]
        self.assertIn('destination_cidr_block = "0.0.0.0/0"', bloque)
        self.assertIn("gateway_id             = var.internet_gateway_id", bloque)
        self.assertNotIn("nat_gateway_id", bloque)

    def test_ruta_privada_al_nat(self) -> None:
        bloque = self.main.split('resource "aws_route" "privada_nat"', 1)[1].split("\n}", 1)[0]
        self.assertIn("nat_gateway_id", bloque)
        self.assertIn("var.private_route_table_ids", bloque)
        self.assertIn("var.numero_nat_gateways", bloque)

    def test_regla_443_es_opcional_y_declarada_como_riesgo(self) -> None:
        bloque = self.main.split('resource "aws_vpc_security_group_egress_rule" "backend_https_internet"', 1)[1].split("\n}", 1)[0]
        self.assertIn("var.habilitar_regla_egreso_backend", bloque)
        self.assertIn('from_port         = 443', bloque)
        self.assertIn('to_port           = 443', bloque)
        self.assertIn('cidr_ipv4         = "0.0.0.0/0"', bloque)
        self.assertIn("riesgo residual", bloque)
        self.assertIn("Sin allowlist de dominio", bloque)

    def test_una_sola_regla_de_egreso_amplia(self) -> None:
        self.assertEqual(self.main.count('cidr_ipv4         = "0.0.0.0/0"'), 1)

    def test_preconditions_de_nat_y_tablas(self) -> None:
        self.assertIn("length(var.public_subnet_cidrs) >= var.numero_nat_gateways", self.main)
        self.assertIn("length(var.availability_zones) >= var.numero_nat_gateways", self.main)
        self.assertIn("length(var.private_route_table_ids) >= 1", self.main)
        self.assertIn("numero_nat_gateways == 1 || length(var.private_route_table_ids) >= var.numero_nat_gateways", self.main)
        self.assertIn("var.internet_gateway_id != \"\"", self.main)

    def test_numero_de_nat_acotado(self) -> None:
        bloque = self.vars.split('variable "numero_nat_gateways"', 1)[1]
        self.assertIn("contains([1, 2], var.numero_nat_gateways)", bloque)

    def test_sin_secretos_ni_estado(self) -> None:
        texto = "\n".join(_leer(MODULO / n) for n in ("main.tf", "variables.tf", "outputs.tf", "README.md"))
        for prohibido in ("BEGIN RSA", "AWS_SECRET", "password", "tfstate", "client_secret"):
            self.assertNotIn(prohibido, texto)


if __name__ == "__main__":
    unittest.main()
