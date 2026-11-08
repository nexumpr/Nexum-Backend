"""Pruebas estructurales de I-070 (bootstrap S3, locking nativo y condiciones IaC).

No consultan AWS ni ejecutan plan/apply. Verifican protecciones del state, el
mecanismo de locking declarado (S3 `use_lockfile`, sin DynamoDB) y el contrato IAM
de acceso mínimo al state y al fichero de lock. La aplicación real sigue NOT_RUN.
"""

from __future__ import annotations

import json
import re
import unittest
from pathlib import Path

from test_balanceo_contrato import _leer, _sin_comentarios, bloque, recursos

RAIZ = Path(__file__).resolve().parents[3]
BOOTSTRAP = RAIZ / "infra" / "terraform" / "bootstrap"
ENV = RAIZ / "infra" / "terraform" / "envs" / "h08i"
POLICY = RAIZ / "infra" / "terraform" / "policies" / "terraform-state-access.json.tftpl"
LOCKFILE = "envs/h08i/terraform.tfstate.tflock"


def _render_politica() -> dict:
    texto = _leer(POLICY)
    texto = texto.replace("${state_bucket}", "nexum-h08i-tfstate-ejemplo123")
    texto = texto.replace("${state_key}", "envs/h08i/terraform.tfstate")
    return json.loads(texto)


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
        self.assertIn('sse_algorithm = "AES256"', self.main)
        self.assertIn(("aws_s3_bucket_public_access_block", "state"), self.r)
        for flag in (
            "block_public_acls       = true",
            "block_public_policy     = true",
            "ignore_public_acls      = true",
            "restrict_public_buckets = true",
        ):
            self.assertIn(flag, self.main)

    def test_politica_exige_tls_y_ownership(self) -> None:
        self.assertIn(("aws_s3_bucket_policy", "state"), self.r)
        self.assertIn('"aws:SecureTransport" = "false"', self.main)
        self.assertIn('Effect    = "Deny"', self.main)
        self.assertIn('object_ownership = "BucketOwnerEnforced"', self.main)

    def test_state_no_se_destruye_por_ahorro(self) -> None:
        self.assertIn("prevent_destroy = true", self.r[("aws_s3_bucket", "state")])

    def test_lifecycle_acota_versiones(self) -> None:
        self.assertIn("noncurrent_version_expiration", self.main)
        self.assertIn("dias_retencion_versiones", self.main)


class LockingNativoS3(unittest.TestCase):
    """El locking con DynamoDB está deprecado: se exige S3 `use_lockfile`."""

    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(BOOTSTRAP / "main.tf")
        cls.outs = _leer(BOOTSTRAP / "outputs.tf")
        cls.example = _leer(ENV / "backend.tf.example")
        cls.env_main = _leer(ENV / "main.tf")
        cls.env_vars = _leer(ENV / "variables.tf")

    def test_no_se_crea_tabla_dynamodb_de_lock(self) -> None:
        tipos = {t for t, _ in recursos(self.main)}
        self.assertNotIn("aws_dynamodb_table", tipos)
        self.assertNotIn("LockID", self.main)

    def test_backend_usa_lockfile_y_no_dynamodb(self) -> None:
        self.assertIn("use_lockfile = true", self.example)
        self.assertNotIn("dynamodb_table", self.example)
        self.assertIn("encrypt      = true", self.example)

    def test_version_minima_permite_lock_nativo(self) -> None:
        self.assertIn('required_version = ">= 1.10.0, < 2.0.0"', self.env_main)
        self.assertIn('required_version = ">= 1.10.0, < 2.0.0"', self.main)
        self.assertIn("1.10", self.example)

    def test_outputs_declaran_mecanismo_y_archivo_de_lock(self) -> None:
        self.assertIn('mecanismo          = "S3 nativo (use_lockfile)"', self.outs)
        self.assertIn("terraform.tfstate.tflock", self.outs)
        self.assertIn("dynamodb_lock      = \"NO", self.outs)

    def test_backend_remoto_sigue_inactivo(self) -> None:
        self.assertNotRegex(self.env_main, r'backend\s+"s3"')
        self.assertIn("Sin bloque backend", self.env_main)
        self.assertTrue((ENV / "backend.tf.example").is_file())


class PoliticaDeAccesoAlState(unittest.TestCase):
    """Contrato IAM de mínimo privilegio para state y `.tflock` (diseño, no aplicado)."""

    @classmethod
    def setUpClass(cls) -> None:
        cls.doc = _render_politica()
        cls.statements = {s["Sid"]: s for s in cls.doc["Statement"]}

    def test_listado_acotado_al_prefijo_exacto(self) -> None:
        st = self.statements["ListOnlyStatePrefix"]
        self.assertEqual(st["Action"], "s3:ListBucket")
        self.assertEqual(st["Resource"], "arn:aws:s3:::nexum-h08i-tfstate-ejemplo123")
        self.assertEqual(st["Condition"]["StringEquals"]["s3:prefix"], "envs/h08i/terraform.tfstate")

    def test_state_solo_lectura_y_escritura(self) -> None:
        st = self.statements["ReadWriteStateObjectOnly"]
        self.assertEqual(sorted(st["Action"]), ["s3:GetObject", "s3:PutObject"])
        self.assertTrue(st["Resource"].endswith("/envs/h08i/terraform.tfstate"))
        self.assertNotIn("s3:DeleteObject", st["Action"])

    def test_lock_permite_delete_solo_en_tflock(self) -> None:
        st = self.statements["LockFileLifecycleOnly"]
        self.assertIn("s3:DeleteObject", st["Action"])
        self.assertTrue(st["Resource"].endswith("/" + LOCKFILE))

    def test_delete_del_state_denegado_explicitamente(self) -> None:
        st = self.statements["DenyStateDeletion"]
        self.assertEqual(st["Effect"], "Deny")
        self.assertIn("s3:DeleteObject", st["Action"])
        self.assertIn("s3:DeleteObjectVersion", st["Action"])
        self.assertFalse(st["Resource"].endswith(".tflock"))

    def test_tls_obligatorio_en_los_allow(self) -> None:
        for sid, st in self.statements.items():
            if st["Effect"] != "Allow":
                continue
            self.assertEqual(st["Condition"]["Bool"]["aws:SecureTransport"], "true", sid)

    def test_sin_wildcards_ni_dynamodb(self) -> None:
        texto = _leer(POLICY)
        self.assertNotIn('"s3:*"', texto)
        self.assertNotIn("dynamodb:", texto)
        self.assertNotIn('arn:aws:s3:::*', texto)
        for st in self.doc["Statement"]:
            recursos_st = st["Resource"] if isinstance(st["Resource"], list) else [st["Resource"]]
            for r in recursos_st:
                self.assertTrue(r.startswith("arn:aws:s3:::nexum-h08i-tfstate-ejemplo123"), r)

    def test_backend_no_contiene_credenciales(self) -> None:
        texto = _leer(ENV / "backend.tf.example")
        for prohibido in ("access_key", "secret_key", "AWS_ACCESS_KEY_ID", "token ="):
            self.assertNotIn(prohibido, texto)


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

    def test_sufijo_de_cuenta_obligatorio(self) -> None:
        self.assertIn('startswith(var.sufijo_cuenta, "PENDIENTE")', self.main)
        self.assertIn('default     = ""', self.vars.split('variable "sufijo_cuenta"')[1].split("}")[0])

    def test_region_fija(self) -> None:
        self.assertIn('var.region == "us-east-2"', self.main)

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

    def test_runbook_cubre_lock_huerfano_y_recuperacion(self) -> None:
        texto = _leer(RAIZ / "sdd" / "_control" / "H08" / "RUNBOOK_IAC.md")
        self.assertIn("tflock", texto)
        self.assertIn("force-unlock", texto)
        self.assertIn("prevent_destroy", texto)
        self.assertIn("auto-approve", texto)


if __name__ == "__main__":
    unittest.main()
