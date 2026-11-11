"""Regresión de la recuperación de fase base (2026-11-10).

Dos defectos reales detectados en el primer `apply` autorizado:

1. `module.iam` calculaba `count = length(aws_iam_role.terraform_apply)`. La
   longitud de la lista de instancias de **otro** recurso no es conocida durante
   `terraform import` (ni en operaciones parciales): Terraform aborta con
   «Invalid count argument» y no se pueden adoptar recursos ya creados.
2. `principal_operador_arn` podía apuntar a un ARN con el path equivocado. AWS
   lo rechaza al crear el rol (`MalformedPolicyDocument: Invalid principal in
   policy`) y el fallo aparece sólo en el `apply`; el ARN de una sesión STS no
   revela el path, así que no sirve como comprobación.

Estas pruebas no contactan AWS ni ejecutan Terraform.
"""

from __future__ import annotations

import re
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
TF = RAIZ / "infra" / "terraform"
MODULOS = TF / "modules"
IAM = MODULOS / "iam"
IDENTIDADES = MODULOS / "identidades_publicacion"


def _leer(p: Path) -> str:
    return p.read_text(encoding="utf-8")


def _sin_comentarios(texto: str) -> str:
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


def _tf() -> list[Path]:
    return [
        p
        for p in TF.rglob("*.tf")
        if ".terraform" not in p.parts and p.is_file()
    ]


# --- Detector: `count` no puede depender de otro recurso --------------------

PATRON_COUNT_RECURSO = re.compile(
    r"(?m)^\s*count\s*=\s*(?:length\(\s*(?:module\.|aws_|data\.)[A-Za-z0-9_.\[\]\"-]+"
    r"|(?:module\.|aws_|data\.)[A-Za-z0-9_.\[\]\"-]+)"
)


def problemas_count(texto: str) -> list[str]:
    """`count` que depende de otro recurso: rompe import y operaciones parciales."""
    limpio = _sin_comentarios(texto)
    return [
        f"count depende de otro recurso: {m.group(0).strip()}"
        for m in PATRON_COUNT_RECURSO.finditer(limpio)
    ]


def nombre_rol_desde_arn(arn: str) -> str | None:
    """Nombre del rol IAM (último segmento) o None si el ARN no es de rol."""
    if ":role/" not in arn:
        return None
    return arn.split(":role/", 1)[1].split("/")[-1]


class CountDeterminista(unittest.TestCase):
    def test_ningun_modulo_cuenta_con_otro_recurso(self) -> None:
        for ruta in _tf():
            with self.subTest(fichero=ruta.relative_to(RAIZ).as_posix()):
                self.assertEqual(problemas_count(_leer(ruta)), [])

    def test_politica_apply_usa_la_expresion_de_variables(self) -> None:
        texto = _leer(IAM / "main.tf")
        self.assertIn(
            "count = local.oidc_enabled && var.enable_apply_oidc_role && var.apply_environment_protected ? 1 : 0",
            texto,
        )
        self.assertNotIn("count = length(aws_iam_role.terraform_apply)", texto)

    def test_el_rol_y_su_politica_comparten_condicion(self) -> None:
        texto = _leer(IAM / "main.tf")
        condicion = (
            "local.oidc_enabled && var.enable_apply_oidc_role && var.apply_environment_protected ? 1 : 0"
        )
        # Dos apariciones: el rol y la política que lo acompaña.
        self.assertEqual(texto.count(condicion), 2)

    # Fixtures: el detector debe tener dientes.
    def test_detector_rechaza_length_de_recurso(self) -> None:
        self.assertTrue(problemas_count('resource "x" "y" {\n  count = length(aws_iam_role.z)\n}\n'))

    def test_detector_rechaza_atributo_de_recurso(self) -> None:
        self.assertTrue(problemas_count('resource "x" "y" {\n  count = aws_lb.z.desired_capacity\n}\n'))

    def test_detector_rechaza_modulo(self) -> None:
        self.assertTrue(problemas_count('resource "x" "y" {\n  count = length(module.red.zonas)\n}\n'))

    def test_detector_acepta_variables(self) -> None:
        self.assertEqual(problemas_count('resource "x" "y" {\n  count = var.habilitar ? 1 : 0\n}\n'), [])


class PrincipalExistente(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(IDENTIDADES / "main.tf")

    def test_resuelve_el_rol_por_data_source(self) -> None:
        self.assertIn('data "aws_iam_role" "operador"', self.main)
        self.assertIn('can(regex(":role/", var.principal_operador_arn)) ? 1 : 0', self.main)
        self.assertIn('reverse(split("/", split(":role/", var.principal_operador_arn)[1]))[0]', self.main)

    def test_precondition_compara_el_arn_real(self) -> None:
        self.assertIn("try(data.aws_iam_role.operador[0].arn, var.principal_operador_arn)", self.main)
        self.assertIn("revisar el path del ARN", self.main)

    def test_nombre_rol_con_y_sin_path(self) -> None:
        self.assertEqual(
            nombre_rol_desde_arn("arn:aws:iam::713797602090:role/managed/AccountFullAccessRole"),
            "AccountFullAccessRole",
        )
        self.assertEqual(
            nombre_rol_desde_arn("arn:aws:iam::713797602090:role/Simple"), "Simple"
        )
        self.assertIsNone(nombre_rol_desde_arn("arn:aws:iam::713797602090:user/operador"))
        self.assertIsNone(
            nombre_rol_desde_arn("arn:aws:sts::713797602090:assumed-role/AccountFullAccessRole/s")
        )

    def test_la_variable_sigue_siendo_obligatoria(self) -> None:
        variables = _leer(IDENTIDADES / "variables.tf")
        cuerpo = variables.split('variable "principal_operador_arn"', 1)[1].split("variable ", 1)[0]
        self.assertNotIn("default", cuerpo)


class SinRecursosHuerfanosPorDiseno(unittest.TestCase):
    """El plan de fase base no debe depender de nombres que ya existan."""

    def test_nombres_deterministas_documentados(self) -> None:
        env = _leer(TF / "envs" / "h08i" / "main.tf")
        self.assertIn('bucket_artefactos_nombre = lower("nexum-${var.entorno}-artefactos-${local.cuenta_id}")', env)
        self.assertIn('bucket_secretos_nombre   = lower("nexum-${var.entorno}-secretos-${local.cuenta_id}")', env)

    def test_dynamodb_con_nombre_de_variable(self) -> None:
        variables = _leer(TF / "envs" / "h08i" / "variables.tf")
        self.assertIn('variable "dynamodb_nombre_tabla"', variables)


if __name__ == "__main__":
    unittest.main()
