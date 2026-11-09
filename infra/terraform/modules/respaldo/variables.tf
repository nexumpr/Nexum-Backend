variable "proyecto" {
  description = "Prefijo de nombres"
  type        = string
  default     = "nexum"

  validation {
    condition     = can(regex("^[a-z0-9-]{2,20}$", var.proyecto))
    error_message = "proyecto invalido."
  }
}

variable "entorno" {
  description = "Entorno logico"
  type        = string
  default     = "h08i"

  validation {
    condition     = can(regex("^[a-z0-9-]{2,12}$", var.entorno))
    error_message = "entorno invalido."
  }
}

variable "region" {
  description = "Region AWS"
  type        = string
  default     = "us-east-2"

  validation {
    condition     = var.region == "us-east-2"
    error_message = "I-000/I-010 fijan us-east-2."
  }
}

variable "tabla_origen" {
  description = "Nombre de la tabla activa que se respalda (I-040)"
  type        = string
}

variable "tabla_arn" {
  description = "ARN de la tabla activa (I-040)"
  type        = string

  validation {
    condition     = can(regex("^arn:aws:dynamodb:us-east-2:[0-9]{12}:table/[A-Za-z0-9_.-]+$", var.tabla_arn))
    error_message = "tabla_arn debe ser un ARN de tabla DynamoDB en us-east-2."
  }
}

variable "pitr_habilitado" {
  description = "PITR de I-040. Sin PITR no se declara capacidad de restauracion a un punto en el tiempo."
  type        = bool

  validation {
    condition     = var.pitr_habilitado
    error_message = "INF-130-02: sin PITR no se puede afirmar restauracion a un punto en el tiempo; habilitarlo o registrarlo como bloqueo."
  }
}


variable "retencion_dias" {
  description = "Retencion declarada de las copias. El PITR de DynamoDB admite como maximo 35 dias."
  type        = number
  default     = 30

  validation {
    condition     = var.retencion_dias >= 1 && var.retencion_dias <= 35
    error_message = "retencion_dias debe estar entre 1 y 35: PITR no conserva mas de 35 dias."
  }
}

variable "state_versionado" {
  description = "Versionado del bucket de state (I-070). Debe ser true."
  type        = bool
  default     = true

  validation {
    condition     = var.state_versionado
    error_message = "INF-130-03: el state debe tener versionado; sin el no hay recuperacion de versiones anteriores."
  }
}

variable "state_cifrado" {
  description = "Cifrado en reposo del state (I-070). Debe ser true."
  type        = bool
  default     = true

  validation {
    condition     = var.state_cifrado
    error_message = "INF-130-03: el state debe estar cifrado; no se admiten copias en claro."
  }
}

variable "state_proteccion_borrado" {
  description = "prevent_destroy del bucket de state (I-070). Debe ser true."
  type        = bool
  default     = true

  validation {
    condition     = var.state_proteccion_borrado
    error_message = "INF-130-03: el bucket de state conserva prevent_destroy; no se puede planificar su destruccion."
  }
}

variable "patron_tabla_destino" {
  description = "Patron obligatorio del nombre de la tabla de restauracion (nunca la tabla activa)."
  type        = string
  default     = "^NEXUM_CORE_RESTORE_[a-z0-9]{4,20}$"

  validation {
    condition     = can(regex("^\\^NEXUM_CORE_RESTORE_", var.patron_tabla_destino))
    error_message = "El patron de restauracion debe exigir el prefijo NEXUM_CORE_RESTORE_."
  }
}

variable "permitir_destroy_datos" {
  description = "Capacidad inexistente: destruir datos esta prohibido en I-130."
  type        = bool
  default     = false

  validation {
    condition     = var.permitir_destroy_datos == false
    error_message = "destroy-data esta prohibido: los datos financieros no se destruyen por rutina de backup o ahorro."
  }
}

variable "permitir_borrado_copias" {
  description = "Borrado de copias fuera de retencion. Requiere politica y Gate; apagado por defecto."
  type        = bool
  default     = false

  validation {
    condition     = var.permitir_borrado_copias == false
    error_message = "El borrado de copias depende de la politica de retencion y de un Gate de despliegue; no se habilita en local."
  }
}
