output "runtime_role_name" {
  description = "Nombre del rol runtime EC2"
  value       = aws_iam_role.runtime.name
}

output "runtime_role_arn" {
  description = "ARN interno para PassRole futuro; sensible para evitar imprimir el ID de cuenta"
  value       = aws_iam_role.runtime.arn
  sensitive   = true
}

output "runtime_instance_profile_name" {
  description = "Instance profile consumido por el Launch Template I-020"
  value       = aws_iam_instance_profile.runtime.name
}

output "runtime_permissions" {
  description = "Acciones/recursos concedidos y dependencias aún pendientes"
  value = {
    dynamodb_table_arn     = var.dynamodb_table_arn
    dynamodb_index_arns    = var.dynamodb_index_arns
    dynamodb_actions       = local.ddb_item_actions
    artifact_object_arns   = var.artifact_object_arns
    artifact_access        = length(var.artifact_object_arns) > 0 ? "EXACT_OBJECTS_ONLY" : "BLOCKED_I080_BUCKET_KEYS_UNKNOWN"
    cloudwatch_log_groups  = var.cloudwatch_log_group_arns
    cloudwatch_logs_access = length(var.cloudwatch_log_group_arns) > 0 ? "EXACT_GROUPS_ONLY" : "BLOCKED_I110_SINK_UNDEFINED"
    secrets_access         = "BLOCKED_RFC004_NOT_APPROVED"
    redis_auth             = "PASSWORD_RBAC; NO elasticache:Connect IAM action"
    ssm_maintenance        = "NOT_GRANTED; I010/I120 maintenance window and exact policy pending"
  }
}

output "oidc" {
  description = "Trust contracts for GitHub; provider ARN/environment must be verified separately"
  value = {
    provider_configured = local.oidc_enabled
    provider_created    = false
    issuer              = "https://token.actions.githubusercontent.com"
    audience            = local.github_audience
    owner_id            = local.github_owner_id
    repository_id       = local.github_repo_id
    ci_subject          = local.github_subject_ci
    plan_subject        = local.github_subject_main
    apply_subject       = local.github_subject_apply
    ci_role_active      = length(aws_iam_role.ci_validation) == 1
    plan_role_active    = length(aws_iam_role.terraform_plan) == 1
    apply_role_active   = length(aws_iam_role.terraform_apply) == 1
    roles_have_policies = false
    apply_environment   = "aws-deploy (not present/protection NOT_VERIFIED)"
  }
}

output "scp_state" {
  description = "SCP is not queried or modified by local I-060"
  value       = "AWS_SCP_NOT_VERIFIED"
}

output "secret_state" {
  description = "No secret permissions or secret material are configured by this module"
  value       = "BLOCKED_RFC004_AND_I050_TERRAFORM_STATE_RISK"
}

output "state_and_workflows" {
  description = "Terraform backend and AWS workflows belong to I-070/I-080 and remain inactive"
  value = {
    backend_state     = "LOCAL_ONLY_I070_PENDING"
    oidc_id_token     = "NO_WORKFLOW_HAS_ID_TOKEN_WRITE"
    plan_permissions  = "NONE_I070_BACKEND_AND_RESOURCE_INVENTORY_PENDING"
    apply_permissions = "NONE_I080_I090_AND_DEPLOYMENT_GATE_PENDING"
  }
}

output "estado_despliegue" {
  description = "I-060 IAM is declared locally; no AWS IAM resources were created"
  value       = "NOT_RUN_AWS"
}
