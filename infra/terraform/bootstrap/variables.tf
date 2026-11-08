variable "proyecto" {
  description = "Prefijo de nombres"
  type        = string
  default     = "nexum"

  validation {
    condition     = can(regex("^[a-z0-9-]{2,20}$", var.proyecto))
    error_message = "proyecto debe ser minúsculas, dígitos o guiones."
  }
}

variable "entorno" {
  description = "Entorno lógico (h08i)"
  type        = string
  default     = "h08i"
}

variable "region" {
  description = "Región del bucket/tabla de state; us-east-2"
  type        = string
  default     = "us-east-2"
}

variable "sufijo_cuenta" {
  description = <<-EOT
    Sufijo único para el nombre global del bucket (por ejemplo, el ID de cuenta o
    un alias corto). Sin valor por defecto: el bootstrap no debe inventarlo. No es
    un secreto, pero no se publica en el informe.
  EOT
  type        = string
  default     = ""
}

variable "bucket_state_nombre" {
  description = "Nombre explícito del bucket de state; si está vacío se compone proyecto-entorno-sufijo"
  type        = string
  default     = ""
}

variable "dias_retencion_versiones" {
  description = "Días de retención de versiones no actuales del state"
  type        = number
  default     = 30

  validation {
    condition     = var.dias_retencion_versiones >= 7 && var.dias_retencion_versiones <= 365
    error_message = "La retención de versiones debe estar entre 7 y 365 días."
  }
}

variable "etiquetas" {
  description = "Etiquetas adicionales"
  type        = map(string)
  default     = {}
}
