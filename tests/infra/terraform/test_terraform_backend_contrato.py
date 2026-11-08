"""Pruebas estructurales de I-070 (bootstrap de backend remoto y condiciones IaC).

No consultan AWS ni ejecutan plan/apply. Verifican las protecciones declaradas
sobre el state, la tabla de lock y la plantilla de backend; la aplicación real
sigue NOT_RUN y el backend remoto permanece inactivo.
"""

from __future__ import annotations

import re
import unittest
from pathlib import Path

from test_balanceo_contrato import _leer, _sin_comentarios, atributos, recursos

RAIZ = Path(__file__).resolve().parents[3]
BOOTSTRAP = RAIZ / "infra" / "terraform" / "bootstrap"
ENV = RAIZ / "infra" / "terraform" / "envs" / "h08i"


class BucketDeState(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(BOOTSTRAP / "main.tf")
        cls.vars = _leer(BOOTSTRAP / "variables.tf")
        cls.outs = _leer(BOOTSTRAP / "outputs.tf")
        cls.r = recursos(cls.main)

    def test_state_cifrado_versionado_y_privado(self) -> None:
        self.assertIn(("aws_s3_bucket_versioning", "state"), self.r)
        self.assertIn('status = "Enabled"', self.main)
        self.assertIn(("aws_s3_bucket_server_side_encryption_configuration", "state"), self.r)
        self.assertIn('sse_algorithm = "AES256"', self.main)
        self.assertNotIn("aws_kms_key", {t for t, _ in self.r})
        for flag in (
            "block_public_acls       = true",
            "block_public_policy     = true",
            "ignore_public_acls      = true",
            "restrict_public_buckets = true",
        ):
            self.assertIn(flag, self.main)

    def test_politica_exige_tls(self) -> None:
        self.assertIn(("aws_s3_bucket_policy", "state"), self.r)
        self.assertIn('"aws:SecureTransport" = "false"', self.main)
        self.assertIn('Effect    = "Deny"', self.main)

    def test_state_no_se_destruye_por_ahorro(self) -> None:
        bucket = self.r[("aws_s3_bucket", "state")]
        self.assertIn("prevent_destroy = true", bucket)
        lock = self.r[("aws_dynamodb_table", "lock")]
        self.assertIn("prevent_destroy = true", lock)

    def test_lifecycle_acota_versiones(self) -> None:
        self.assertIn(("aws_s3_bucket_lifecycle_configuration", "state"), self.r)
        self.assertIn("noncurrent_version_expiration", self.main)
        self.assertIn("dias_retencion_versiones", self.main)


class TablaDeLock(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(BOOTSTRAP / "main.tf")
        cls.r = recursos(cls.main)
        cls.lock = cls.r[("aws_dynamodb_table", "lock")]

    def test_lock_key_y_on_demand(self) -> None:
        self.assertIn('hash_key     = "LockID"', self.lock)
        self.assertIn('billing_mode = "PAY_PER_REQUEST"', self.lock)
        self.assertIn('name = "LockID"', self.lock)

    def test_lock_sin_pitr_ni_backups(self) -> None:
        self.assertIn("point_in_time_recovery {\n    enabled = false", self.lock) if False else None
        self.assertRegex(self.lock, r"point_in_time_recovery\s*\{\s*enabled = false")
        self.assertNotIn("snapshot", self.lock.lower())

    def test_lock_no_es_la_tabla_de_negocio(self) -> None:
        self.assertIn('lock   = local.nombre_lock', self.main)
        self.assertIn('nombre_lock   = "${var.proyecto}-${var.entorno}-tf-lock"', self.main)
        # El comentario menciona NEXUM_CORE para aclarar que no es esa tabla.
        self.assertNotIn('name         = "NEXUM_CORE', self.main)


class BackendRemotoInactivo(unittest.TestCase):
    def test_entorno_sin_bloque_backend_activo(self) -> None:
        env = _leer(ENV / "main.tf")
        self.assertNotRegex(env, r'backend\s+"s3"')
        self.assertIn("Sin bloque backend", env)

    def test_plantilla_existe_y_esta_marcada_como_inactiva(self) -> None:
        plantilla = ENV / "backend.tf.example"
        self.assertTrue(plantilla.is_file())
        texto = _leer(plantilla)
        self.assertIn('backend "s3"', texto)
        self.assertIn("NO ACTIVA", texto)
        self.assertIn("encrypt        = true", texto)
        self.assertIn("dynamodb_table", texto)
        self.assertIn("PENDIENTE_BOOTSTRAP", texto)

    def test_outputs_documentan_backend_y_protecciones(self) -> None:
        outs = _leer(BOOTSTRAP / "outputs.tf")
        self.assertIn("configuracion_backend", outs)
        self.assertIn("prevent_destroy = true", outs)
        self.assertIn('value       = "NOT_RUN_AWS"', outs)


class CondicionesDeSeguridad(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(BOOTSTRAP / "main.tf")
        cls.vars = _leer(BOOTSTRAP / "variables.tf")

    def test_sin_auto_approve_ni_credenciales(self) -> None:
        texto = self.main + self.vars
        self.assertNotIn("auto-approve", texto)
        self.assertNotRegex(texto, r"AKIA[0-9A-Z]{16}")
        self.assertNotIn("aws_access_key", texto)

    def test_sufijo_de_cuenta_obligatorio_y_no_marcador(self) -> None:
        self.assertIn("startswith(var.sufijo_cuenta, \"PENDIENTE\")", self.main)
        self.assertIn('default     = ""', self.vars.split('variable "sufijo_cuenta"')[1].split("}")[0])

    def test_region_fija_y_bucket_valido(self) -> None:
        self.assertIn('var.region == "us-east-2"', self.main)
        self.assertIn('default     = "us-east-2"', self.vars)

    def test_no_crea_recursos_de_otras_specs(self) -> None:
        tipos = {t for t, _ in recursos(self.main)}
        for prohibido in (
            "aws_vpc",
            "aws_instance",
            "aws_autoscaling_group",
            "aws_lb",
            "aws_cloudfront_distribution",
            "aws_elasticache_replication_group",
            "aws_iam_role",
            "aws_kms_key",
        ):
            self.assertNotIn(prohibido, tipos)


class DocumentosObligatorios(unittest.TestCase):
    def test_entregables_de_i070_existen(self) -> None:
        for nombre in ("BLUEPRINT_MODULOS_TF.md", "MATRIZ_RECURSOS_STATE.md", "RUNBOOK_IAC.md"):
            ruta = RAIZ / "sdd" / "_control" / "H08" / nombre
            self.assertTrue(ruta.is_file(), nombre)
            self.assertGreater(ruta.stat().st_size, 500, nombre)

    def test_runbook_prohibe_autoapprove_y_exige_gate(self) -> None:
        texto = _leer(RAIZ / "sdd" / "_control" / "H08" / "RUNBOOK_IAC.md")
        self.assertIn("Gate", texto)
        self.assertIn("auto-approve", texto)
        self.assertIn("prevent_destroy", texto)


if __name__ == "__main__":
    unittest.main()
