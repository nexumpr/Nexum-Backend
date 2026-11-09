"""Pruebas estructurales de I-060 (IAM, OIDC, secretos y state).

Son pruebas locales del código declarativo; NO consultan AWS, policy simulator,
SCP, OIDC provider existente ni permisos efectivos. Terraform validate tampoco
evalúa las preconditions ni prueba políticas contra la cuenta.
"""

from __future__ import annotations

import re
import unittest
from pathlib import Path

from test_balanceo_contrato import _leer, _sin_comentarios, atributos, recursos

RAIZ = Path(__file__).resolve().parents[3]
MODULO = RAIZ / "infra" / "terraform" / "modules" / "iam"
ENV = RAIZ / "infra" / "terraform" / "envs" / "h08i"
COMPUTO = RAIZ / "infra" / "terraform" / "modules" / "computo"


class RuntimeRole(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(MODULO / "main.tf")
        cls.vars = _leer(MODULO / "variables.tf")
        cls.outs = _leer(MODULO / "outputs.tf")
        cls.r = recursos(cls.main)

    def test_rol_runtime_solo_asumible_por_ec2(self) -> None:
        role = self.r[("aws_iam_role", "runtime")]
        self.assertIn('Service = "ec2.amazonaws.com"', role)
        self.assertIn('Action = "sts:AssumeRole"', role)
        self.assertNotIn('AWS = "*"', role)

    def test_instance_profile_real_compuesto_en_i020(self) -> None:
        self.assertIn(("aws_iam_instance_profile", "runtime"), self.r)
        profile = self.r[("aws_iam_instance_profile", "runtime")]
        self.assertIn("aws_iam_role.runtime.name", profile)
        self.assertIn("iam_instance_profile_name = module.iam.runtime_instance_profile_name", _leer(ENV / "main.tf"))
        self.assertIn("iam_instance_profile", _leer(COMPUTO / "main.tf"))

    def test_table_policy_usa_arn_de_tabla_e_indices_exactos(self) -> None:
        self.assertRegex(self.main, r"Resource\s+= \[var\.dynamodb_table_arn\]")
        self.assertRegex(self.main, r"Resource\s+= var\.dynamodb_index_arns")
        self.assertIn("startswith(arn, \"${var.dynamodb_table_arn}/index/\")", self.main)
        self.assertIn("!strcontains(arn, \"*\")", self.main)
        self.assertIn('dynamodb_index_arns = [', _leer(ENV / "main.tf"))
        self.assertIn('for index_name in keys(module.dynamodb.indices)', _leer(ENV / "main.tf"))

    def test_actions_de_datos_minimas_y_sin_scan(self) -> None:
        for action in (
            "dynamodb:GetItem",
            "dynamodb:PutItem",
            "dynamodb:UpdateItem",
            "dynamodb:DeleteItem",
            "dynamodb:ConditionCheckItem",
            "dynamodb:Query",
        ):
            self.assertIn(f'"{action}"', self.main)
        for forbidden in (
            '"dynamodb:Scan"',
            '"dynamodb:BatchGetItem"',
            '"dynamodb:BatchWriteItem"',
            '"dynamodb:DescribeTable"',
            '"dynamodb:ListTables"',
            '"dynamodb:CreateTable"',
            '"dynamodb:DeleteTable"',
            '"dynamodb:TransactWriteItems"',
            '"dynamodb:*"',
        ):
            self.assertNotIn(forbidden, self.main)
        # TransactWriteItems se autoriza mediante las acciones subyacentes por ítem.
        self.assertIn("TransactWriteItems", self.main)
        self.assertIn("ConditionCheckItem", self.main)

    def test_partition_key_namespaces_limitados(self) -> None:
        for pattern in (
            "USER#*",
            "EMAIL#*",
            "SESSION#*",
            "REFRESH#*",
            "ONE_USE#*",
            "LOOKUP#SESSION#*",
            "PUBLIC#CATALOG",
            "PUBLIC#CONTENT",
        ):
            self.assertIn(f'"{pattern}"', self.main)
        self.assertIn("dynamodb_gsi2_enabled ? [\"DUE#*\"] : []", self.main)
        self.assertIn('"ForAllValues:StringLike"', self.main)

    def test_s3_solo_lectura_de_arn_exactos_sin_secretos(self) -> None:
        statement = self.main.split("artifact_statement =", 1)[1].split("logs_statement =", 1)[0]
        self.assertIn('"s3:GetObject"', statement)
        self.assertIn("var.artifact_object_arns", statement)
        self.assertIn('"aws:SecureTransport" = "true"', statement)
        for forbidden in ('"s3:ListBucket"', '"s3:*"', "nexum/secrets", "secretsmanager:", "ssm:GetParameter"):
            self.assertNotIn(forbidden, self.main)
        self.assertIn('default     = []', self.vars.split('variable "artifact_object_arns"')[1].split("}")[0])

    def test_logs_opcionales_solo_en_grupos_exactos(self) -> None:
        statement = self.main.split("logs_statement =", 1)[1].split("runtime_policy =", 1)[0]
        self.assertIn('"logs:CreateLogStream"', statement)
        self.assertIn('"logs:PutLogEvents"', statement)
        self.assertNotIn('"logs:*"', self.main)
        self.assertIn("BLOCKED_I110_SINK_UNDEFINED", self.outs)

    def test_no_elasticache_connect_con_password_rbac(self) -> None:
        self.assertNotIn("elasticache:Connect", _sin_comentarios(self.main))
        self.assertIn("PASSWORD_RBAC", self.outs)

    def test_no_iam_wildcard_resource_ni_permisos_administrativos(self) -> None:
        self.assertNotRegex(self.main, r"Resource\s*=\s*\[?\s*\"\*\"")
        self.assertNotRegex(self.main, r"Action\s*=\s*\[?\s*\"iam:\*\"")
        # La única excepción a IAM actions es PassRole del apply futuro, acotado
        # al runtime y al servicio EC2; nunca forma parte de la policy runtime.
        runtime = self.r[("aws_iam_role_policy", "runtime")]
        self.assertNotIn("iam:PassRole", runtime)
        passrole = self.r[("aws_iam_role_policy", "terraform_apply_pass_runtime")]
        self.assertIn('"iam:PassRole"', passrole)
        self.assertIn("aws_iam_role.runtime.arn", passrole)
        self.assertIn('"iam:PassedToService" = "ec2.amazonaws.com"', passrole)

    def test_state_secrets_blocked_y_scp_not_verified(self) -> None:
        self.assertIn("AWS_SCP_NOT_VERIFIED", self.outs)
        self.assertIn("I050_TERRAFORM_STATE_RISK", self.outs)
        self.assertIn("No secret permissions", self.outs)


class GitHubOIDC(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(MODULO / "main.tf")
        cls.vars = _leer(MODULO / "variables.tf")
        cls.r = recursos(cls.main)

    def test_immutable_owner_and_repo_ids_verified(self) -> None:
        self.assertIn('github_owner_id = "349666511"', self.main)
        self.assertIn('github_repo_id  = "1447636433"', self.main)
        self.assertIn("repo:nexumpr@${local.github_owner_id}/Nexum-Backend@${local.github_repo_id}", self.main)
        self.assertIn("ref:refs/heads/main", self.main)
        self.assertIn("environment:aws-deploy", self.main)

    def test_audience_and_subject_are_exact_without_wildcards(self) -> None:
        for name in ("ci_validation", "terraform_plan", "terraform_apply"):
            start = self.main.index(f"trust_policy_{name} = {{")
            stop = self.main.find("\n  trust_policy_", start + 1)
            if stop < 0:
                stop = self.main.find("\n}", start)
            trust = self.main[start:stop]
            self.assertIn('"${local.github_issuer}:aud" = local.github_audience', trust)
            self.assertIn('"${local.github_issuer}:sub" = local.github_subject_', trust)
            self.assertNotIn("StringLike", trust)
        self.assertIn('github_audience = "sts.amazonaws.com"', self.main)

    def test_no_provider_created_and_empty_arn_disables_oidc_roles(self) -> None:
        self.assertNotIn("aws_iam_openid_connect_provider", {t for t, _ in self.r})
        self.assertIn('default     = ""', self.vars.split('variable "github_oidc_provider_arn"')[1].split("}")[0])
        self.assertIn("oidc_enabled = var.github_oidc_provider_arn != \"\"", self.main)
        self.assertIn("provider_created    = false", _leer(MODULO / "outputs.tf"))

    def test_apply_requires_protected_environment_and_defaults_off(self) -> None:
        self.assertIn("!var.enable_apply_oidc_role || (local.oidc_enabled && var.apply_environment_protected)", self.main)
        self.assertIn('default     = false', self.vars.split('variable "enable_apply_oidc_role"')[1].split("}")[0])
        self.assertIn('default     = false', self.vars.split('variable "apply_environment_protected"')[1].split("}")[0])

    def test_oidc_roles_have_no_broad_permissions_attached(self) -> None:
        for name in ("ci_validation", "terraform_plan", "terraform_apply"):
            self.assertIn(("aws_iam_role", name), self.r)
        policies = {n for t, n in self.r if t == "aws_iam_role_policy"}
        # RFC-004 (2026-11-09): se anade una policy acotada de lectura del bundle de secretos.
        self.assertEqual(policies, {"runtime", "terraform_apply_pass_runtime", "runtime_secretos"})
        self.assertIn("NONE_I070_BACKEND_AND_RESOURCE_INVENTORY_PENDING", _leer(MODULO / "outputs.tf"))
        self.assertIn("NONE_I080_I090_AND_DEPLOYMENT_GATE_PENDING", _leer(MODULO / "outputs.tf"))

    def test_workflows_do_not_request_oidc_or_aws_credentials(self) -> None:
        for workflow in (RAIZ / ".github" / "workflows").glob("*.yml"):
            text = _leer(workflow)
            if workflow.name == "cd-backend.yml":
                # I-090: OIDC permitido SOLO en el despliegue manual con entorno protegido.
                self.assertIn("workflow_dispatch:", text)
                self.assertIn("environment: aws-deploy", text)
                self.assertIn("id-token: write", text)
                self.assertNotIn("pull_request:", text)
                self.assertNotIn("push:", text)
                self.assertNotIn("AWS_ACCESS_KEY_ID", text)
                self.assertNotIn("AWS_SECRET_ACCESS_KEY", text)
                continue
            self.assertNotIn("id-token: write", text)
            self.assertNotIn("aws-actions/configure-aws-credentials", text)


class IntegracionYDependencias(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(ENV / "main.tf")
        cls.vars = _leer(ENV / "variables.tf")

    def test_module_iam_compuesto_y_instance_profile_entregado_a_i020(self) -> None:
        self.assertIn('module "iam"', self.main)
        self.assertIn('source = "../../modules/iam"', self.main)
        self.assertIn("iam_instance_profile_name = module.iam.runtime_instance_profile_name", self.main)
        self.assertIn("dynamodb_table_arn = module.dynamodb.table_arn", self.main)

    def test_arn_de_bucket_y_log_groups_no_inventados(self) -> None:
        self.assertIn('default     = []', self.vars.split('variable "iam_artifact_object_arns"')[1].split("}")[0])
        self.assertIn('default     = []', self.vars.split('variable "iam_cloudwatch_log_group_arns"')[1].split("}")[0])
        self.assertIn("PENDIENTE-I080", self.vars)


def _statement(main: str, nombre: str) -> str:
    """Devuelve el bloque de un statement local declarado por nombre."""
    inicio = main.index(f"{nombre} = {{")
    nivel, pos = 0, main.index("{", inicio)
    for i in range(pos, len(main)):
        if main[i] == "{":
            nivel += 1
        elif main[i] == "}":
            nivel -= 1
            if nivel == 0:
                return main[inicio : i + 1]
    raise AssertionError(f"statement {nombre} sin cierre")


def _leading_keys_eval(patrones: list[str], request_keys: list[str] | None) -> bool:
    """Evalúa las dos condiciones del statement como lo haría IAM (sin AWS).

    - `Null: {dynamodb:LeadingKeys: "false"}` exige que la clave exista.
    - `ForAllValues:StringLike` es verdadero si el conjunto está vacío/ausente
      (verdad vacua), por eso la conjunción con Null es obligatoria.
    Devuelve True sólo si la petición satisface ambas condiciones.
    """

    if request_keys is None or len(request_keys) == 0:
        # Null:false falla si la clave no está presente (también con conjunto vacío).
        return False
    for value in request_keys:
        if not any(re.fullmatch(p.replace("*", ".*"), value) for p in patrones):
            return False
    return True


class DynamoDBQueryYLeadingKeys(unittest.TestCase):
    """Regresión de los dos defectos corregidos en I-060.

    Estructura + simulación determinista de las condiciones; NO es una evaluación
    de IAM en AWS (IAM_EFFECTIVE_VERIFIED sigue siendo falso).
    """

    PATRONES = [
        "USER#*",
        "EMAIL#*",
        "SESSION#*",
        "REFRESH#*",
        "ONE_USE#*",
        "LOOKUP#SESSION#*",
        "PUBLIC#CATALOG",
        "PUBLIC#CONTENT",
    ]

    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(MODULO / "main.tf")
        cls.tabla = _statement(cls.main, "ddb_statement_table")
        cls.indices = cls.main.split("ddb_statement_indices =", 1)[1].split("] : []", 1)[0]
        cls.condicion = _statement(cls.main, "leading_keys_condition")

    def test_query_permitido_sobre_la_tabla_exacta(self) -> None:
        self.assertIn("Action    = local.ddb_table_actions", self.tabla)
        self.assertIn('concat(local.ddb_item_actions, ["dynamodb:Query"])', self.main)
        self.assertEqual(
            self.tabla.split("Resource")[1].split("\n")[0].strip(),
            "= [var.dynamodb_table_arn]",
        )

    def test_query_permitido_sobre_indices_exactos(self) -> None:
        self.assertIn('Action    = ["dynamodb:Query"]', self.indices)
        self.assertIn("Resource  = var.dynamodb_index_arns", self.indices)

    def test_scan_sigue_excluido(self) -> None:
        self.assertNotIn("dynamodb:Scan", _sin_comentarios(self.main))

    def test_sin_permisos_administrativos_ni_wildcards(self) -> None:
        for prohibido in (
            "dynamodb:CreateTable",
            "dynamodb:DeleteTable",
            "dynamodb:UpdateTable",
            "dynamodb:DescribeTable",
            "dynamodb:ListTables",
            "dynamodb:*",
        ):
            self.assertNotIn(prohibido, self.main)
        self.assertNotRegex(self.main, r"Resource\s*=\s*\"\*\"")

    def test_condiciones_leadingkeys_presentes_en_ambos_statements(self) -> None:
        for statement in (self.tabla, self.indices):
            self.assertIn("Condition = local.leading_keys_condition", statement)

    def test_null_false_y_forallvalues_simultaneos(self) -> None:
        self.assertIn("Null = {", self.condicion)
        self.assertIn('"dynamodb:LeadingKeys" = "false"', self.condicion)
        self.assertIn('"ForAllValues:StringLike" = {', self.condicion)
        self.assertIn('"dynamodb:LeadingKeys" = local.leading_key_patterns', self.condicion)
        self.assertIn("leading_keys_condition", self.main)

    def test_namespaces_no_ampliados(self) -> None:
        for pattern in self.PATRONES:
            self.assertIn(f'"{pattern}"', self.main)
        # DUE# sólo aparece en el concat condicionado por GSI2.
        self.assertIn('var.dynamodb_gsi2_enabled ? ["DUE#*"] : []', self.main)
        self.assertEqual(self.main.count('"DUE#*"'), 1)

    def test_sin_recursos_dynamodb_fuera_de_tabla_e_indices(self) -> None:
        recursos = re.findall(r"Resource\s*=\s*(\[?[^\n]+)", _sin_comentarios(self.main))
        ddb = [r for r in recursos if "dynamodb" in r.lower() or "table_arn" in r or "index_arns" in r]
        self.assertEqual(len(ddb), 2, ddb)

    def test_la_politica_no_sustituye_ownership_en_go(self) -> None:
        self.assertRegex(self.main, r"ownership")
        claves = _leer(RAIZ / "internal" / "domain" / "dynamo" / "keys.go")
        self.assertIn('PrefixUser      = "USER#"', claves)

    def test_gsi2_deshabilitado_no_amplia_la_politica(self) -> None:
        self.assertIn('default     = false', _leer(ENV / "variables.tf").split('variable "dynamodb_habilitar_gsi2"')[1].split("}")[0])
        self.assertIn("var.dynamodb_gsi2_enabled ?", self.main)

    def test_negativo_peticion_sin_leadingkeys_no_autoriza(self) -> None:
        self.assertFalse(_leading_keys_eval(self.PATRONES, None))
        self.assertFalse(_leading_keys_eval(self.PATRONES, []))

    def test_negativo_namespace_no_aprobado_no_autoriza(self) -> None:
        self.assertFalse(_leading_keys_eval(self.PATRONES, ["DUE#2026-11-07#0"]))
        self.assertFalse(_leading_keys_eval(self.PATRONES, ["USER#abc", "ADMIN#1"]))
        self.assertFalse(_leading_keys_eval(self.PATRONES, ["user#minusculas"]))

    def test_positivo_namespaces_aprobados_autorizan(self) -> None:
        self.assertTrue(_leading_keys_eval(self.PATRONES, ["USER#01HXYZ"]))
        self.assertTrue(_leading_keys_eval(self.PATRONES, ["USER#1", "PUBLIC#CATALOG"]))
        self.assertTrue(_leading_keys_eval(self.PATRONES + ["DUE#*"], ["DUE#2026-11-07#3"]))


if __name__ == "__main__":
    unittest.main()
