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


def _bloques(texto: str, etiqueta: str) -> list[str]:
    """Devuelve el contenido de cada bloque `etiqueta { ... }` con anidamiento."""
    resultado: list[str] = []
    patron = re.compile(rf"\b{etiqueta}\s*\{{")
    for coincidencia in patron.finditer(texto):
        profundidad = 1
        i = coincidencia.end()
        while i < len(texto) and profundidad:
            if texto[i] == "{":
                profundidad += 1
            elif texto[i] == "}":
                profundidad -= 1
            i += 1
        resultado.append(texto[coincidencia.end() : i - 1])
    return resultado


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

    def _statements(self) -> list[dict]:
        """Extrae las sentencias de la politica del bucket con sus campos."""
        base = (
            self.main.replace("${aws_s3_bucket.secretos.arn}", "arn:aws:s3:::bucket")
            .replace("${var.prefijo}", "nexum/secrets/")
            .replace("var.rol_runtime_arn", f'"{self.RUNTIME}"')
            .replace("var.publicador_arn", f'"{self.PUBLICADOR}"')
            .replace("var.admin_arn", f'"{self.ADMIN}"')
            .replace("var.gestor_iac_arn", f'"{self.IAC}"')
        )
        salida: list[dict] = []
        for trozo in _bloques(base, "statement"):
            efecto = re.search(r'effect += +"(Allow|Deny)"', trozo)
            acciones = re.findall(r'"(s3:[A-Za-z*]+)"', trozo)
            recursos = re.findall(r'"(arn:aws:s3:::[^"]+)"', trozo)
            ids = re.search(r"identifiers = \[(.*?)\]", trozo, re.DOTALL)
            principal = re.findall(r'"([^"]+)"', ids.group(1)) if ids else []
            condiciones = []
            for cond in _bloques(trozo, "condition"):
                prueba = re.search(r'test += +"(\w+)"', cond)
                variable = re.search(r'variable += +"([^"]+)"', cond)
                extraido = re.search(r"values\s*=\s*(.+)", cond)
                valores = None
                if extraido:
                    crudo = extraido.group(1)
                    ini = crudo.find("[")
                    fin = crudo.rfind("]")
                    if ini >= 0 and fin > ini:
                        valores = re.findall(r'"([^"]*)"', crudo[ini + 1 : fin])
                if prueba and variable and valores:
                    condiciones.append(
                        (
                            prueba.group(1),
                            variable.group(1),
                            list(valores),
                        )
                    )
            if efecto:
                salida.append(
                    {
                        "sid": (re.search(r'sid += +"(\w+)"', trozo) or [None, ""])[1],
                        "effect": efecto.group(1),
                        "actions": acciones,
                        "resources": recursos,
                        "principals": principal,
                        "conditions": condiciones,
                        "texto": trozo,
                    }
                )
        return salida

    def _decidir(
        self,
        principal: str,
        accion: str,
        recurso: str,
        *,
        tls: bool = True,
        sse: str | None = "AES256",
        identidad_permite: bool = True,
    ) -> str:
        sentencias = self._statements()

        def coincide_accion(patrones: list[str]) -> bool:
            return any(
                p == "*" or p == accion or (p.endswith("*") and accion.startswith(p[:-1]))
                for p in patrones
            )

        def coincide_recurso(patrones: list[str]) -> bool:
            for p in patrones:
                if p.endswith("*"):
                    if recurso.startswith(p[:-1]):
                        return True
                elif p == recurso:
                    return True
            return False

        def coincide_principal(ids: list[str]) -> bool:
            if "*" in ids:
                return True
            return principal in ids

        def condiciones_aplican(condiciones: list[tuple[str, str, list[str]]]) -> bool:
            for prueba, variable, valores in condiciones:
                if variable == "aws:SecureTransport" and prueba == "Bool":
                    if tls is True and "false" in valores:
                        return False
                elif variable == "aws:PrincipalArn" and prueba in ("StringNotEquals", "ArnNotEquals"):
                    if principal in valores:
                        return False
                elif variable == "s3:x-amz-server-side-encryption":
                    if prueba == "StringEquals" and sse not in valores:
                        return False
                    if prueba == "StringNotEquals" and sse in valores:
                        return False
                else:
                    raise AssertionError(f"condicion no reconocida: {prueba} {variable}")
            return True

        for s in sentencias:
            if s["effect"] != "Deny":
                continue
            if not coincide_accion(s["actions"]):
                continue
            if not coincide_recurso(s["resources"]):
                continue
            if not coincide_principal(s["principals"]):
                continue
            if condiciones_aplican(s["conditions"]):
                return "DENY"

        for s in sentencias:
            if s["effect"] != "Allow":
                continue
            if (
                coincide_accion(s["actions"])
                and coincide_recurso(s["resources"])
                and coincide_principal(s["principals"])
                and condiciones_aplican(s["conditions"])
            ):
                return "ALLOW"

        # Misma cuenta: sin Deny aplicable, decide la policy de identidad.
        return "ALLOW" if identidad_permite else "DENY"

    RUNTIME = "arn:aws:iam::111122223333:role/nexum-runtime"
    PUBLICADOR = "arn:aws:iam::111122223333:role/nexum-publicador"
    AJENO = "arn:aws:iam::111122223333:role/otra-app-amplia"
    ADMIN = "arn:aws:iam::111122223333:role/nexum-recuperacion"
    IAC = "arn:aws:iam::111122223333:role/nexum-iac-gestor"
    OBJETO = "arn:aws:s3:::bucket/nexum/secrets/v1/secrets.json"
    FUERA = "arn:aws:s3:::bucket/otros/secrets.json"

    def test_runtime_lee_el_bundle(self) -> None:
        self.assertEqual(self._decidir(self.RUNTIME, "s3:GetObject", self.OBJETO), "ALLOW")

    def test_runtime_no_escribe(self) -> None:
        self.assertEqual(self._decidir(self.RUNTIME, "s3:PutObject", self.OBJETO), "DENY")

    def test_runtime_no_borra(self) -> None:
        self.assertEqual(self._decidir(self.RUNTIME, "s3:DeleteObject", self.OBJETO), "DENY")

    def test_publicador_publica_con_sse(self) -> None:
        self.assertEqual(self._decidir(self.PUBLICADOR, "s3:PutObject", self.OBJETO), "ALLOW")

    def test_publicador_no_lee(self) -> None:
        self.assertEqual(self._decidir(self.PUBLICADOR, "s3:GetObject", self.OBJETO), "DENY")

    def test_publicador_fuera_del_prefijo(self) -> None:
        # Fuera del prefijo no hay Allow del bucket y su identidad esta acotada.
        self.assertEqual(
            self._decidir(self.PUBLICADOR, "s3:PutObject", self.FUERA, identidad_permite=False),
            "DENY",
        )

    def test_principal_ajeno_lee_denegado(self) -> None:
        self.assertEqual(self._decidir(self.AJENO, "s3:GetObject", self.OBJETO), "DENY")

    def test_principal_ajeno_escribe_denegado(self) -> None:
        self.assertEqual(self._decidir(self.AJENO, "s3:PutObject", self.OBJETO), "DENY")

    def test_principal_ajeno_borra_denegado(self) -> None:
        self.assertEqual(self._decidir(self.AJENO, "s3:DeleteObject", self.OBJETO), "DENY")

    def test_sin_tls_denegado_para_todos(self) -> None:
        for principal in (self.RUNTIME, self.PUBLICADOR, self.AJENO):
            self.assertEqual(
                self._decidir(principal, "s3:GetObject", self.OBJETO, tls=False), "DENY", principal
            )

    def test_publicador_sin_sse_denegado(self) -> None:
        self.assertEqual(
            self._decidir(self.PUBLICADOR, "s3:PutObject", self.OBJETO, sse=None), "DENY"
        )

    def test_publicador_no_borra_ni_con_iam_amplio(self) -> None:
        for accion in ("s3:DeleteObject", "s3:DeleteObjectVersion"):
            self.assertEqual(
                self._decidir(self.PUBLICADOR, accion, self.OBJETO, identidad_permite=True),
                "DENY",
                accion,
            )
            self.assertEqual(
                self._decidir(self.PUBLICADOR, accion, self.FUERA, identidad_permite=True),
                "DENY",
                accion,
            )

    def test_nadie_lista_el_bucket(self) -> None:
        bucket = "arn:aws:s3:::bucket"
        for principal in (self.AJENO, self.RUNTIME, self.PUBLICADOR):
            self.assertEqual(
                self._decidir(principal, "s3:ListBucket", bucket, identidad_permite=True),
                "DENY",
                principal,
            )

    def test_runtime_no_escribe_ni_borra_fuera_del_prefijo(self) -> None:
        for accion in ("s3:PutObject", "s3:DeleteObject"):
            self.assertEqual(
                self._decidir(self.RUNTIME, accion, self.FUERA, identidad_permite=True),
                "DENY",
                accion,
            )

    def test_lectura_y_publicacion_legitimas_siguen_permitidas(self) -> None:
        self.assertEqual(self._decidir(self.RUNTIME, "s3:GetObject", self.OBJETO), "ALLOW")
        self.assertEqual(self._decidir(self.PUBLICADOR, "s3:PutObject", self.OBJETO), "ALLOW")

    def test_condiciones_usan_principalarn_y_no_notprincipal(self) -> None:
        self.assertIn('variable = "aws:PrincipalArn"', self.main)
        self.assertNotIn("not_principals", self.main)
        self.assertNotIn("NotPrincipal", self.main)

    def test_gestor_iac_lista_bucket_sin_deny(self) -> None:
        # La excepcion es ausencia de Deny, no un Allow del bucket.
        bucket = "arn:aws:s3:::bucket"
        self.assertEqual(
            self._decidir(self.IAC, "s3:ListBucket", bucket, identidad_permite=True), "ALLOW"
        )

    def test_gestor_iac_sin_iam_no_obtiene_allow(self) -> None:
        bucket = "arn:aws:s3:::bucket"
        self.assertEqual(
            self._decidir(self.IAC, "s3:ListBucket", bucket, identidad_permite=False), "DENY"
        )

    def test_gestor_iac_no_lista_versiones_ni_multipart(self) -> None:
        bucket = "arn:aws:s3:::bucket"
        for accion in ("s3:ListBucketVersions", "s3:ListBucketMultipartUploads"):
            self.assertEqual(
                self._decidir(self.IAC, accion, bucket, identidad_permite=True), "DENY", accion
            )

    def test_gestor_iac_no_accede_a_los_objetos(self) -> None:
        for accion in ("s3:GetObject", "s3:PutObject", "s3:DeleteObject"):
            self.assertEqual(
                self._decidir(self.IAC, accion, self.OBJETO, identidad_permite=True), "DENY", accion
            )

    def test_list_bucket_denegado_al_resto(self) -> None:
        bucket = "arn:aws:s3:::bucket"
        for principal in (self.RUNTIME, self.PUBLICADOR, self.AJENO, self.ADMIN):
            self.assertEqual(
                self._decidir(principal, "s3:ListBucket", bucket, identidad_permite=True),
                "DENY",
                principal,
            )

    def test_gestor_iac_aislado_y_validado(self) -> None:
        self.assertIn('variable "gestor_iac_arn"', self.variables)
        self.assertIn("var.gestor_iac_arn != var.rol_runtime_arn", self.main)
        self.assertIn("var.gestor_iac_arn != var.publicador_arn", self.main)
        self.assertIn("^arn:aws:iam::[0-9]{12}:role/.+$", self.main)
        self.assertIn("ArnNotEquals", self.main)

    def test_administracion_no_implica_lectura_de_secretos(self) -> None:
        # La recuperacion usa s3:PutBucketPolicy (plano de control), que esta
        # politica de objetos ni concede ni exime: no hay admin_arn exento.
        self.assertNotIn("admin_arn", self.main)
        self.assertNotIn("admin_arn", self.variables)
        self.assertEqual(self._decidir(self.ADMIN, "s3:GetObject", self.OBJETO), "DENY")
        self.assertEqual(self._decidir(self.ADMIN, "s3:DeleteObject", self.OBJETO), "DENY")

    def test_principal_ajeno_denegado_tambien_fuera_del_prefijo(self) -> None:
        self.assertEqual(self._decidir(self.AJENO, "s3:GetObject", self.FUERA), "DENY")

    def test_la_politica_no_concede_listbucket(self) -> None:
        # ListBucket es de plano de control/identidad: este modulo no lo concede
        # ni lo deniega, y ninguna sentencia debe nombrarlo.
        self.assertIn("DenegarListBucket", self.main)

    def test_condicion_no_reconocida_no_se_asume_verdadera(self) -> None:
        sentencias = self._statements()
        variables = {
            cond[1] for s in sentencias for cond in s["conditions"]
        }
        reconocidas = {
            "aws:SecureTransport",
            "aws:PrincipalArn",
            "s3:x-amz-server-side-encryption",
        }
        self.assertEqual(variables - reconocidas, set(), variables)

    def test_runtime_y_publicador_no_pueden_coincidir(self) -> None:
        self.assertIn("var.rol_runtime_arn != var.publicador_arn", self.main)

    def test_publicador_ausente_falla_cerrado(self) -> None:
        self.assertIn("publicador_arn debe ser un ARN", self.main)
        self.assertIn('default     = ""', self.variables)

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
        compacto = re.sub(r"[ ]+", " ", self.main)
        # RFC-011: el publicador y el gestor IaC los crea el módulo
        # `identidades_publicacion`; la raíz los referencia por ARN determinista y
        # ya no existe una variable vacía que pueda quedar sin cablear.
        self.assertIn("publicador_arn = local.arn_secretos_publicador", compacto)
        # La exención de ListBucket es el operador: es quien ejecuta Terraform.
        self.assertIn("gestor_iac_arn = var.principal_operador_arn", compacto)
        self.assertNotIn("gestor_iac_arn = local.arn_secretos_gestor_iac", compacto)
        self.assertNotIn('variable "secretos_publicador_arn"', self.variables)
        self.assertNotIn('variable "secretos_gestor_iac_arn"', self.variables)

        # El principal operador es obligatorio: sin él el plan falla cerrado.
        self.assertIn('variable "principal_operador_arn"', self.variables)
        self.assertIn('default     = ""', self.variables)

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
        self.assertEqual(texto.count("- id: AWS-0132"), 3)


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
