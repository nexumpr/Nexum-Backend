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
  description = "Prefijo S3 de los releases (debe terminar en '/')."
  type        = string
  default     = "releases/"

  validation {
    condition     = endswith(var.prefijo, "/")
    error_message = "El prefijo debe terminar en '/'."
  }
}

variable "retencion_version_dias" {
  description = "Dias que se conservan las versiones no actuales del artefacto."
  type        = number
  default     = 365
}

variable "rol_runtime_arn" {
  description = "ARN del rol de instancia: solo descarga artefactos."
  type        = string
}

variable "publicador_arn" {
  description = "ARN del rol de CI que publica binarios y SBOM."
  type        = string
  default     = ""
}

variable "gestor_iac_arn" {
  description = "ARN del rol de Terraform; exento del Deny de ListBucket para HeadBucket."
  type        = string
  default     = ""
}
