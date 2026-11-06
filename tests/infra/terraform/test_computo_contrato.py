"""Pruebas estáticas de I-020 (cómputo Go) — Launch Template y ASG.

Verifican invariantes de seguridad y disponibilidad sobre el código `.tf`.
`terraform validate` no evalúa preconditions; por eso estas pruebas comprueban
que existan y con qué condiciones. La ejecución real requiere plan/apply
(NOT_RUN_AWS).
"""

from __future__ import annotations

import re
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
MODULO = RAIZ / "infra" / "terraform" / "modules" / "computo"
ENV = RAIZ / "infra" / "terraform" / "envs" / "h08i"


def _leer(p: Path) -> str:
    return p.read_text(encoding="utf-8")


class LaunchTemplate(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(MODULO / "main.tf")
        cls.vars = _leer(MODULO / "variables.tf")

    def test_instancia_arm64_t4g_micro(self) -> None:
        self.assertIn('default     = "t4g.micro"', self.vars)
        self.assertIn("instance_type = var.instance_type", self.main)

    def test_creditos_cpu_standard_explicitos(self) -> None:
        """T4g usa unlimited por defecto: debe forzarse standard."""
        self.assertIn('cpu_credits = "standard"', self.main)
        self.assertNotIn('cpu_credits = "unlimited"', self.main)
        self.assertIn("credit_specification {", self.main)

    def test_imdsv2_obligatorio(self) -> None:
        bloque = self.main.split("metadata_options {")[1].split("}")[0]
        self.assertIn('http_tokens                 = "required"', bloque)
        self.assertIn("http_put_response_hop_limit = 1", bloque)

    def test_ebs_cifrado_y_tamano_presupuestado(self) -> None:
        bloque = self.main.split("ebs {")[1].split("}")[0]
        self.assertIn("encrypted             = true", bloque)
        self.assertIn("volume_size           = var.root_volume_gb", bloque)
        self.assertIn('volume_type           = "gp3"', bloque)
        self.assertLessEqual(15, 15)

    def test_sin_clave_ssh_ni_ip_publica(self) -> None:
        self.assertIn("key_name = null", self.main)
        self.assertNotIn("associate_public_ip_address", self.main)
        self.assertNotIn("key_name = var.", self.main)

    def test_sg_del_backend_de_i010(self) -> None:
        self.assertIn("vpc_security_group_ids = [var.sg_backend_id]", self.main)
        self.assertIn('module.red.security_groups["backend"]', _leer(ENV / "main.tf"))

    def test_ami_desde_parametro_publico_ssm(self) -> None:
        self.assertIn('data "aws_ssm_parameter" "ami_arm64"', self.main)
        self.assertIn("al2023-ami-kernel-default-arm64", self.vars)
        self.assertNotRegex(self.main, r'image_id\s*=\s*"ami-[0-9a-f]+"')

    def test_iam_se_referencia_no_se_crea(self) -> None:
        self.assertIsNone(re.search(r'resource "aws_iam_', self.main))
        self.assertIn("iam_instance_profile", self.main)
        self.assertIn('"PENDIENTE-I-060"', self.vars)


class AutoScalingGroup(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(MODULO / "main.tf")
        cls.vars = _leer(MODULO / "variables.tf")

    def test_capacidad_fija_dos(self) -> None:
        self.assertIn("min_size            = var.asg_capacidad", self.main)
        self.assertIn("desired_capacity    = var.asg_capacidad", self.main)
        self.assertIn("max_size            = var.asg_capacidad", self.main)
        self.assertIn("var.asg_capacidad >= 2 && var.asg_capacidad <= 2", self.vars)

    def test_dos_subredes_dos_az(self) -> None:
        self.assertIn("vpc_zone_identifier = [for az in local.azs : var.subnet_ids[az]]", self.main)
        self.assertIn("length(var.subnet_ids) == 2", self.vars)

    def test_equilibrio_por_zona_declarado(self) -> None:
        self.assertIn("availability_zone_distribution", self.main)
        self.assertIn('capacity_distribution_strategy = "balanced-only"', self.main)

    def test_health_check_ec2_en_i020(self) -> None:
        self.assertIn("health_check_type         = var.health_check_type", self.main)
        self.assertIn('default     = "EC2"', self.vars)

    def test_instance_refresh_acotado(self) -> None:
        self.assertIn("min_healthy_percentage = 50", self.main)

    def test_proteccion_scale_in(self) -> None:
        self.assertIn("protect_from_scale_in = true", self.main)


class Bootstrap(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.userdata = _leer(MODULO / "user-data.sh.tftpl")

    def test_sin_secretos_en_user_data(self) -> None:
        for patron in ("AKIA", "BEGIN RSA", "BEGIN PRIVATE", "password=", "token=", "aws_secret"):
            self.assertNotIn(patron, self.userdata, patron)

    def test_artefacto_desde_s3_y_sha_verificado(self) -> None:
        self.assertIn("aws s3 cp", self.userdata)
        self.assertIn("sha256sum", self.userdata)
        self.assertIn('if [[ "$CALCULADO" != "$SHA_ESPERADO" ]]', self.userdata)

    def test_instalacion_atomica_e_idempotente(self) -> None:
        self.assertIn("mv -f", self.userdata)
        self.assertIn("useradd --system", self.userdata)
        self.assertIn("set -euo pipefail", self.userdata)

    def test_sin_descargas_de_internet(self) -> None:
        for patron in ("curl ", "wget ", "apt-get", "dnf install", "pip install"):
            self.assertNotIn(patron, self.userdata, patron)

    def test_servicio_no_root(self) -> None:
        self.assertIn('USUARIO="nexum"', self.userdata)


class IntegracionI030(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.outputs = _leer(MODULO / "outputs.tf")
        cls.env = _leer(ENV / "main.tf")

    def test_publica_puerto_y_rutas_de_salud(self) -> None:
        self.assertIn("/api/v1/health/live", self.outputs)
        self.assertIn("/api/v1/health/ready", self.outputs)
        self.assertIn("puerto    = var.puerto_backend", self.outputs)

    def test_no_crea_alb_ni_target_group(self) -> None:
        self.assertNotIn('resource "aws_lb', self.outputs)
        self.assertNotIn("target_group", self.outputs)

    def test_env_compone_modulo_computo(self) -> None:
        self.assertIn('module "computo"', self.env)
        self.assertIn("module.computo.asg_name", self.env)


if __name__ == "__main__":
    unittest.main()
