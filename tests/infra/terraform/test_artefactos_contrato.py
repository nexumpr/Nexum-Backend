"""Contrato del modulo `artefactos` (ARTIFACT_DELIVERY, 2026-11-09).

Analiza la politica real y verifica el cableado sin AWS. Las semantica de las
sentencias (Deny/Allow, condiciones, principal y recurso) se evalua con el mismo
motor ya probado en `test_secretos_contrato`, que cubre estas mismas formas.
"""

from __future__ import annotations

import re
import unittest
from pathlib import Path

from test_secretos_contrato import _bloques

RAIZ = Path(__file__).resolve().parents[3]
MODULO = RAIZ / "infra" / "terraform" / "modules" / "artefactos"
ENVS = RAIZ / "infra" / "terraform" / "envs" / "h08i"


def _leer(ruta: Path) -> str:
    return ruta.read_text(encoding="utf-8")


def _sin_comentarios(texto: str) -> str:
    return "\n".join(l for l in texto.splitlines() if not l.lstrip().startswith("#"))


class BucketDeArtefactos(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _sin_comentarios(_leer(MODULO / "main.tf"))
        cls.variables = _leer(MODULO / "variables.tf")
        cls.outputs = _leer(MODULO / "outputs.tf")

    def test_bucket_privado_cifrado_y_versionado(self) -> None:
        self.assertIn("force_destroy = false", self.main)
        for clave in (
            "block_public_acls       = true",
            "block_public_policy     = true",
            "ignore_public_acls      = true",
            "restrict_public_buckets = true",
        ):
            self.assertIn(clave, self.main, clave)
        self.assertIn('object_ownership = "BucketOwnerEnforced"', self.main)
        self.assertIn('status = "Enabled"', self.main)
        self.assertIn('sse_algorithm = "AES256"', self.main)
        self.assertIn("noncurrent_version_expiration", self.main)

    def test_seis_sentencias_con_orden_de_responsabilidades(self) -> None:
        sids = re.findall(r'sid\s*=\s*"([A-Za-z]+)"', self.main)
        self.assertEqual(
            sids,
            [
                "DenegarSinTLS",
                "DenegarEscrituraAlRuntime",
                "DenegarLecturaAlPublicador",
                "DenegarBorradoAlPublicador",
                "DenegarPublicacionSinSSE",
                "PermitirPublicacionAlPublicador",
                "DenegarAPrincipalsNoAutorizados",
                "DenegarListBucket",
                "DenegarListadoDeVersionesYMultipart",
            ],
        )

    def test_runtime_no_publica_y_publicador_solo_escribe(self) -> None:
        sentencias = {sid: cuerpo for sid, cuerpo in zip(
            re.findall(r'sid\s*=\s*"([A-Za-z]+)"', self.main),
            re.findall(r"sid\s*=.*?(?=sid\s*=|\Z)", self.main, re.DOTALL),
        )}
        runtime = sentencias["DenegarEscrituraAlRuntime"]
        self.assertIn('effect = "Deny"', runtime)
        self.assertIn("s3:PutObject", runtime)
        self.assertIn("s3:DeleteObject", runtime)
        publicador = sentencias["PermitirPublicacionAlPublicador"]
        self.assertIn('effect = "Allow"', publicador)
        self.assertIn('actions   = ["s3:PutObject"]', publicador)
        self.assertIn('"${aws_s3_bucket.artefactos.arn}/${var.prefijo}*"', publicador)
        self.assertIn('variable = "s3:x-amz-server-side-encryption"', publicador)

    def test_terceros_denegados_y_listbucket_solo_para_iac(self) -> None:
        terceros = [s for s in _bloques(self.main, "statement") if "DenegarAPrincipalsNoAutorizados" in s][0]
        self.assertIn('variable = "aws:PrincipalArn"', terceros)
        self.assertIn('test     = "StringNotEquals"', terceros)
        self.assertIn("var.rol_runtime_arn, var.publicador_arn]", terceros)
        listado = [s for s in _bloques(self.main, "statement") if "DenegarListBucket" in s][0]
        self.assertIn('test     = "ArnNotEquals"', listado)
        self.assertIn("var.gestor_iac_arn", listado)

    def test_preconditions_fail_closed(self) -> None:
        self.assertIn("length(distinct([var.rol_runtime_arn, var.publicador_arn, var.gestor_iac_arn])) == 3", self.main)
        self.assertIn("publicador_arn debe ser un ARN de rol IAM", self.main)
        self.assertIn('endswith(var.prefijo, "/")', self.main)
        self.assertIn("var.retencion_version_dias >= 1", self.main)

    def test_outputs_para_el_launch_template(self) -> None:
        for salida in ("bucket_name", "bucket_arn", "objetos_arn", "uri_base"):
            self.assertIn(f'output "{salida}"', self.outputs, salida)
        self.assertIn('"${aws_s3_bucket.artefactos.arn}/${var.prefijo}*"', self.outputs)
        self.assertIn('"s3://${aws_s3_bucket.artefactos.bucket}/${var.prefijo}"', self.outputs)


    def test_prefijo_unico_entre_modulo_cd_y_verificador(self) -> None:
        # El prefijo debe ser identico en bucket, IAM, LT, verificador y bootstrap.
        variables = _leer(MODULO / "variables.tf")
        self.assertIn('default     = "releases/"', variables)
        self.assertIn('prefijo = "releases/"', re.sub(r"[ \t]+", " ", _leer(ENVS / "main.tf")))
        self.assertIn("/releases/*", _leer(ENVS / "main.tf"))
        verificador = _leer(RAIZ / "herramientas" / "verificar_lt_release.py")
        self.assertIn('releases/{release_id}/nexum-backend-linux-arm64', verificador)
        cd = _leer(RAIZ / ".github" / "workflows" / "cd-backend.yml")
        self.assertIn('CLAVE="releases/$IN_RELEASE_ID/nexum-backend-linux-arm64"', cd)

    def test_cd_exige_sse_al_publicar(self) -> None:
        cd = _leer(RAIZ / ".github" / "workflows" / "cd-backend.yml")
        self.assertIn("--server-side-encryption AES256", cd)
        self.assertIn('--if-none-match "*"', cd)

    def test_publicador_no_lee_no_borra_y_exige_sse(self) -> None:
        sids = re.findall(r'sid\s*=\s*"([A-Za-z]+)"', self.main)
        for sid in ("DenegarLecturaAlPublicador", "DenegarBorradoAlPublicador", "DenegarPublicacionSinSSE"):
            self.assertIn(sid, sids, sid)
        self.assertIn('variable = "s3:x-amz-server-side-encryption"', self.main)


class CableadoYNombreDeterminista(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.envs = _leer(ENVS / "main.tf")
        cls.variables = _leer(ENVS / "variables.tf")
        cls.modulo = _leer(MODULO / "main.tf")

    def test_modulo_cableado(self) -> None:
        self.assertIn('module "artefactos"', self.envs)
        self.assertIn('source = "../../modules/artefactos"', self.envs)
        compacto = re.sub(r"[ \t]+", " ", self.envs)
        self.assertIn("rol_runtime_arn = module.iam.runtime_role_arn", compacto)

    def test_nombre_determinista_coincide_con_el_modulo(self) -> None:
        # El entorno replica el nombre para evitar el ciclo artefactos -> iam -> artefactos.
        self.assertIn(
            'lower("nexum-${var.entorno}-artefactos-${data.aws_caller_identity.para_artefactos.account_id}")',
            self.envs,
        )
        self.assertIn(
            'lower("${var.proyecto}-${var.entorno}-artefactos-${data.aws_caller_identity.actual.account_id}")',
            self.modulo,
        )

    def test_runtime_recibe_el_prefijo_de_releases(self) -> None:
        compacto = re.sub(r"[ \t]+", " ", self.envs)
        self.assertIn("artifact_object_arns", compacto)
        self.assertIn("arn:aws:s3:::${local.artefactos_bucket}/releases/*", compacto)

    def test_publicador_por_variable_fail_closed(self) -> None:
        self.assertIn('variable "artefactos_publicador_arn"', self.variables)
        self.assertIn("default     = \"\"", self.variables)

    def test_sin_secretos_en_el_modulo(self) -> None:
        texto = _leer(MODULO / "main.tf") + _leer(MODULO / "variables.tf")
        for prohibido in ("password", "PRIVATE KEY", "BEGIN RSA"):
            self.assertNotIn(prohibido, texto, prohibido)


if __name__ == "__main__":
    unittest.main()
