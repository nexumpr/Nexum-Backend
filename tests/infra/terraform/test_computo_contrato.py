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

    def test_sg_validacion_estricta(self) -> None:
        # No debe aceptarse cualquier cadena no vacia.
        self.assertNotIn('|| var.sg_backend_id != ""', self.vars)
        self.assertIn('can(regex("^sg-[0-9a-f]{8,17}$", var.sg_backend_id))', self.vars)

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

    def test_instance_refresh_no_baja_de_dos(self) -> None:
        self.assertIn("min_healthy_percentage = 100", self.main)
        self.assertIn("max_healthy_percentage = 200", self.main)
        self.assertNotIn("min_healthy_percentage = 50", self.main)

    def test_proteccion_scale_in(self) -> None:
        self.assertIn("protect_from_scale_in = true", self.main)


class Bootstrap(unittest.TestCase):
    """Cadena completa: binario + unidad + config + arranque, con fallo explicito."""

    @classmethod
    def setUpClass(cls) -> None:
        cls.userdata = _leer(MODULO / "user-data.sh.tftpl")

    def test_sin_secretos_en_user_data(self) -> None:
        for patron in ("AKIA", "BEGIN RSA", "BEGIN PRIVATE", "password=", "token=", "aws_secret"):
            self.assertNotIn(patron, self.userdata, patron)

    def test_descarga_binario_y_unidad_con_sha(self) -> None:
        self.assertIn('descargar "$BINARIO_URI"', self.userdata)
        self.assertIn('descargar "$UNIDAD_URI"', self.userdata)
        self.assertIn("sha256sum", self.userdata)
        self.assertIn('[[ "$calculado" == "$sha_esperado" ]]', self.userdata)

    def test_instala_unidad_systemd(self) -> None:
        self.assertIn('install -m 0644 -o root -g root "$TMP/$SERVICIO.service"', self.userdata)
        self.assertIn("/etc/systemd/system/$SERVICIO.service", self.userdata)
        self.assertIn("systemctl daemon-reload", self.userdata)
        self.assertIn('systemctl enable "$SERVICIO"', self.userdata)
        self.assertIn('systemctl restart "$SERVICIO"', self.userdata)

    def test_binario_propiedad_de_root(self) -> None:
        self.assertIn('install -m 0755 -o root -g root "$TMP/nexum-backend"', self.userdata)
        self.assertIn('install -d -m 0755 -o root -g root "$DIR_BIN"', self.userdata)
        self.assertNotIn('-o "$USUARIO" -g "$USUARIO" "$TMP/nexum-backend"', self.userdata)

    def test_falla_si_falta_configuracion(self) -> None:
        self.assertIn('if [[ ! -f "$ENV_FILE" ]]', self.userdata)
        self.assertIn("fail ", self.userdata)
        self.assertIn("exit 1", self.userdata)

    def test_comprueba_servicio_activo_y_liveness(self) -> None:
        self.assertIn('systemctl is-active --quiet "$SERVICIO"', self.userdata)
        self.assertIn("/api/v1/health/live", self.userdata)
        self.assertIn('fail "el servicio no alcanzo estado activo', self.userdata)

    def test_sin_exito_silencioso(self) -> None:
        # El bootstrap nunca debe terminar con AVISO por falta de unidad.
        self.assertNotIn("AVISO: unidad", self.userdata)
        self.assertNotIn("sin activar", self.userdata)

    def test_instalacion_atomica_e_idempotente(self) -> None:
        self.assertIn("mv -f", self.userdata)
        self.assertIn("useradd --system", self.userdata)
        self.assertIn("set -euo pipefail", self.userdata)

    def test_verifica_herramientas_de_la_ami(self) -> None:
        for cmd in ("aws", "sha256sum", "systemctl", "curl"):
            self.assertIn(cmd, self.userdata)

    def test_sin_descargas_de_internet(self) -> None:
        for patron in ("curl http://", "curl https://", "wget ", "apt-get", "dnf install", "pip install"):
            self.assertNotIn(patron, self.userdata, patron)


class ScriptDeBuild(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.ps1 = (RAIZ / "scripts" / "build-backend.ps1").read_text(encoding="utf-8")

    def test_compila_linux_arm64_sin_cgo(self) -> None:
        for fragmento in ('GOOS = "linux"', 'GOARCH = "arm64"', 'CGO_ENABLED = "0"', "-trimpath"):
            self.assertIn(fragmento, self.ps1, fragmento)

    def test_restaura_variables_de_entorno(self) -> None:
        self.assertIn("$previo", self.ps1)
        self.assertIn("Set-Item \"Env:$clave\"", self.ps1)

    def test_preflight_anti_sobrescritura(self) -> None:
        self.assertIn("ya existe", self.ps1)
        self.assertIn("-Force", self.ps1)

    def test_integridad_y_manifiesto(self) -> None:
        self.assertIn("Get-FileHash -Algorithm SHA256", self.ps1)
        self.assertIn("manifest.json", self.ps1)
        self.assertIn("recalculado", self.ps1)

    def test_sin_secretos(self) -> None:
        for patron in ("AKIA", "BEGIN PRIVATE", "password=", "aws_secret"):
            self.assertNotIn(patron, self.ps1, patron)


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
