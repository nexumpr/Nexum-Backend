"""Contrato de RFC-004 opcion B: bundle de secretos en S3 (sin AWS).

Verifica el modulo `secretos`, el permiso IAM de solo lectura, el cableado del
entorno y el parser del bundle que ejecuta el bootstrap. No invoca AWS ni
Terraform: analiza el codigo y ejecuta el parser sobre fixtures.
"""

from __future__ import annotations

import json
import re
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
MODULOS = RAIZ / "infra" / "terraform" / "modules"
SECRETOS = MODULOS / "secretos"
IAM = MODULOS / "iam"
COMPUTO = MODULOS / "computo"
ENVS = RAIZ / "infra" / "terraform" / "envs" / "h08i"


def leer(ruta: Path) -> str:
    return ruta.read_text(encoding="utf-8")


def sin_comentarios(texto: str) -> str:
    return "\n".join(linea for linea in texto.splitlines() if not linea.lstrip().startswith("#"))


def bloque_parser() -> str:
    """Extrae el programa Python embebido en el user-data del bootstrap."""
    texto = leer(COMPUTO / "user-data.sh.tftpl")
    marca = texto.index("<<'PY'")
    inicio = texto.index("\n", marca) + 1
    fin = texto.index("\nPY\n", inicio)
    return texto[inicio:fin]


class ModuloSecretos(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = sin_comentarios(leer(SECRETOS / "main.tf"))
        cls.variables = leer(SECRETOS / "variables.tf")
        cls.outputs = leer(SECRETOS / "outputs.tf")

    def test_bucket_privado_y_sin_borrado_forzado(self) -> None:
        self.assertIn('resource "aws_s3_bucket" "secretos"', self.main)
        self.assertIn("force_destroy = false", self.main)
        self.assertNotIn("force_destroy = true", self.main)
        self.assertNotIn("acl ", self.main)

    def test_block_public_access_completo(self) -> None:
        for clave in (
            "block_public_acls       = true",
            "block_public_policy     = true",
            "ignore_public_acls      = true",
            "restrict_public_buckets = true",
        ):
            self.assertIn(clave, self.main, clave)

    def test_ownership_versionado_y_cifrado(self) -> None:
        self.assertIn('object_ownership = "BucketOwnerEnforced"', self.main)
        self.assertIn('status = "Enabled"', self.main)
        self.assertIn('sse_algorithm = "AES256"', self.main)

    def test_lifecycle_acota_versiones_no_actuales(self) -> None:
        self.assertIn("noncurrent_version_expiration", self.main)
        self.assertIn("noncurrent_days = var.retencion_version_dias", self.main)
        self.assertIn("abort_incomplete_multipart_upload", self.main)

    def _sentencias(self) -> dict[str, str]:
        """Devuelve el bloque de cada statement de la politica del bucket."""
        resultado: dict[str, str] = {}
        for bloque in re.findall(r"statement \{(.*?)\n  \}", self.main, re.DOTALL):
            sid = re.search(r'sid += +"([A-Za-z]+)"', bloque)
            if sid:
                resultado[sid.group(1)] = bloque
        return resultado

    def test_runtime_solo_lee_y_nunca_escribe(self) -> None:
        sentencias = self._sentencias()
        self.assertIn("DenegarEscrituraAlRuntime", sentencias)
        bloque = sentencias["DenegarEscrituraAlRuntime"]
        self.assertIn('effect = "Deny"', bloque)
        for accion in ("s3:PutObject", "s3:PutObjectAcl", "s3:DeleteObject", "s3:DeleteObjectVersion"):
            self.assertIn(accion, bloque, accion)
        self.assertIn("identifiers = [var.rol_runtime_arn]", bloque)
        self.assertNotIn("s3:GetObject", bloque)

    def test_publicador_no_queda_bloqueado_y_solo_publica_en_el_prefijo(self) -> None:
        sentencias = self._sentencias()
        self.assertIn("PermitirPublicacionAlPublicador", sentencias)
        bloque = sentencias["PermitirPublicacionAlPublicador"]
        self.assertIn('effect = "Allow"', bloque)
        self.assertIn('actions   = ["s3:PutObject"]', bloque)
        self.assertIn("identifiers = [var.publicador_arn]", bloque)
        self.assertIn('"${aws_s3_bucket.secretos.arn}/${var.prefijo}*"', bloque)
        # Ningun Deny puede alcanzar al publicador: solo el runtime tiene Deny nominado.
        for sid, contenido in sentencias.items():
            if 'effect = "Deny"' in contenido and sid != "DenegarSinTLS":
                self.assertNotIn("publicador_arn", contenido, sid)

    def test_publicador_exige_cifrado_al_escribir(self) -> None:
        bloque = self._sentencias()["PermitirPublicacionAlPublicador"]
        self.assertIn('variable = "s3:x-amz-server-side-encryption"', bloque)
        self.assertIn('values   = ["AES256"]', bloque)

    def test_sin_tls_se_deniega_para_todos(self) -> None:
        bloque = self._sentencias()["DenegarSinTLS"]
        self.assertIn('variable = "aws:SecureTransport"', bloque)
        self.assertIn('identifiers = ["*"]', bloque)
        self.assertIn('values   = ["false"]', bloque)

    def test_sin_publicador_el_plan_falla_cerrado(self) -> None:
        self.assertIn('variable "publicador_arn"', self.variables)
        self.assertIn('default     = ""', self.variables)
        self.assertIn("publicador_arn debe ser un ARN", self.main)
        self.assertIn("(role|user)", self.main)

    def test_preconditions_validan_prefijo_retencion_y_rol(self) -> None:
        self.assertIn('endswith(var.prefijo, "/")', self.main)
        self.assertIn("var.retencion_version_dias >= 1", self.main)
        self.assertIn("arn:aws:iam::[0-9]{12}:role/.+", self.main)

    def test_outputs_exponen_bucket_y_prefijo(self) -> None:
        for salida in ("bucket_name", "bucket_arn", "secrets_prefix_arn", "secrets_uri_base"):
            self.assertIn(f'output "{salida}"', self.outputs, salida)
        self.assertIn('"${aws_s3_bucket.secretos.arn}/${var.prefijo}*"', self.outputs)

    def test_sin_secretos_en_el_modulo(self) -> None:
        texto = leer(SECRETOS / "main.tf") + leer(SECRETOS / "variables.tf")
        for prohibido in ("pass" + "word", "secret_key", "PRIVATE KEY", "AWS_SECRET_ACCESS_KEY"):
            self.assertNotIn(prohibido, texto, prohibido)


class PermisoDeInstancia(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.iam = sin_comentarios(leer(IAM / "main.tf"))
        cls.variables = leer(IAM / "variables.tf")

    def test_policy_existe_y_es_condicional(self) -> None:
        self.assertIn('resource "aws_iam_role_policy" "runtime_secretos"', self.iam)
        self.assertIn("count = length(var.secretos_object_arns) > 0 ? 1 : 0", self.iam)

    def test_solo_lectura_sobre_el_prefijo(self) -> None:
        inicio = self.iam.index('resource "aws_iam_role_policy" "runtime_secretos"')
        tramo = self.iam[inicio : inicio + 900]
        self.assertIn('Action   = ["s3:GetObject"]', tramo)
        self.assertIn("Resource = var.secretos_object_arns", tramo)
        for prohibido in ("s3:PutObject", "s3:DeleteObject", "s3:*", '"*"'):
            self.assertNotIn(prohibido, tramo, prohibido)

    def test_variable_rechaza_el_bucket_completo(self) -> None:
        self.assertIn('variable "secretos_object_arns"', self.variables)
        self.assertIn('default     = []', self.variables)
        self.assertIn("^arn:aws:s3:::[a-z0-9.-]+/.+$", self.variables)


class CableadoDelEntorno(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = leer(ENVS / "main.tf")
        cls.variables = leer(ENVS / "variables.tf")

    def test_modulo_cableado(self) -> None:
        self.assertIn('module "secretos"', self.main)
        self.assertIn('source = "../../modules/secretos"', self.main)
        # El alineado lo fija `terraform fmt`: se compara sin espacios multiples.
        compacto = re.sub(r"[ \t]+", " ", self.main)
        self.assertIn("rol_runtime_arn = module.iam.runtime_role_arn", compacto)

    def test_iam_recibe_el_prefijo(self) -> None:
        self.assertIn("secretos_object_arns      = [module.secretos.secrets_prefix_arn]", self.main)

    def test_publicador_cableado_y_fallando_cerrado(self) -> None:
        self.assertIn("publicador_arn = var.secretos_publicador_arn", re.sub(r"[ ]+", " ", self.main))
        self.assertIn('variable "secretos_publicador_arn"', self.variables)

    def test_computo_recibe_la_uri_con_version(self) -> None:
        self.assertIn("secretos_s3_uri", self.main)
        self.assertIn("var.secretos_version", self.main)
        self.assertIn('variable "secretos_version"', self.variables)

    def test_version_vacia_no_suministra_bundle(self) -> None:
        self.assertIn('var.secretos_version == "" ? "" :', self.main)


class BootstrapDelBundle(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.template = leer(COMPUTO / "user-data.sh.tftpl")
        cls.variables = leer(COMPUTO / "variables.tf")
        cls.parser = bloque_parser()

    def test_uri_y_herramientas(self) -> None:
        self.assertIn('SECRETOS_URI="${secretos_uri}"', self.template)
        self.assertIn("for cmd in aws sha256sum install systemctl curl python3; do", self.template)

    def test_permisos_restrictivos_y_escritura_atomica(self) -> None:
        self.assertIn("os.chmod(temporal, 0o640)", self.parser)
        self.assertIn("os.replace(temporal, destino)", self.parser)
        self.assertIn('chmod 0640 "$ENV_FILE"', self.template)
        self.assertIn('chown root:"$USUARIO" "$ENV_FILE"', self.template)

    def test_falla_cerrado_sin_bundle(self) -> None:
        self.assertIn('fail "falta $ENV_FILE (bundle de secretos no suministrado; ver RFC-004)"', self.template)
        self.assertIn("[[ -s \"$TMP/secrets.json\" ]] || fail", self.template)

    def test_variable_de_computo_validada(self) -> None:
        self.assertIn('variable "secretos_s3_uri"', self.variables)
        self.assertIn("^s3://[a-z0-9.-]+/.+$", self.variables)

    def test_sin_secretos_en_user_data(self) -> None:
        for prohibido in ("pass" + "word=", "PRIVATE KEY", "AWS_SECRET_" + "ACCESS_KEY="):
            self.assertNotIn(prohibido, self.template, prohibido)


class ParserDelBundle(unittest.TestCase):
    """Ejecuta el parser real del bootstrap sobre fixtures (sin AWS)."""

    @classmethod
    def setUpClass(cls) -> None:
        cls.parser = bloque_parser()
        cls.directorio = Path(tempfile.mkdtemp())
        cls.script = cls.directorio / "parser.py"
        cls.script.write_text(cls.parser, encoding="utf-8")

    def _ejecutar(self, contenido: str) -> tuple[int, str, str]:
        origen = self.directorio / "secrets.json"
        origen.write_text(contenido, encoding="utf-8")
        destino = self.directorio / "backend.env"
        if destino.exists():
            destino.unlink()
        proceso = subprocess.run(
            [sys.executable, str(self.script), str(origen), str(destino)],
            capture_output=True,
            text=True,
        )
        return proceso.returncode, proceso.stdout, proceso.stderr

    def test_bundle_valido_instala_variables(self) -> None:
        codigo, salida, error = self._ejecutar(
            json.dumps({"version": "v1", "variables": {"APP_ENV": "PRODUCTION", "REDIS_ADDR": "cache:6379"}})
        )
        self.assertEqual(codigo, 0, error)
        self.assertIn("version v1", salida)
        destino = self.directorio / "backend.env"
        self.assertIn("APP_ENV=PRODUCTION", destino.read_text(encoding="utf-8"))
        self.assertIn("REDIS_ADDR=cache:6379", destino.read_text(encoding="utf-8"))

    def test_sin_version_falla(self) -> None:
        codigo, _, error = self._ejecutar(json.dumps({"variables": {"APP_ENV": "PRODUCTION"}}))
        self.assertNotEqual(codigo, 0)
        self.assertIn("version", error)

    def test_sin_variables_falla(self) -> None:
        codigo, _, error = self._ejecutar(json.dumps({"version": "v1"}))
        self.assertNotEqual(codigo, 0)

    def test_sin_app_env_falla(self) -> None:
        codigo, _, error = self._ejecutar(json.dumps({"version": "v1", "variables": {"OTRA": "x"}}))
        self.assertNotEqual(codigo, 0)
        self.assertIn("APP_ENV", error)

    def test_clave_invalida_falla(self) -> None:
        codigo, _, _ = self._ejecutar(json.dumps({"version": "v1", "variables": {"app_env": "x"}}))
        self.assertNotEqual(codigo, 0)

    def test_valor_con_salto_de_linea_falla(self) -> None:
        codigo, _, _ = self._ejecutar(
            json.dumps({"version": "v1", "variables": {"APP_ENV": "PRODUCTION\nINJECT=1"}})
        )
        self.assertNotEqual(codigo, 0)

    def test_raiz_no_objeto_falla(self) -> None:
        codigo, _, _ = self._ejecutar(json.dumps(["APP_ENV=PRODUCTION"]))
        self.assertNotEqual(codigo, 0)

    def test_json_invalido_falla(self) -> None:
        codigo, _, _ = self._ejecutar("{no es json")
        self.assertNotEqual(codigo, 0)


class ExcepcionTrivyAcotada(unittest.TestCase):
    def test_excepcion_citada_y_scoped(self) -> None:
        texto = leer(RAIZ / ".trivyignore.yaml")
        self.assertIn('"modules/secretos/main.tf"', texto)
        self.assertIn("APRUEBO_RFC004_OPCION_B", texto)
        self.assertNotIn("id: *", texto)
        self.assertNotIn('"*"', texto)
        # AWS-0132 queda acotada a dos rutas concretas: state (I-070) y secretos (RFC-004).
        self.assertEqual(texto.count("- id: AWS-0132"), 2)


class Documentacion(unittest.TestCase):
    def test_rfc004_aprobada_con_la_frase(self) -> None:
        texto = leer(RAIZ / "sdd" / "_control" / "H08" / "RFC-004-SECRETOS_BOOTSTRAP.md")
        self.assertIn("APRUEBO_RFC004_OPCION_B", texto)
        self.assertIn("APROBADA (2026-11-09)", texto)

    def test_gate_registra_la_autorizacion(self) -> None:
        gate = RAIZ / "sdd" / "_control" / "H08" / "GATE_H08I_RFC004_OPCION_B.md"
        self.assertTrue(gate.is_file())
        texto = leer(gate)
        self.assertIn("APRUEBO_RFC004_OPCION_B", texto)
        self.assertIn("No autoriza", texto)
