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

variable "recursos" {
  description = <<-EOT
    Inventario declarativo de recursos operables. Es la unica entrada de
    clasificacion: `criticality=core` es intocable 24x7 y `criticality=auxiliary`
    solo puede suspenderse si figura en `allowlist_auxiliar`.
  EOT
  type = map(object({
    tipo                = string
    componente          = string
    criticidad          = string
    etiqueta_criticidad = string
    suspendible         = bool
    estado_recurso      = string
  }))

  validation {
    condition     = length(var.recursos) > 0
    error_message = "El inventario no puede estar vacio: una lista vacia haria parecer que no hay core que proteger."
  }
}

variable "allowlist_auxiliar" {
  description = <<-EOT
    Allowlist revisada de recursos auxiliares suspendibles. Se declara de forma
    explicita y separada del inventario: anadir un auxiliar nuevo obliga a
    revisar este archivo, no basta con ampliar la fuente de datos.
  EOT
  type        = list(string)
  default     = []

  validation {
    condition     = alltrue([for nombre in var.allowlist_auxiliar : can(regex("^[a-z0-9-]{2,64}$", nombre))])
    error_message = "La allowlist solo admite identificadores en minusculas, numeros y guiones."
  }
}

variable "max_denegados" {
  description = "Maximo de recursos denegados tolerado. Fase I-120 exige 0 (deny por defecto)."
  type        = number
  default     = 0

  validation {
    condition     = var.max_denegados == 0
    error_message = "I-120 no admite recursos denegados: desconocido o sin etiqueta implica denegacion."
  }
}

variable "destroy_datos" {
  description = "Separacion explicita de destroys. Debe ser false siempre: no existe destroy-data en I-120."
  type        = bool
  default     = false

  validation {
    condition     = var.destroy_datos == false
    error_message = "destroy-data esta prohibido: DynamoDB nunca se destruye por rutina de ahorro (spec I-120 seccion 9)."
  }
}

variable "destroy_estado" {
  description = "Separacion explicita de destroys. Debe ser false siempre: perder el state impide reconciliar."
  type        = bool
  default     = false

  validation {
    condition     = var.destroy_estado == false
    error_message = "destroy-state esta prohibido: el state Terraform tiene prevent_destroy y es irrecuperable sin el (I-070)."
  }
}

variable "cambios_propuestos" {
  description = <<-EOT
    Cambios del plan a verificar (direccion + accion). Vacio significa "sin plan
    evaluado": no se asume alcance. Cualquier cambio sobre un recurso core o sobre
    capacidad por debajo de 2 se rechaza (proteccion contra destruccion indirecta).
  EOT
  type = list(object({
    direccion = string
    accion    = string
    desired   = optional(number)
  }))
  default = []

  validation {
    condition     = alltrue([for c in var.cambios_propuestos : contains(["create", "update", "read", "no-op"], lower(c.accion))])
    error_message = "Solo se admiten acciones create/update/read/no-op: delete y destroy no existen para el core ni para auxiliares en I-120."
  }

  validation {
    condition     = alltrue([for c in var.cambios_propuestos : c.desired == null || c.desired >= 2])
    error_message = "desired por debajo de 2 rompe la doble instancia (GOV-02)."
  }
}
