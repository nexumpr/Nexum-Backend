# RFC-011 — Variables del módulo de identidades de publicación.

variable "proyecto" {
  description = "Prefijo de nombres"
  type        = string
}

variable "entorno" {
  description = "Entorno lógico (h08i)"
  type        = string
}

variable "etiquetas" {
  description = "Etiquetas adicionales"
  type        = map(string)
  default     = {}
}

variable "cuenta_id" {
  description = "ID de la cuenta AWS (12 dígitos). Se usa para derivar los ARN de rol de forma determinista."
  type        = string
}

variable "principal_operador_arn" {
  description = "ARN del principal humano/operador autorizado a asumir las tres identidades. Sin valor por defecto: fail-closed."
  type        = string

  validation {
    condition     = can(regex("^arn:aws:iam::[0-9]{12}:(role|user)/[A-Za-z0-9+=,.@_/-]{1,64}$", var.principal_operador_arn))
    error_message = "principal_operador_arn debe ser un ARN IAM concreto de rol o usuario con nombre válido (fail-closed)."
  }

  # El ARN de una sesión (`arn:aws:sts::…:assumed-role/<rol>/<sesión>`) cambia en
  # cada login y no es un principal estable: anclar la confianza ahí rompería el
  # despliegue en la siguiente sesión. Se exige el ARN IAM del rol real.
  validation {
    condition     = !startswith(var.principal_operador_arn, "arn:aws:sts::")
    error_message = "principal_operador_arn no puede ser un ARN STS de sesión; use el ARN IAM del rol (arn:aws:iam::…:role/…)."
  }

  validation {
    condition     = !endswith(var.principal_operador_arn, ":root")
    error_message = "El trust no puede apuntar a la cuenta raíz."
  }
}

variable "bucket_artefactos_nombre" {
  description = "Nombre del bucket de artefactos (sin ARN, para evitar ciclos con el módulo de bucket)."
  type        = string
}

variable "bucket_secretos_nombre" {
  description = "Nombre del bucket de secretos (sin ARN, para evitar ciclos con el módulo de bucket)."
  type        = string
}

variable "prefijo_artefactos" {
  description = "Prefijo de objetos que el publicador de artefactos puede escribir"
  type        = string
  default     = "releases/"

  validation {
    condition     = endswith(var.prefijo_artefactos, "/")
    error_message = "El prefijo de artefactos debe terminar en '/'."
  }
}

variable "prefijo_secretos" {
  description = "Prefijo de objetos del bundle de secretos"
  type        = string
  default     = "nexum/secrets/"

  validation {
    condition     = endswith(var.prefijo_secretos, "/")
    error_message = "El prefijo de secretos debe terminar en '/'."
  }
}

variable "rol_runtime_arn" {
  description = "ARN del rol runtime de las instancias; se exige que sea distinto de las tres identidades nuevas."
  type        = string

  validation {
    condition     = can(regex("^arn:aws:iam::[0-9]{12}:role/.+$", var.rol_runtime_arn))
    error_message = "rol_runtime_arn debe ser un ARN de rol válido."
  }
}
