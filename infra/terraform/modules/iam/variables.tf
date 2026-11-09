variable "proyecto" {
  description = "Prefijo lógico del proyecto"
  type        = string
  default     = "nexum"

  validation {
    condition     = can(regex("^[a-z0-9-]{2,20}$", var.proyecto))
    error_message = "proyecto debe ser minúsculas, dígitos o guiones."
  }
}

variable "entorno" {
  description = "Entorno lógico h08i"
  type        = string
  default     = "h08i"

  validation {
    condition     = can(regex("^[a-z0-9-]{2,12}$", var.entorno))
    error_message = "entorno debe ser minúsculas, dígitos o guiones (2-12)."
  }
}

variable "dynamodb_table_arn" {
  description = "ARN exacto de NEXUM_CORE_<ENV> (I-040), no se acepta wildcard"
  type        = string

  validation {
    condition     = can(regex("^arn:aws:dynamodb:[a-z0-9-]+:[0-9]{12}:table/[^/*]+$", var.dynamodb_table_arn))
    error_message = "dynamodb_table_arn debe ser el ARN exacto de la tabla I-040, sin comodines."
  }
}

variable "dynamodb_index_arns" {
  description = "ARN exactos de índices habilitados de I-040; GSI2 no se añade si está desactivado"
  type        = list(string)
  default     = []

  validation {
    condition = alltrue([
      for arn in var.dynamodb_index_arns :
      can(regex("^arn:aws:dynamodb:[a-z0-9-]+:[0-9]{12}:table/[^/*]+/index/[^/*]+$", arn))
    ])
    error_message = "Cada índice debe ser un ARN exacto de DynamoDB table/index, sin comodines. La relación con table_arn se comprueba como precondition del módulo."
  }
}

variable "dynamodb_gsi2_enabled" {
  description = "Debe reflejar habilitar_gsi2 de I-040; por defecto false"
  type        = bool
  default     = false
}

variable "artifact_object_arns" {
  description = <<-EOT
    ARN exactos de objetos (binario y unidad systemd) que I-020 descarga.
    Vacío hasta que I-080 defina bucket y keys aprobados; no se concede ListBucket
    ni acceso a secretos.
  EOT
  type        = list(string)
  default     = []

  validation {
    condition = alltrue([
      for arn in var.artifact_object_arns :
      can(regex("^arn:aws:s3:::[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]/.+$", arn)) && !strcontains(arn, "*")
    ])
    error_message = "Cada artefacto debe tener ARN exacto de objeto S3, sin bucket o prefix wildcard."
  }
}

variable "cloudwatch_log_group_arns" {
  description = "ARN exactos de log groups definidos por I-110; vacío hasta que exista sink aprobado"
  type        = list(string)
  default     = []

  validation {
    condition = alltrue([
      for arn in var.cloudwatch_log_group_arns :
      can(regex("^arn:aws:logs:[a-z0-9-]+:[0-9]{12}:log-group:[^*]+$", arn)) && !strcontains(arn, "*")
    ])
    error_message = "Log groups deben ser ARN exactos sin wildcard; sólo se añaden permisos a streams de esos grupos."
  }
}

variable "github_oidc_provider_arn" {
  description = <<-EOT
    ARN existente y verificado del IAM OIDC provider de GitHub. Se deja vacío
    porque I-060 no consulta AWS ni crea provider; vacío significa que no se
    instancian roles OIDC. Nunca se crean credenciales estáticas.
  EOT
  type        = string
  default     = ""

  validation {
    condition = (
      var.github_oidc_provider_arn == "" ||
      can(regex("^arn:aws:iam::[0-9]{12}:oidc-provider/token\\.actions\\.githubusercontent\\.com$", var.github_oidc_provider_arn))
    )
    error_message = "OIDC ARN debe ser el provider GitHub exacto; no se admiten issuer ni paths distintos."
  }
}

variable "enable_ci_validation_role" {
  description = "Instancia el rol trust-only para CI validación, sólo si se verifica provider y environment"
  type        = bool
  default     = false
}

variable "ci_environment_protected" {
  description = "Afirmación de que el environment ci-validation existe y tiene branch protection en GitHub; requiere verificación humana"
  type        = bool
  default     = false
}

variable "enable_plan_oidc_role" {
  description = "Instancia el rol trust-only de plan; sin policy hasta I-070"
  type        = bool
  default     = true
}

variable "enable_apply_oidc_role" {
  description = "Instancia el rol trust-only de apply; desactivado hasta environment protegido y Gate de despliegue"
  type        = bool
  default     = false
}

variable "apply_environment_protected" {
  description = "Afirmación externa de protección del environment aws-deploy; requiere verificación humana en GitHub y no cambia permisos AWS"
  type        = bool
  default     = false
}

variable "etiquetas" {
  description = "Etiquetas adicionales"
  type        = map(string)
  default     = {}
}

variable "secretos_object_arns" {
  description = "ARNs del prefijo del bundle de secretos (RFC-004). Vacio = sin permiso."
  type        = list(string)
  default     = []

  validation {
    condition     = alltrue([for arn in var.secretos_object_arns : can(regex("^arn:aws:s3:::[a-z0-9.-]+/.+$", arn))])
    error_message = "Los ARNs de secretos deben apuntar a un prefijo de bucket, no al bucket completo."
  }
}
