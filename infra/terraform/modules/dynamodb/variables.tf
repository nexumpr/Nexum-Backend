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
  description = "Entorno lógico; forma parte del nombre NEXUM_CORE_<ENV>"
  type        = string
  default     = "h08i"

  validation {
    condition     = can(regex("^[a-z0-9-]{2,12}$", var.entorno))
    error_message = "entorno debe ser minúsculas, dígitos o guiones (2-12)."
  }
}

variable "region" {
  description = "Región AWS; la tabla y el gateway endpoint de I-010 deben coincidir"
  type        = string
  default     = "us-east-2"
}

variable "nombre_tabla" {
  description = "Nombre base de la tabla única de A-060 (se sufija con el entorno)"
  type        = string
  default     = "NEXUM_CORE"

  validation {
    condition     = var.nombre_tabla == "NEXUM_CORE"
    error_message = "A-060 fija una única tabla NEXUM_CORE; otra tabla exigiría RFC."
  }
}

variable "atributo_ttl" {
  description = "Atributo TTL en epoch seconds, tal como lo fija la matriz física H05"
  type        = string
  default     = "expiresAt"

  validation {
    condition     = var.atributo_ttl == "expiresAt"
    error_message = "La matriz física H05 y A-060 usan `expiresAt`; cambiarlo rompería el adaptador."
  }
}

variable "pitr_habilitado" {
  description = "Point-in-time recovery; presupuestado en I-000 (DDB_PITR 3 GB-mes)"
  type        = bool
  default     = true
}

variable "proteccion_borrado" {
  description = "Deletion protection; en h08i queda desactivada para permitir el teardown del plan Free"
  type        = bool
  default     = false
}

variable "habilitar_gsi2" {
  description = <<-EOT
    GSI2 (DUE#<fecha>#<shard> → <nextDueAt>#<reminderId>) sólo se crea si existe un
    worker programado confirmado (matriz física H05, E057–E060). INF-040-07 prohíbe
    GSI especulativos, así que por defecto está desactivado.
  EOT
  type        = bool
  default     = false
}

variable "etiquetas" {
  description = "Etiquetas adicionales"
  type        = map(string)
  default     = {}
}
