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
        self.assertIn("Resource = [var.dynamodb_table_arn]", self.main)
        self.assertIn("Resource = var.dynamodb_index_arns", self.main)
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
        self.assertEqual(policies, {"runtime", "terraform_apply_pass_runtime"})
        self.assertIn("NONE_I070_BACKEND_AND_RESOURCE_INVENTORY_PENDING", _leer(MODULO / "outputs.tf"))
        self.assertIn("NONE_I080_I090_AND_DEPLOYMENT_GATE_PENDING", _leer(MODULO / "outputs.tf"))

    def test_workflows_do_not_request_oidc_or_aws_credentials(self) -> None:
        for workflow in (RAIZ / ".github" / "workflows").glob("*.yml"):
            text = _leer(workflow)
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


if __name__ == "__main__":
    unittest.main()
