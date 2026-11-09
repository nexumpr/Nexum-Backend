variable "entorno" {
  description = "Entorno lógico"
  type        = string
  default     = "h08i"
}

variable "region" {
  description = "Región AWS"
  type        = string
  default     = "us-east-2"
}

variable "cidr_vpc" {
  description = "CIDR RFC1918 de la VPC"
  type        = string
  default     = "10.42.0.0/16"
}

variable "bits_subredes" {
  description = "Bits para derivar las subredes privadas del CIDR de la VPC"
  type        = number
  default     = 8
}

variable "zonas" {
  description = "Zonas de disponibilidad de la región"
  type        = list(string)
  default     = ["us-east-2a", "us-east-2b"]
}

variable "puerto_backend" {
  description = "Puerto interno del backend Go"
  type        = number
  default     = 8080
}

variable "ventana_mantenimiento_activa" {
  description = "Activa los interface endpoints de mantenimiento (Secrets Manager, SSM)"
  type        = bool
  default     = false
}

variable "etiquetas" {
  description = "Etiquetas adicionales para default_tags"
  type        = map(string)
  default     = {}
}

# --- I-020: cómputo ---------------------------------------------------------

variable "root_volume_gb" {
  description = "Volumen raíz por instancia (presupuesto: 15 GB x2)"
  type        = number
  default     = 15
}

variable "artefacto_s3_uri" {
  description = "URI S3 del binario linux/arm64"
  type        = string
  default     = "s3://PENDIENTE-I080/nexum-backend"
}

variable "artefacto_sha256" {
  description = "SHA-256 del binario (lo emite scripts/build-backend.ps1)"
  type        = string
  default     = ""
}

variable "unidad_s3_uri" {
  description = "URI S3 de la unidad systemd"
  type        = string
  default     = "s3://PENDIENTE-I080/nexum-backend.service"
}

variable "unidad_sha256" {
  description = "SHA-256 de la unidad systemd"
  type        = string
  default     = ""
}

# --- I-030: balanceo y publicación HTTPS --------------------------------

variable "health_check_path" {
  description = "Ruta de liveness del target group (A-090); readiness queda excluida por 503 de diseño"
  type        = string
  default     = "/api/v1/health/live"
}

variable "cloudfront_price_class" {
  description = "Price class de CloudFront; PriceClass_100 es la más barata"
  type        = string
  default     = "PriceClass_100"
}

# --- I-040: DynamoDB ---------------------------------------------------------

variable "dynamodb_nombre_tabla" {
  description = "Nombre base de la tabla única de A-060 (el módulo añade el entorno)"
  type        = string
  default     = "NEXUM_CORE"
}

variable "dynamodb_atributo_ttl" {
  description = "Atributo TTL epoch seconds fijado por la matriz física H05"
  type        = string
  default     = "expiresAt"
}

variable "dynamodb_pitr_habilitado" {
  description = "PITR presupuestado en I-000 (DDB_PITR)"
  type        = bool
  default     = true
}

variable "dynamodb_proteccion_borrado" {
  description = "Deletion protection; desactivada en h08i para permitir teardown autorizado"
  type        = bool
  default     = false
}

variable "dynamodb_habilitar_gsi2" {
  description = "GSI2 para workers programados (E057–E060); desactivado si no hay scheduler confirmado"
  type        = bool
  default     = false
}

# --- I-050: Redis OSS compartido --------------------------------------------

variable "redis_password_aplicacion" {
  description = <<-EOT
    Contraseña del usuario RBAC de Redis. Sensible y sin valor por defecto: en
    I-050 no hay despliegue y ningún secreto se versiona. En despliegue llegará
    por el mecanismo aprobado (RFC-004, aún PROPUESTO) o por variable de entorno
    fuera del repositorio.
  EOT
  type        = string
  sensitive   = true
  default     = ""
}

# --- I-060: IAM / OIDC ------------------------------------------------------

variable "iam_artifact_object_arns" {
  description = "ARN exactos de los dos objetos S3 de I-020; vacío mientras I-080 no defina bucket y keys"
  type        = list(string)
  default     = []
}

variable "iam_cloudwatch_log_group_arns" {
  description = "ARN exactos de log groups de I-110; vacío hasta que esa spec defina un sink"
  type        = list(string)
  default     = []
}

variable "iam_github_oidc_provider_arn" {
  description = "ARN existente/verificado de token.actions.githubusercontent.com; no se consulta ni crea en I-060"
  type        = string
  default     = ""
}

variable "iam_enable_ci_validation_role" {
  description = "Activa role trust-only CI tras verificar provider y environment protegido"
  type        = bool
  default     = false
}

variable "iam_ci_environment_protected" {
  description = "Confirmación externa de protection del environment ci-validation en GitHub"
  type        = bool
  default     = false
}

variable "iam_enable_plan_oidc_role" {
  description = "Activa role trust-only plan; permisos de state/backend pertenecen a I-070"
  type        = bool
  default     = true
}

variable "iam_enable_apply_oidc_role" {
  description = "Activa role trust-only apply; permanece desactivado hasta Gate de despliegue"
  type        = bool
  default     = false
}

variable "iam_apply_environment_protected" {
  description = "Confirmación externa de protection del environment aws-deploy en GitHub"
  type        = bool
  default     = false
}

variable "secretos_version" {
  description = "Version del bundle de secretos a fijar en el bootstrap (RFC-004). Vacio = sin bundle."
  type        = string
  default     = ""
}
