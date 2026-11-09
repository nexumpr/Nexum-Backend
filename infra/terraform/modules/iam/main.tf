# I-060 — Identidades IAM, least privilege y trust OIDC declarativos.
#
# I-060 se implementa localmente. Ningún recurso IAM se ha creado: AWS_NOT_RUN.
# El rol runtime recibe permisos de datos exactos para la tabla/índices I-040.
# S3/logs sólo reciben permisos cuando otra spec aporta ARN exactos.
#
# Bloqueos explícitos:
# - RFC-004 PROPUESTO: no hay permisos de secretos; no se aprueba S3 bundle.
# - I-050 Redis usa contraseña RBAC. No se concede elasticache:Connect (IAM auth).
# - I-070 no define backend/state/locking: OIDC plan/apply no recibe policies.
# - I-080/I-090 no han creado workflows AWS ni id-token: write.
# - El OIDC provider y el environment aws-deploy no se consultan/crean aquí.

terraform {
  required_version = ">= 1.4.0, < 2.0.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.72"
    }
  }
}

locals {
  nombre = "${var.proyecto}-${var.entorno}"

  # IDs inmutables del repo verificados con GitHub API (repo creado 2026-11-05,
  # después de la adopción de immutable sub claims del 2026-07-15).
  github_owner_id = "349666511"
  github_repo_id  = "1447636433"
  github_issuer   = "token.actions.githubusercontent.com"
  github_audience = "sts.amazonaws.com"

  github_subject_main  = "repo:nexumpr@${local.github_owner_id}/Nexum-Backend@${local.github_repo_id}:ref:refs/heads/main"
  github_subject_ci    = "repo:nexumpr@${local.github_owner_id}/Nexum-Backend@${local.github_repo_id}:environment:ci-validation"
  github_subject_apply = "repo:nexumpr@${local.github_owner_id}/Nexum-Backend@${local.github_repo_id}:environment:aws-deploy"

  oidc_enabled = var.github_oidc_provider_arn != ""

  # Namespaces PK de H06/A-060. El rol EC2 se comparte entre usuarios y no puede
  # sustituir el ownership que valida Go usando `sub` del JWT (R-031). LeadingKeys
  # restringe las particiones permitidas; no intenta dar aislamiento por usuario.
  leading_key_patterns = concat(
    [
      "USER#*",
      "EMAIL#*",
      "SESSION#*",
      "REFRESH#*",
      "ONE_USE#*",
      "LOOKUP#SESSION#*",
      "PUBLIC#CATALOG",
      "PUBLIC#CONTENT",
    ],
    var.dynamodb_gsi2_enabled ? ["DUE#*"] : []
  )

  # IAM autoriza TransactWriteItems por las acciones subyacentes de cada item
  # (PutItem/UpdateItem/DeleteItem/ConditionCheckItem), no por una acción
  # dynamodb:TransactWriteItems. Fuente: AWS DynamoDB transaction IAM docs.
  ddb_item_actions = [
    "dynamodb:GetItem",
    "dynamodb:PutItem",
    "dynamodb:UpdateItem",
    "dynamodb:DeleteItem",
    "dynamodb:ConditionCheckItem",
  ]

  # Query también se necesita sobre la partición del usuario (movimientos,
  # presupuestos, metas, consentimientos...), no sólo sobre GSI1/GSI2. Sin esta
  # acción, la matriz física E001–E075 devolvería AccessDenied en lecturas por PK
  # y SK, que son el patrón dominante de A-060.
  ddb_table_actions = concat(local.ddb_item_actions, ["dynamodb:Query"])

  # `ForAllValues:StringLike` es verdadero si la clave no está presente (verdad
  # vacua). Para exigir que TODA petición lleve dynamodb:LeadingKeys se añade
  # `Null = {"dynamodb:LeadingKeys" = "false"}`, que exige su presencia. Ambos
  # controles se mantienen simultáneamente.
  leading_keys_condition = {
    Null = {
      "dynamodb:LeadingKeys" = "false"
    }
    "ForAllValues:StringLike" = {
      "dynamodb:LeadingKeys" = local.leading_key_patterns
    }
  }

  ddb_statement_table = {
    Sid       = "DynamoDBItemsOnApprovedTable"
    Effect    = "Allow"
    Action    = local.ddb_table_actions
    Resource  = [var.dynamodb_table_arn]
    Condition = local.leading_keys_condition
  }

  ddb_statement_indices = length(var.dynamodb_index_arns) > 0 ? [
    {
      Sid       = "QueryApprovedDynamoDBIndexes"
      Effect    = "Allow"
      Action    = ["dynamodb:Query"]
      Resource  = var.dynamodb_index_arns
      Condition = local.leading_keys_condition
    }
  ] : []

  # Bucket y keys de binario/unit todavía son placeholders de I-080. Sólo si I-080
  # provee los ARN exactos se habilita GetObject, sin ListBucket ni escritura.
  artifact_statement = length(var.artifact_object_arns) > 0 ? [
    {
      Sid      = "ReadOnlyExactBootstrapArtifacts"
      Effect   = "Allow"
      Action   = ["s3:GetObject"]
      Resource = var.artifact_object_arns
      Condition = {
        Bool = {
          "aws:SecureTransport" = "true"
        }
      }
    }
  ] : []

  # I-110 debe definir grupos/sink antes de conceder escritura de logs. El módulo
  # no inventa log group, namespace ni acción global `cloudwatch:PutMetricData`.
  logs_statement = length(var.cloudwatch_log_group_arns) > 0 ? [
    {
      Sid      = "WriteOnlyApprovedLogGroups"
      Effect   = "Allow"
      Action   = ["logs:CreateLogStream", "logs:PutLogEvents"]
      Resource = [for arn in var.cloudwatch_log_group_arns : "${arn}:*"]
      Condition = {
        Bool = {
          "aws:SecureTransport" = "true"
        }
      }
    }
  ] : []

  runtime_policy = {
    Version = "2012-10-17"
    Statement = concat(
      [local.ddb_statement_table],
      local.ddb_statement_indices,
      local.artifact_statement,
      local.logs_statement
    )
  }

  # Trust contracts use immutable GitHub subject claims. The provider ARN is
  # injected only after a separate AWS read confirms it exists. No wildcard sub.
  trust_policy_ci_validation = {
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Principal = {
        Federated = var.github_oidc_provider_arn
      }
      Action = "sts:AssumeRoleWithWebIdentity"
      Condition = {
        StringEquals = {
          "${local.github_issuer}:aud" = local.github_audience
          "${local.github_issuer}:sub" = local.github_subject_ci
        }
      }
    }]
  }

  trust_policy_terraform_plan = {
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Principal = {
        Federated = var.github_oidc_provider_arn
      }
      Action = "sts:AssumeRoleWithWebIdentity"
      Condition = {
        StringEquals = {
          "${local.github_issuer}:aud" = local.github_audience
          "${local.github_issuer}:sub" = local.github_subject_main
        }
      }
    }]
  }

  trust_policy_terraform_apply = {
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Principal = {
        Federated = var.github_oidc_provider_arn
      }
      Action = "sts:AssumeRoleWithWebIdentity"
      Condition = {
        StringEquals = {
          "${local.github_issuer}:aud" = local.github_audience
          "${local.github_issuer}:sub" = local.github_subject_apply
        }
      }
    }]
  }
}

resource "terraform_data" "iam_preconditions" {
  input = {
    table_arn       = var.dynamodb_table_arn
    index_arns      = var.dynamodb_index_arns
    oidc_provider   = var.github_oidc_provider_arn
    apply_role      = var.enable_apply_oidc_role
    apply_protected = var.apply_environment_protected
    artifacts       = var.artifact_object_arns
    log_groups      = var.cloudwatch_log_group_arns
  }

  lifecycle {
    precondition {
      condition     = can(regex("^arn:aws:dynamodb:[a-z0-9-]+:[0-9]{12}:table/[^/*]+$", var.dynamodb_table_arn))
      error_message = "dynamodb_table_arn debe ser el ARN exacto de la tabla I-040, sin comodines."
    }

    precondition {
      condition = alltrue([
        for arn in var.dynamodb_index_arns :
        startswith(arn, "${var.dynamodb_table_arn}/index/") && !strcontains(arn, "*")
      ])
      error_message = "Cada índice DynamoDB debe ser un ARN exacto bajo la tabla aprobada, sin comodines."
    }

    precondition {
      condition     = !var.enable_apply_oidc_role || (local.oidc_enabled && var.apply_environment_protected)
      error_message = "El rol apply exige OIDC verificado y protection del environment aws-deploy en GitHub."
    }

    precondition {
      condition = alltrue([
        for arn in var.artifact_object_arns :
        can(regex("^arn:aws:s3:::[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]/.+$", arn)) && !strcontains(arn, "*")
      ])
      error_message = "Artefactos deben ser ARN exactos de objetos S3; no se permiten bucket/prefix wildcards."
    }

    precondition {
      condition = alltrue([
        for arn in var.cloudwatch_log_group_arns :
        can(regex("^arn:aws:logs:[a-z0-9-]+:[0-9]{12}:log-group:[^*]+$", arn)) && !strcontains(arn, "*")
      ])
      error_message = "Log groups deben ser ARN exactos; el módulo añadirá sólo el sufijo de streams."
    }
  }
}

# --- Runtime EC2 ------------------------------------------------------------

resource "aws_iam_role" "runtime" {
  name                 = "${local.nombre}-backend-runtime"
  description          = "Rol EC2 runtime Nexum; permisos mínimos para DynamoDB"
  max_session_duration = 3600

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Principal = {
        Service = "ec2.amazonaws.com"
      }
      Action = "sts:AssumeRole"
    }]
  })

  tags = merge({
    Name = "${local.nombre}-backend-runtime"
    Spec = "I-060-iam-seguridad"
  }, var.etiquetas)
}

resource "aws_iam_instance_profile" "runtime" {
  name = "${local.nombre}-backend-runtime"
  role = aws_iam_role.runtime.name

  tags = merge({
    Name = "${local.nombre}-backend-runtime"
    Spec = "I-060-iam-seguridad"
  }, var.etiquetas)
}

resource "aws_iam_role_policy" "runtime" {
  name   = "${local.nombre}-runtime-approved-data-access"
  role   = aws_iam_role.runtime.id
  policy = jsonencode(local.runtime_policy)
}

# --- GitHub OIDC: trust-only roles (no AWS permissions attached) -------------
# Provider ARN vacío por defecto: I-060 no consulta AWS ni crea un provider que
# pueda colisionar con uno preexistente administrado fuera de este Terraform.
# Ninguno de estos roles recibe permissions policy: I-070 define state/locking,
# I-080/I-090 definen los jobs y su matriz completa de permisos.

resource "aws_iam_role" "ci_validation" {
  count = local.oidc_enabled && var.enable_ci_validation_role && var.ci_environment_protected ? 1 : 0

  name                 = "${local.nombre}-github-ci-validate"
  description          = "Trust-only CI validation role; no AWS permissions attached"
  max_session_duration = 3600
  assume_role_policy   = jsonencode(local.trust_policy_ci_validation)

  tags = merge({
    Name = "${local.nombre}-github-ci-validate"
    Spec = "I-060-iam-seguridad"
  }, var.etiquetas)
}

resource "aws_iam_role" "terraform_plan" {
  count = local.oidc_enabled && var.enable_plan_oidc_role ? 1 : 0

  name                 = "${local.nombre}-github-terraform-plan"
  description          = "Trust-only Terraform plan role; permissions await I-070 backend/state"
  max_session_duration = 3600
  assume_role_policy   = jsonencode(local.trust_policy_terraform_plan)

  tags = merge({
    Name = "${local.nombre}-github-terraform-plan"
    Spec = "I-060-iam-seguridad"
  }, var.etiquetas)
}

resource "aws_iam_role" "terraform_apply" {
  count = local.oidc_enabled && var.enable_apply_oidc_role && var.apply_environment_protected ? 1 : 0

  name                 = "${local.nombre}-github-terraform-apply"
  description          = "Trust-only apply role; requires protected aws-deploy environment and I-080/I-090 permissions"
  max_session_duration = 3600
  assume_role_policy   = jsonencode(local.trust_policy_terraform_apply)

  tags = merge({
    Name = "${local.nombre}-github-terraform-apply"
    Spec = "I-060-iam-seguridad"
  }, var.etiquetas)
}

# El único permiso IAM implementado para apply en I-060: permitir pasar el rol
# runtime exacto al servicio EC2. No concede Create/Update/Delete a ningún recurso;
# el resto de la matriz IaC depende de I-070/I-080/I-090.
resource "aws_iam_role_policy" "terraform_apply_pass_runtime" {
  count = length(aws_iam_role.terraform_apply)

  name = "${local.nombre}-apply-pass-runtime-to-ec2"
  role = aws_iam_role.terraform_apply[0].id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid      = "PassOnlyNexumRuntimeRoleToEC2"
      Effect   = "Allow"
      Action   = ["iam:PassRole"]
      Resource = [aws_iam_role.runtime.arn]
      Condition = {
        StringEquals = {
          "iam:PassedToService" = "ec2.amazonaws.com"
        }
      }
    }]
  })
}

# RFC-004 opcion B: lectura del bundle de secretos. Permiso de LECTURA y solo sobre
# el prefijo del bundle; no se concede PutObject ni acceso al bucket completo.
resource "aws_iam_role_policy" "runtime_secretos" {
  count = length(var.secretos_object_arns) > 0 ? 1 : 0

  name = "${local.nombre}-runtime-secretos-lectura"
  role = aws_iam_role.runtime.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid      = "LeerBundleDeSecretos"
        Effect   = "Allow"
        Action   = ["s3:GetObject"]
        Resource = var.secretos_object_arns
      }
    ]
  })
}
