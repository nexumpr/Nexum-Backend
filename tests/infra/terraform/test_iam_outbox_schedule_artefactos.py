"""Contrato IAM: outbox/scheduler globales y artefactos de rollback.

Cubre tres regresiones reales de preparación H08-E:

1. El outbox push usa `PK=OUTBOX#GLOBAL` y el scheduler de recordatorios usará
   `PK=SCHEDULE#GLOBAL`; sin esos patrones, `LeadingKeys` deniega la operación.
2. La política debe conservar `Query` en tabla e índices y no puede abrir
   `dynamodb:*`, `Scan` ni `Resource = "*"`.
3. El release `v0.19.2+3098f379d7ab` estuvo desplegado y debe seguir en la lista
   de artefactos junto a `v0.20.0+4bf6758d372`; omitirlo rompe el rollback.
"""

from __future__ import annotations

import re
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
IAM = RAIZ / "infra" / "terraform" / "modules" / "iam" / "main.tf"
VARIABLES = RAIZ / "infra" / "terraform" / "envs" / "h08i" / "variables.tf"
ARTEFACTOS = RAIZ / "infra" / "terraform" / "envs" / "h08i" / "ARTEFACTOS_RELEASE.md"

BUCKET = "nexum-h08i-artefactos-713797602090"
RELEASE_ROLLBACK = "v0.19.2+3098f379d7ab"
RELEASE_ACTIVO = "v0.20.0+4bf6758d372"

ARN_EXACTOS = [
    f"arn:aws:s3:::{BUCKET}/releases/{RELEASE_ROLLBACK}/nexum-backend-linux-arm64",
    f"arn:aws:s3:::{BUCKET}/releases/{RELEASE_ROLLBACK}/nexum-backend.service",
    f"arn:aws:s3:::{BUCKET}/releases/{RELEASE_ACTIVO}/nexum-backend-linux-arm64",
    f"arn:aws:s3:::{BUCKET}/releases/{RELEASE_ACTIVO}/nexum-backend.service",
]


def _leer(p: Path) -> str:
    return p.read_text(encoding="utf-8")


def _sin_comentarios(texto: str) -> str:
    return "\n".join(
        re.sub(r"#.*$", "", linea) for linea in texto.splitlines()
    )


def _patrones_leading_keys(texto: str) -> list[str]:
    inicio = texto.index("leading_key_patterns = concat(")
    fin = texto.index("var.dynamodb_gsi2_enabled", inicio)
    return re.findall(r'"([A-Za-z_#*]+)"', texto[inicio:fin])


def _statement(texto: str, nombre: str) -> str:
    inicio = texto.index(f"{nombre} = ")
    fin = texto.find("\n  }", inicio)
    if fin < 0:
        raise AssertionError(f"no se encontró el cierre de {nombre}")
    return texto[inicio : fin + 4]


class OutboxYScheduler(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.iam = _leer(IAM)
        cls.patrones = _patrones_leading_keys(cls.iam)

    def test_outbox_y_schedule_estan_autorizados(self) -> None:
        self.assertIn("OUTBOX#*", self.patrones)
        self.assertIn("SCHEDULE#*", self.patrones)

    def test_origen_y_minimo_privilegio_documentados(self) -> None:
        inicio = self.iam.index("leading_key_patterns = concat(")
        fin = self.iam.index("var.dynamodb_gsi2_enabled", inicio)
        bloque = self.iam[inicio:fin]
        self.assertIn("OUTBOX#GLOBAL", bloque)
        self.assertIn("SCHEDULE#GLOBAL", bloque)
        self.assertIn("SK=DUE#<RFC3339Nano>#<id>", bloque)
        # Las particiones globales no justifican Scan ni acciones administrativas.
        self.assertNotIn("dynamodb:Scan", self.iam)
        self.assertNotIn("dynamodb:*", self.iam)

    def test_particiones_previas_intactas(self) -> None:
        for patron in (
            "USER#*",
            "EMAIL#*",
            "IDEM#*",
            "SESSION#*",
            "REFRESH#*",
            "ONE_USE#*",
            "LOOKUP#SESSION#*",
            "RESET#*",
            "QUEUE#*",
            "PUBLIC#CATALOG",
            "PUBLIC#CONTENT",
        ):
            self.assertIn(patron, self.patrones, patron)


class AccionesDynamoDB(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.iam = _leer(IAM)
        cls.tabla = _statement(cls.iam, "ddb_statement_table")
        cls.indices = cls.iam.split("ddb_statement_indices =", 1)[1].split("] : []", 1)[0]

    def test_acciones_de_datos_exactas(self) -> None:
        self.assertIn("local.ddb_table_actions", self.tabla)
        for accion in (
            "dynamodb:GetItem",
            "dynamodb:PutItem",
            "dynamodb:UpdateItem",
            "dynamodb:DeleteItem",
            "dynamodb:ConditionCheckItem",
        ):
            self.assertIn(accion, self.iam)
        self.assertIn('concat(local.ddb_item_actions, ["dynamodb:Query"])', self.iam)

    def test_query_en_tabla_e_indices(self) -> None:
        self.assertIn('"dynamodb:Query"', self.iam)
        self.assertIn('Action    = ["dynamodb:Query"]', self.indices)
        self.assertIn("Resource  = var.dynamodb_index_arns", self.indices)

    def test_recursos_exactos_sin_wildcard_de_tabla(self) -> None:
        self.assertIn("Resource  = [var.dynamodb_table_arn]", self.tabla)
        self.assertIn("Resource  = var.dynamodb_index_arns", self.indices)
        self.assertNotIn("Resource  = \"*\"", self.iam)
        self.assertNotIn("Resource = \"*\"", self.iam)
        self.assertNotIn("arn:aws:dynamodb:*", self.iam)

    def test_sin_scan_ni_administracion(self) -> None:
        self.assertNotIn("dynamodb:Scan", self.iam)
        self.assertNotIn("dynamodb:*", self.iam)
        self.assertNotIn("dynamodb:DescribeTable", self.indices)
        # DescribeTable es de plano de control y usa el ARN exacto de la tabla.
        describir = _statement(self.iam, "ddb_statement_describe")
        self.assertIn('Action   = ["dynamodb:DescribeTable"]', describir)
        self.assertIn("Resource = var.dynamodb_table_arn", describir)


class ArtefactosDeRelease(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.artefactos = _leer(ARTEFACTOS)
        cls.variables = _leer(VARIABLES)
        cls.env_main = _leer(RAIZ / "infra" / "terraform" / "envs" / "h08i" / "main.tf")
        cls.checks = _leer(RAIZ / "infra" / "terraform" / "envs" / "h08i" / "checks.tf")

    def test_release_de_rollback_y_activo_documentados(self) -> None:
        self.assertIn(RELEASE_ROLLBACK, self.artefactos)
        self.assertIn(RELEASE_ACTIVO, self.artefactos)

    def test_cuatro_arn_exactos_presentes(self) -> None:
        for arn in ARN_EXACTOS:
            self.assertIn(arn, self.artefactos, arn)

    def test_sin_wildcards_en_los_arn_de_ejemplo(self) -> None:
        bloque = self.artefactos.split("```hcl", 1)[1].split("```", 1)[0]
        for linea in bloque.splitlines():
            if "arn:aws:s3:::" not in linea:
                continue
            self.assertNotIn("*", linea, linea)
            self.assertFalse(linea.rstrip().rstrip('",').endswith("/"), linea)

    def test_dos_objetos_por_release(self) -> None:
        for release in (RELEASE_ROLLBACK, RELEASE_ACTIVO):
            self.assertEqual(
                self.artefactos.count(f"/releases/{release}/nexum-backend-linux-arm64"),
                1,
                release,
            )
            self.assertEqual(
                self.artefactos.count(f"/releases/{release}/nexum-backend.service"),
                1,
                release,
            )

    def test_variable_rechaza_wildcard_y_prefijo(self) -> None:
        bloque = self.variables.split('variable "iam_artifact_object_arns"', 1)[1]
        self.assertIn("alltrue", bloque)
        self.assertIn("!strcontains(arn, \"*\")", bloque)
        self.assertIn("!endswith(arn, \"/\")", bloque)
        self.assertIn("arn:aws:s3:::", bloque)

    def test_entorno_declara_los_arn_requeridos_y_deriva_el_bucket(self) -> None:
        self.assertIn("artefactos_release_requeridos", self.env_main)
        for release in (RELEASE_ROLLBACK, RELEASE_ACTIVO):
            self.assertIn(release, self.env_main, release)
        self.assertIn("${local.bucket_artefactos_nombre}/releases/", self.env_main)
        self.assertNotIn("713797602090", self.env_main)

    def test_check_de_rollback_compara_la_lista_efectiva(self) -> None:
        self.assertIn('check "artefactos_release_rollback"', self.checks)
        self.assertIn("contains(var.iam_artifact_object_arns, arn)", self.checks)
        self.assertIn("!strcontains(arn, \"*\")", self.checks)
        self.assertIn("v0.19.2", ARTEFACTOS.read_text(encoding="utf-8"))
        self.assertIn("v0.20.0", ARTEFACTOS.read_text(encoding="utf-8"))


if __name__ == "__main__":
    unittest.main()
