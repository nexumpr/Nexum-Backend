variable "proyecto" {
  description = "Prefijo del nombre del bucket."
  type        = string
}

variable "entorno" {
  description = "Entorno (h08i, prod...)."
  type        = string
}

variable "etiquetas" {
  description = "Etiquetas comunes."
  type        = map(string)
  default     = {}
}

variable "prefijo" {
  description = "Prefijo S3 del bundle de secretos (debe terminar en '/')."
  type        = string
  default     = "nexum/secrets/"

  validation {
    condition     = endswith(var.prefijo, "/")
    error_message = "El prefijo debe terminar en '/'."
  }
}

variable "retencion_version_dias" {
  description = "Dias que se conservan las versiones no actuales del bundle (RFC-004)."
  type        = number
  default     = 30
}

variable "rol_runtime_arn" {
  description = "ARN del rol de instancia; unico principal autorizado sobre el prefijo."
  type        = string
}

variable "publicador_arn" {
  description = "ARN de la identidad IAM autorizada a publicar el bundle (RFC-004). Sin valor valido, el plan falla cerrado."
  type        = string
  default     = ""
}
