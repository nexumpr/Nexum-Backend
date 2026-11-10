"""Pruebas estáticas de RFC-011 (identidades de publicación).

Motivo: `terraform validate` **no** evalúa preconditions ni detecta que una
*trust policy* incluya `Resource`, algo que AWS rechaza con
`MalformedPolicyDocument` en el `apply`. Estas pruebas cubren ese hueco y, sobre
todo, impiden la regresión: además de comprobar el módulo real, ejecutan el mismo
detector contra fixtures sintéticas **defectuosas** para demostrar que las
detecciones tienen dientes (una prueba que sólo busca cadenas en el archivo real
pasaría aunque el detector estuviera roto).

No contacta AWS ni ejecuta Terraform. Complementa —no sustituye— `terraform
validate` y el plan real, que siguen `NOT_RUN` en esta unidad.
"""

from __future__ import annotations

import re
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
MODULO = RAIZ / "infra" / "terraform" / "modules" / "identidades_publicacion"
ENV = RAIZ / "infra" / "terraform" / "envs" / "h08i"
MODULOS = RAIZ / "infra" / "terraform" / "modules"


# --- Utilidades de análisis estructural -----------------------------------


def _leer(p: Path) -> str:
    return p.read_text(encoding="utf-8")


def _sin_comentarios(texto: str) -> str:
    """Elimina comentarios `#` fuera de cadenas (el módulo sólo usa `#`)."""
    salida, i, en_cadena = [], 0, False
    while i < len(texto):
        c = texto[i]
        if c == '"' and (i == 0 or texto[i - 1] != "\\"):
            en_cadena = not en_cadena
        if not en_cadena and c == "#":
            while i < len(texto) and texto[i] != "\n":
                i += 1
            continue
        salida.append(c)
        i += 1
    return "".join(salida)


def _interior(texto: str, apertura: int) -> str:
    """Interior del bloque cuya `{` está en `apertura`."""
    nivel = 0
    for pos in range(apertura, len(texto)):
        if texto[pos] == "{":
            nivel += 1
        elif texto[pos] == "}":
            nivel -= 1
            if nivel == 0:
                return texto[apertura + 1 : pos]
    raise AssertionError("bloque sin cierre")


def bloque(texto: str, cabecera: str) -> str:
    """Interior del primer bloque `cabecera ... { ... }`."""
    limpio = _sin_comentarios(texto)
    inicio = limpio.find(cabecera)
    if inicio < 0:
        raise AssertionError(f"no se encontró el bloque {cabecera!r}")
    return _interior(limpio, limpio.index("{", inicio + len(cabecera) - 1))


def bloques(texto: str, cabecera: str) -> list[str]:
    """Interiores de **todos** los bloques `cabecera ... { ... }`."""
    limpio = _sin_comentarios(texto)
    resultado, desde = [], 0
    while True:
        inicio = limpio.find(cabecera, desde)
        if inicio < 0:
            return resultado
        apertura = limpio.index("{", inicio + len(cabecera) - 1)
        resultado.append(_interior(limpio, apertura))
        desde = inicio + len(cabecera)


def atributos(cuerpo: str) -> dict[str, str]:
    """Atributos de primer nivel de un bloque, como `clave -> valor`."""
    resultado, nivel = {}, 0
    for linea in _sin_comentarios(cuerpo).splitlines():
        limpia = linea.strip()
        if nivel == 0 and re.match(r"^[a-zA-Z_][a-zA-Z0-9_]*\s*=", limpia):
            clave, valor = limpia.split("=", 1)
            resultado[clave.strip()] = valor.strip()
            continue
        nivel += limpia.count("{") - limpia.count("}")
    return resultado


# --- Detector de trust policies (se prueba contra el módulo y contra fixtures) ---

OPERADOR = "var.principal_operador_arn"
ATRIBUTOS_PROHIBIDOS = ("resources", "not_resources", "source_policy_documents", "override_policy_documents")


def problemas_trust(cuerpo: str) -> list[str]:
    """Problemas de una trust policy en HCL; lista vacía = documento aceptable.

    Reglas (AWS + mínimo privilegio):
      * una trust policy no admite `Resource`/`NotResource` (ni documentos
        incrustados que pudieran reintroducirlos);
      * un único enunciado `Allow`;
      * exactamente `Principal { type = "AWS", identifiers = [operador] }`;
      * condición `ArnEquals`/`StringEquals` sobre `aws:PrincipalArn` = operador;
      * ningún comodín ni `:root`.
    """
    problemas: list[str] = []
    try:
        doc = atributos(cuerpo)
    except AssertionError as ex:  # bloque malformado: fallar cerrado
        return [f"trust ilegible: {ex}"]

    for prohibido in ATRIBUTOS_PROHIBIDOS:
        if prohibido in doc:
            problemas.append(f"trust declara {prohibido}, prohibido en una política de confianza")

    enunciados = bloques(cuerpo, "statement")
    if len(enunciados) != 1:
        problemas.append(f"trust debe tener un único enunciado (tiene {len(enunciados)})")

    for enunciado in enunciados:
        campos = atributos(enunciado)
        for prohibido in ATRIBUTOS_PROHIBIDOS:
            if prohibido in campos:
                problemas.append(f"enunciado declara {prohibido}")
        if campos.get("effect") != '"Allow"':
            problemas.append(f"enunciado con effect {campos.get('effect')!r}; sólo se admite Allow")
        if campos.get("actions") != '["sts:AssumeRole"]':
            problemas.append(f"enunciado con actions {campos.get('actions')!r}; sólo sts:AssumeRole")

        principales = bloques(enunciado, "principals")
        if len(principales) != 1:
            problemas.append(f"se esperaba un bloque principals y hay {len(principales)}")
        for principal in principales:
            valores = atributos(principal)
            if valores.get("type") != '"AWS"':
                problemas.append(f"principal con type {valores.get('type')!r}; sólo AWS")
            if valores.get("identifiers") != f"[{OPERADOR}]":
                problemas.append(f"principal con identifiers {valores.get('identifiers')!r}; debe ser el operador exacto")

        condiciones = bloques(enunciado, "condition")
        if len(condiciones) != 1:
            problemas.append(f"se esperaba una condición y hay {len(condiciones)}")
        for condicion in condiciones:
            valores = atributos(condicion)
            if valores.get("test") not in ('"ArnEquals"', '"StringEquals"'):
                problemas.append(f"condición con test {valores.get('test')!r}")
            if valores.get("variable") != '"aws:PrincipalArn"':
                problemas.append(f"condición sobre {valores.get('variable')!r}; debe ser aws:PrincipalArn")
            if valores.get("values") != f"[{OPERADOR}]":
                problemas.append(f"condición con values {valores.get('values')!r}; debe ser el operador exacto")

    texto = _sin_comentarios(cuerpo)
    for patron, motivo in (
        (r'identifiers\s*=\s*\[\s*"\*"\s*\]', "principal comodín"),
        (r'values\s*=\s*\[\s*"\*"\s*\]', "condición comodín"),
        (r":root", "principal de cuenta raíz"),
    ):
        if re.search(patron, texto):
            problemas.append(motivo)
    return problemas


class Estructura(unittest.TestCase):
    def test_hay_ficheros_esperados(self) -> None:
        for nombre in ("main.tf", "variables.tf", "outputs.tf"):
            self.assertTrue((MODULO / nombre).is_file(), nombre)

    def test_tres_roles_una_politica_cada_uno(self) -> None:
        main = _leer(MODULO / "main.tf")
        roles = re.findall(r'resource\s+"aws_iam_role"\s+"([a-z_]+)"', main)
        self.assertEqual(sorted(roles), ["artefactos_publicador", "secretos_gestor_iac", "secretos_publicador"])
        for rol in roles:
            self.assertIn(f'resource "aws_iam_role_policy" "{rol}"', main)

    def test_sin_usuarios_claves_ni_oidc(self) -> None:
        main = _leer(MODULO / "main.tf")
        for prohibido in (
            "aws_iam_user",
            "aws_iam_access_key",
            "aws_iam_openid_connect_provider",
            "aws_iam_role_policy_attachment",
        ):
            self.assertNotIn(f'resource "{prohibido}"', main, prohibido)

    def test_provider_versionado(self) -> None:
        main = _leer(MODULO / "main.tf")
        self.assertIn('required_version = ">= 1.10.0', main)
        self.assertRegex(main, r'source\s*=\s*"hashicorp/aws"')
        self.assertRegex(main, r'version\s*=\s*"~> 5\.72"')

    def test_nombres_deterministas_y_en_minusculas(self) -> None:
        main = _leer(MODULO / "main.tf")
        for nombre in ("artefactos-publicador", "secretos-publicador", "secretos-gestor-iac"):
            self.assertIn(nombre, main)
        self.assertIn('lower("${var.proyecto}-${var.entorno}', main)

    def test_arn_derivados_de_cuenta_y_nombre(self) -> None:
        main = _sin_comentarios(_leer(MODULO / "main.tf"))
        self.assertIn('"arn:aws:iam::${var.cuenta_id}:role/${local.nombre_', main)
        # Ningún ARN STS concreto (cuenta numérica): la confianza se ancla al rol IAM.
        self.assertNotRegex(main, r"arn:aws:sts::[0-9]{12}")

    def test_sin_numeros_de_cuenta_ni_credenciales_literales(self) -> None:
        for ruta in MODULO.glob("*.tf"):
            texto = _leer(ruta)
            self.assertNotRegex(texto, r"\b\d{12}\b", ruta.name)
            self.assertNotRegex(texto, r"AKIA[0-9A-Z]{16}", ruta.name)


class TrustPolicy(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(MODULO / "main.tf")
        cls.trust = bloque(cls.main, 'data "aws_iam_policy_document" "trust_operador"')

    def test_los_tres_roles_usan_el_mismo_documento_de_confianza(self) -> None:
        self.assertEqual(
            self.main.count("assume_role_policy   = data.aws_iam_policy_document.trust_operador.json"),
            3,
        )

    def test_no_contiene_resource_ni_notresource(self) -> None:
        self.assertNotIn("resources", atributos(self.trust))
        self.assertNotIn("not_resources", atributos(self.trust))
        self.assertEqual(problemas_trust(self.trust), [])

    def test_condicion_arn_equals_sobre_principal_arn(self) -> None:
        condicion = bloque(self.trust, "condition")
        valores = atributos(condicion)
        self.assertEqual(valores["test"], '"ArnEquals"')
        self.assertEqual(valores["variable"], '"aws:PrincipalArn"')
        self.assertEqual(valores["values"], f"[{OPERADOR}]")

    def test_principal_exacto_sin_comodin(self) -> None:
        principal = bloque(self.trust, "principals")
        valores = atributos(principal)
        self.assertEqual(valores["type"], '"AWS"')
        self.assertEqual(valores["identifiers"], f"[{OPERADOR}]")
        self.assertNotIn('"*"', principal)

    def test_un_unico_enunciado_allow_de_assume_role(self) -> None:
        enunciados = bloques(self.trust, "statement")
        self.assertEqual(len(enunciados), 1)
        self.assertEqual(atributos(enunciados[0])["effect"], '"Allow"')
        self.assertEqual(atributos(enunciados[0])["actions"], '["sts:AssumeRole"]')

    def test_sin_jsonencode_manual_de_la_confianza(self) -> None:
        # Un `jsonencode` a mano es donde reaparecería un `Resource`: la confianza
        # de los tres roles debe venir del mismo data source verificable.
        self.assertNotIn("assume_role_policy = jsonencode", self.main)


class DetectorConDientes(unittest.TestCase):
    """El detector debe **fallar** ante las variantes defectuosas conocidas."""

    BASE = """
data "aws_iam_policy_document" "trust_operador" {
  statement {
    sid     = "SoloPrincipalOperadorAutorizado"
    effect  = "Allow"
    actions = ["sts:AssumeRole"]

    principals {
      type        = "AWS"
      identifiers = [var.principal_operador_arn]
    }

    condition {
      test     = "ArnEquals"
      variable = "aws:PrincipalArn"
      values   = [var.principal_operador_arn]
    }
  }
}
"""

    def _con(self, extra: str) -> str:
        cuerpo = bloque(self.BASE, 'data "aws_iam_policy_document" "trust_operador"')
        return cuerpo + "\n" + extra

    def test_base_sintetica_es_aceptable(self) -> None:
        cuerpo = bloque(self.BASE, 'data "aws_iam_policy_document" "trust_operador"')
        self.assertEqual(problemas_trust(cuerpo), [])

    def test_rechaza_resource_en_el_enunciado(self) -> None:
        cuerpo = bloque(self.BASE, 'data "aws_iam_policy_document" "trust_operador"')
        roto = cuerpo.replace(
            'actions = ["sts:AssumeRole"]',
            'actions = ["sts:AssumeRole"]\n    resources = ["*"]',
        )
        self.assertIn("enunciado declara resources", " ".join(problemas_trust(roto)))

    def test_rechaza_resource_como_enunciado_separado(self) -> None:
        # Forma exacta del defecto reportado: un Deny con resources = ["*"].
        roto = self._con(
            """
  statement {
    sid       = "DenegarPrincipalNoAutorizado"
    effect    = "Deny"
    actions   = ["sts:AssumeRole"]
    resources = ["*"]
  }
"""
        )
        problemas = " ".join(problemas_trust(roto))
        self.assertIn("resources", problemas)
        self.assertIn("único enunciado", problemas)

    def test_rechaza_notresource(self) -> None:
        cuerpo = bloque(self.BASE, 'data "aws_iam_policy_document" "trust_operador"')
        roto = cuerpo.replace("effect  = \"Allow\"", "effect  = \"Allow\"\n    not_resources = []")
        self.assertIn("not_resources", " ".join(problemas_trust(roto)))

    def test_rechaza_principal_comodin(self) -> None:
        cuerpo = bloque(self.BASE, 'data "aws_iam_policy_document" "trust_operador"')
        roto = cuerpo.replace(f"[{OPERADOR}]", '["*"]', 1)
        self.assertIn("principal comodín", problemas_trust(roto))

    def test_rechaza_segundo_principal(self) -> None:
        cuerpo = bloque(self.BASE, 'data "aws_iam_policy_document" "trust_operador"')
        roto = cuerpo.replace(
            f"identifiers = [{OPERADOR}]",
            f'identifiers = [{OPERADOR}, "arn:aws:iam::713797602090:role/Otro"]',
        )
        self.assertIn("operador exacto", " ".join(problemas_trust(roto)))

    def test_rechaza_condicion_ausente(self) -> None:
        cuerpo = bloque(self.BASE, 'data "aws_iam_policy_document" "trust_operador"')
        roto = cuerpo[: cuerpo.index("condition {")] + "  }\n}\n"
        self.assertTrue(problemas_trust(roto))

    def test_rechaza_effect_deny(self) -> None:
        cuerpo = bloque(self.BASE, 'data "aws_iam_policy_document" "trust_operador"')
        roto = cuerpo.replace('effect  = "Allow"', 'effect  = "Deny"')
        self.assertIn("effect", " ".join(problemas_trust(roto)))

    def test_rechaza_cuenta_raiz(self) -> None:
        cuerpo = bloque(self.BASE, 'data "aws_iam_policy_document" "trust_operador"')
        roto = cuerpo.replace(OPERADOR, '"arn:aws:iam::713797602090:root"')
        self.assertIn("cuenta raíz", " ".join(problemas_trust(roto)))


class GuardasDePlan(unittest.TestCase):
    """Preconditions: `terraform validate` no las evalúa, pero el plan sí."""

    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(MODULO / "main.tf")
        cls.guardas = bloque(cls.main, 'resource "terraform_data" "identidades_preconditions"')

    def test_precondition_sobre_el_json_renderizado(self) -> None:
        for local in (
            "local.trust_sin_campos_prohibidos",
            "local.trust_solo_allow",
            "local.trust_principal_exacto",
            "local.trust_condicion_exacta",
        ):
            self.assertIn(local, self.guardas, local)
        self.assertIn("jsondecode(data.aws_iam_policy_document.trust_operador.json)", self.main)

    def test_precondition_rechaza_sts(self) -> None:
        self.assertIn('!startswith(var.principal_operador_arn, "arn:aws:sts::")', self.guardas)

    def test_precondition_exige_iam(self) -> None:
        self.assertIn('startswith(var.principal_operador_arn, "arn:aws:iam::")', self.guardas)

    def test_precondition_trust_circular(self) -> None:
        self.assertIn("confianza circular", self.guardas)
        self.assertIn("local.arn_artefactos_publicador", self.guardas)

    def test_precondition_cuenta_de_12_digitos(self) -> None:
        self.assertIn('can(regex("^[0-9]{12}$", var.cuenta_id))', self.guardas)

    def test_precondition_identidades_distintas(self) -> None:
        self.assertIn("length(distinct(", self.guardas)
        self.assertIn("var.rol_runtime_arn", self.guardas)


class VariableOperador(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.variables = _leer(MODULO / "variables.tf")
        cls.env_variables = _leer(ENV / "variables.tf")

    def test_sin_valor_por_defecto(self) -> None:
        cuerpo = bloque(self.variables, 'variable "principal_operador_arn"')
        self.assertNotIn("default", atributos(cuerpo))

    def test_validation_sts(self) -> None:
        self.assertIn('!startswith(var.principal_operador_arn, "arn:aws:sts::")', self.variables)

    def test_regex_acepta_rol_y_usuario_del_operador(self) -> None:
        regex = re.search(r'can\(regex\("(\^arn:aws:iam::\S+)", var\.principal_operador_arn\)\)', self.variables)
        self.assertIsNotNone(regex, "no se encontró la validation de formato")
        patron = re.compile(regex.group(1))
        self.assertTrue(patron.fullmatch("arn:aws:iam::713797602090:role/AccountFullAccessRole"))
        self.assertTrue(patron.fullmatch("arn:aws:iam::713797602090:user/operador"))

    def test_regex_rechaza_sts_root_comodin_y_cuenta(self) -> None:
        regex = re.search(r'can\(regex\("(\^arn:aws:iam::\S+)", var\.principal_operador_arn\)\)', self.variables)
        self.assertIsNotNone(regex)
        patron = re.compile(regex.group(1))
        for invalido in (
            "arn:aws:sts::713797602090:assumed-role/AccountFullAccessRole/sesion",
            "arn:aws:iam::713797602090:root",
            "arn:aws:iam::713797602090:role/*",
            "713797602090",
            "",
        ):
            with self.subTest(arn=invalido):
                self.assertIsNone(patron.fullmatch(invalido))

    def test_misma_variable_en_el_entorno(self) -> None:
        cuerpo = bloque(self.env_variables, 'variable "principal_operador_arn"')
        self.assertNotIn("default", atributos(cuerpo))
        self.assertIn("arn:aws:iam::[0-9]{12}:(role|user)/", cuerpo)


class CableadoDelEntorno(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(ENV / "main.tf")
        cls.variables = _leer(ENV / "variables.tf")
        cls.checks = _leer(ENV / "checks.tf")
        cls.backend = _leer(ENV / "backend.tf")

    def test_modulo_identidades_recibe_el_operador(self) -> None:
        modulo = bloque(self.main, 'module "identidades"')
        self.assertIn("principal_operador_arn   = var.principal_operador_arn", modulo)
        self.assertIn('bucket_artefactos_nombre = local.bucket_artefactos_nombre', modulo)
        self.assertIn('bucket_secretos_nombre   = local.bucket_secretos_nombre', modulo)
        self.assertIn("rol_runtime_arn          = module.iam.runtime_role_arn", modulo)

    def test_arn_derivados_del_id_de_cuenta(self) -> None:
        self.assertIn('data "aws_caller_identity" "actual"', self.main)
        self.assertIn("cuenta_id = data.aws_caller_identity.actual.account_id", self.main)
        for local in ("arn_artefactos_publicador", "arn_secretos_publicador", "arn_secretos_gestor_iac"):
            self.assertIn(local, self.main)
        self.assertNotIn("arn:aws:sts::", self.main)

    def test_ya_no_existen_las_variables_arn_vacias(self) -> None:
        for eliminada in ("secretos_publicador_arn", "secretos_gestor_iac_arn", "artefactos_publicador_arn"):
            self.assertNotIn(f'variable "{eliminada}"', self.variables, eliminada)

    def test_check_compara_los_arn_reales(self) -> None:
        self.assertIn('check "identidades_publicacion_coherentes"', self.checks)
        for salida in ("artefactos_publicador_arn", "secretos_publicador_arn", "secretos_gestor_iac_arn"):
            self.assertIn(f"module.identidades.{salida}", self.checks)
        self.assertIn("module.artefactos.bucket_name == local.bucket_artefactos_nombre", self.checks)
        self.assertIn("module.secretos.bucket_name == local.bucket_secretos_nombre", self.checks)

    def test_artifact_object_arns_sin_comodin(self) -> None:
        # D-BASE-02: `releases/*` viola la validación de I-060 (objeto exacto).
        modulo = bloque(self.main, 'module "iam"')
        self.assertIn("artifact_object_arns      = var.iam_artifact_object_arns", modulo)
        self.assertNotIn("releases/*", modulo)

    def test_backend_remoto_con_clave_de_entorno(self) -> None:
        self.assertIn('bucket       = "nexum-h08i-tfstate-713797602090"', self.backend)
        self.assertIn('key          = "envs/h08i/terraform.tfstate"', self.backend)
        self.assertIn("use_lockfile = true", self.backend)
        self.assertIn('region       = "us-east-2"', self.backend)
        self.assertNotIn('"bootstrap/terraform.tfstate"', self.backend)


class PoliticasDeLosRoles(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(MODULO / "main.tf")

    def _documento(self, nombre: str) -> str:
        return bloque(self.main, f'data "aws_iam_policy_document" "{nombre}"')

    def test_artefactos_solo_escribe_en_su_prefijo(self) -> None:
        documento = self._documento("artefactos_publicador")
        self.assertIn('"s3:PutObject"', documento)
        self.assertIn("${local.arn_bucket_artefactos}/${var.prefijo_artefactos}*", documento)
        self.assertNotIn('"s3:GetObject"', documento)

    def test_secretos_publicador_no_lee(self) -> None:
        documento = self._documento("secretos_publicador")
        concesiones, denegaciones = documento.split('sid       = "DenegarLecturaYFueraDePrefijo"')
        self.assertIn('"s3:PutObject"', concesiones)
        self.assertNotIn('"s3:GetObject"', concesiones)
        self.assertIn('"s3:GetObject"', denegaciones)

    def test_gestor_iac_acotado_al_prefijo_de_secretos(self) -> None:
        documento = self._documento("secretos_gestor_iac")
        self.assertIn("${local.arn_bucket_secretos}/${var.prefijo_secretos}*", documento)
        self.assertIn('"s3:GetBucketVersioning"', documento)

    def test_los_tres_deniegan_borrado(self) -> None:
        for nombre in ("artefactos_publicador", "secretos_publicador", "secretos_gestor_iac"):
            with self.subTest(documento=nombre):
                self.assertIn('"s3:DeleteObject"', self._documento(nombre))

    def test_ninguna_politica_concede_asterisco_global(self) -> None:
        for nombre in ("artefactos_publicador", "secretos_publicador", "secretos_gestor_iac"):
            with self.subTest(documento=nombre):
                self.assertNotRegex(self._documento(nombre), r'resources\s*=\s*\[\s*"\*"\s*\]')

    def test_todos_los_modulos_vecinos_declarados(self) -> None:
        # El módulo vive junto a los demás y no duplica el bucket: sólo recibe nombres.
        self.assertTrue((MODULOS / "artefactos").is_dir())
        self.assertTrue((MODULOS / "secretos").is_dir())
        self.assertNotIn('resource "aws_s3_bucket"', self.main)


if __name__ == "__main__":
    unittest.main()
