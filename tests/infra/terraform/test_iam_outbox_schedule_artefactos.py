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
CHECKS = RAIZ / "infra" / "terraform" / "envs" / "h08i" / "checks.tf"
ENV_MAIN = RAIZ / "infra" / "terraform" / "envs" / "h08i" / "main.tf"
MAIN_TF = IAM.read_text(encoding="utf-8")

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


def _bloque_hcl(texto: str, cabecera: str) -> str:
    """Devuelve el contenido del bloque que empieza en `cabecera`."""

    inicio = texto.find(cabecera)
    if inicio < 0:
        raise AssertionError(f"no se encontró {cabecera!r}")
    apertura = texto.index("{", inicio)
    nivel = 0
    for pos in range(apertura, len(texto)):
        if texto[pos] == "{":
            nivel += 1
        elif texto[pos] == "}":
            nivel -= 1
            if nivel == 0:
                return texto[apertura + 1 : pos]
    raise AssertionError(f"{cabecera!r} sin cierre")


def _lista_requeridos(env_main: str) -> list[str]:
    """Extrae los ARN requeridos resolviendo el nombre del bucket derivado."""

    bloque = env_main.split("artefactos_release_requeridos = [", 1)[1].split("]", 1)[0]
    literales = re.findall(r'"(arn:aws:s3:::[^"]+)"', bloque)
    return [
        literal.replace("${local.bucket_artefactos_nombre}", BUCKET)
        for literal in literales
    ]


def _rollback_ok(requeridos: list[str], efectivos: list[str]) -> bool:
    """Evalúa `alltrue([for arn in requeridos : contains(efectivos, arn)])`."""

    return all(arn in efectivos for arn in requeridos)


def _release_objetivo_ok(efectivos: list[str], release: str) -> bool:
    """Evalúa `release == "" || anytrue(startswith(arn, ".../releases/<release>/"))`."""

    if release == "":
        return True
    prefijo = f"arn:aws:s3:::{BUCKET}/releases/{release}/"
    return any(arn.startswith(prefijo) for arn in efectivos)


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

    def test_precondition_bloqueante_de_rollback(self) -> None:
        self.assertIn('resource "terraform_data" "invariantes_release_rollback"', self.checks)
        bloque = self.checks.split('resource "terraform_data" "invariantes_release_rollback"', 1)[1]
        self.assertIn("for arn in local.artefactos_release_requeridos", bloque)
        self.assertIn("contains(var.iam_artifact_object_arns, arn)", bloque)
        self.assertIn("!strcontains(arn, \"*\")", bloque)
        self.assertIn("!endswith(arn, \"/\")", bloque)
        # La precondition debe ser bloqueante: `check` no detiene el apply.
        self.assertIn("lifecycle", bloque)
        self.assertNotIn("check \"", bloque)

    def test_release_objetivo_exige_un_arn_del_release(self) -> None:
        self.assertIn('variable "release_objetivo"', self.variables)
        self.assertIn("release_objetivo", self.checks)
        self.assertIn("startswith(arn, \"arn:aws:s3:::${local.bucket_artefactos_nombre}/releases/${var.release_objetivo}/\")", self.checks)

    def test_documenta_el_cambio_exacto_del_custodio(self) -> None:
        texto = ARTEFACTOS.read_text(encoding="utf-8")
        self.assertIn("release_objetivo", texto)
        self.assertIn("Conservar los cuatro ARN anteriores", texto)
        self.assertIn("Añadir los **dos** objetos del release nuevo", texto)
        self.assertIn("No usar `*`", texto)


class PunterosBajoUser(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.iam = _leer(IAM)
        cls.patrones = _patrones_leading_keys(cls.iam)

    def test_punteros_no_crean_particiones_globales(self) -> None:
        for puntero in ("OUTBOXPTR#", "SCHEDULEPTR#", "REMINDERPTR#"):
            self.assertNotIn(f'"{puntero}*"', self.patrones, puntero)
        self.assertIn("USER#*", self.patrones)

    def test_documenta_que_los_punteros_viven_bajo_user(self) -> None:
        self.assertIn("OUTBOXPTR#<id>", self.iam)
        self.assertIn("SCHEDULEPTR#<id>", self.iam)
        self.assertIn("REMINDERPTR#<id>", self.iam)
        self.assertIn("quedan cubiertos por `USER#*`", self.iam)

    def test_idem_de_outbox_bajo_user_y_idem_global_conservado(self) -> None:
        self.assertIn("Los IDEM# de outbox/recordatorio", self.iam)
        self.assertIn("bajo USER#", self.iam)
        self.assertIn("IDEM#*", self.patrones)


class RollbackFailClosed(unittest.TestCase):
    """La precondition de rollback debe FALLAR, no avisar, si falta un ARN.

    No se ejecuta `terraform plan` (exigiría backend y credenciales). La prueba
    evalúa la condición HCL real sobre los datos reales del entorno: extrae los
    cuatro ARN de `main.tf`, comprueba que la precondition bloqueante los recorre
    con `contains` y simula la evaluación con listas incompletas. Si la
    condición dejara de ser `alltrue([... contains ...])` o se moviera a un
    `check`, la prueba falla.
    """

    @classmethod
    def setUpClass(cls) -> None:
        cls.checks = _leer(CHECKS)
        cls.requeridos = _lista_requeridos(_leer(ENV_MAIN))
        cls.bloque = _bloque_hcl(
            cls.checks, 'resource "terraform_data" "invariantes_release_rollback"'
        )

    def test_la_lista_requerida_son_los_cuatro_arn_documentados(self) -> None:
        self.assertEqual(sorted(self.requeridos), sorted(ARN_EXACTOS))
        for release in (RELEASE_ROLLBACK, RELEASE_ACTIVO):
            self.assertEqual(
                sum(1 for arn in self.requeridos if f"/releases/{release}/" in arn),
                2,
                release,
            )

    def test_condicion_es_alltrue_contains_y_es_bloqueante(self) -> None:
        self.assertIn("for arn in local.artefactos_release_requeridos", self.bloque)
        self.assertIn("contains(var.iam_artifact_object_arns, arn)", self.bloque)
        self.assertIn("condition = alltrue([", self.bloque)
        # `lifecycle.precondition` sí detiene el plan/apply; `check` solo avisa.
        self.assertIn("lifecycle {", self.bloque)
        self.assertIn("precondition {", self.bloque)
        self.assertNotIn('check "', self.bloque)

    def test_falla_si_falta_cualquiera_de_los_cuatro_arn(self) -> None:
        self.assertTrue(_rollback_ok(self.requeridos, ARN_EXACTOS))
        for arn in ARN_EXACTOS:
            efectivos = [a for a in ARN_EXACTOS if a != arn]
            self.assertFalse(_rollback_ok(self.requeridos, efectivos), arn)

    def test_falla_si_falta_un_release_completo(self) -> None:
        solo_rollback = [a for a in ARN_EXACTOS if RELEASE_ROLLBACK in a]
        solo_activo = [a for a in ARN_EXACTOS if RELEASE_ACTIVO in a]
        self.assertFalse(_rollback_ok(self.requeridos, solo_rollback))
        self.assertFalse(_rollback_ok(self.requeridos, solo_activo))

    def test_release_objetivo_no_vacio_exige_un_arn_de_ese_release(self) -> None:
        self.assertTrue(_release_objetivo_ok(ARN_EXACTOS, ""))
        self.assertTrue(_release_objetivo_ok(ARN_EXACTOS, RELEASE_ACTIVO))
        self.assertFalse(_release_objetivo_ok(ARN_EXACTOS, "v0.21.0+abcdef123456"))
        self.assertFalse(_release_objetivo_ok([], "v0.21.0+abcdef123456"))

    def test_precondition_no_usa_self(self) -> None:
        # Terraform no expone `self` dentro de `precondition`; usarlo rompería
        # `terraform validate`. La prueba lo deja escrito para no reintroducirlo.
        self.assertNotIn("self.", self.bloque)


class ReadAllQueueE063(unittest.TestCase):
    """E063 introduce QUEUE#READALL/REQ#<fecha>#<requestId> y READALLPTR# en el titular.

    `QUEUE#*` ya estaba en la política desde I-060; esta prueba impide que una
    edición futura la retire al tocar OUTBOX#*/SCHEDULE#* y deje el worker sin IAM.
    """

    def test_queue_cubre_la_cola_readall(self) -> None:
        self.assertIn('"QUEUE#*"', MAIN_TF)

    def test_worker_y_purga_declaran_readall(self) -> None:
        # El comentario de la política debe nombrar la cola y el puntero para que
        # el revisor no confunda `QUEUE#*` con una familia ajena.
        self.assertIn("QUEUE#READALL", MAIN_TF)
        self.assertIn("READALLPTR#", MAIN_TF)

    def test_punteros_bajo_user(self) -> None:
        for puntero in ("OUTBOXPTR#", "SCHEDULEPTR#", "REMINDERPTR#", "READALLPTR#"):
            self.assertIn(puntero, MAIN_TF)


if __name__ == "__main__":
    unittest.main()
