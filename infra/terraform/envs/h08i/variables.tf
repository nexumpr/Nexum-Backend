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

variable "iam_instance_profile_name" {
  description = "Instance profile real definido por I-060 (no se crea aquí)"
  type        = string
  default     = "PENDIENTE-I-060"
}

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
