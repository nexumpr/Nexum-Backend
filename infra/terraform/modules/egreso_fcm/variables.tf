# Módulo `egreso_fcm` — egreso HTTPS controlado para FCM/OAuth de Google.
#
# NO crea recursos por defecto. `habilitar = false` es el valor seguro y el
# módulo no se instancia en `envs/h08i` hasta que un Gate FinOps/Seguridad
# autorice el gasto y el cambio de conectividad. Ver
# `sdd/_control/H08/PROPUESTA_EGRESO_FCM.md`.

variable "habilitar" {
  description = "false (por defecto): el módulo no crea NAT, EIP, subred pública ni rutas."
  type        = bool
  default     = false
}

variable "proyecto" {
  description = "Prefijo lógico"
  type        = string
  default     = "nexum"
}

variable "entorno" {
  description = "Entorno lógico"
  type        = string
  default     = "h08i"
}

variable "vpc_id" {
  description = "VPC existente de I-010/I-030"
  type        = string
  default     = ""

  validation {
    condition     = var.vpc_id == "" || can(regex("^vpc-[0-9a-f]+$", var.vpc_id))
    error_message = "vpc_id debe ser una VPC existente o vacío cuando habilitar=false."
  }
}

variable "internet_gateway_id" {
  description = "IGW ya adjunto a la VPC (I-010). Obligatorio sólo si habilitar=true."
  type        = string
  default     = ""

  validation {
    condition     = var.internet_gateway_id == "" || can(regex("^igw-[0-9a-f]+$", var.internet_gateway_id))
    error_message = "internet_gateway_id debe ser un IGW existente o vacío."
  }
}

variable "private_route_table_ids" {
  description = "Tablas de rutas privadas que reciben la ruta por defecto al NAT."
  type        = list(string)
  default     = []

  validation {
    condition = alltrue([
      for id in var.private_route_table_ids : can(regex("^rtb-[0-9a-f]+$", id))
    ])
    error_message = "Cada route table debe ser un ID rtb-* existente."
  }
}

variable "availability_zones" {
  description = "Zonas para las subredes públicas del NAT; una por NAT."
  type        = list(string)
  default     = []
}

variable "public_subnet_cidrs" {
  description = "CIDR RFC1918 de las subredes públicas dedicadas al NAT; una por NAT."
  type        = list(string)
  default     = []

  validation {
    condition = alltrue([
      for cidr in var.public_subnet_cidrs : can(cidrhost(cidr, 0))
    ])
    error_message = "Cada CIDR debe ser un bloque IPv4 válido."
  }
}

variable "numero_nat_gateways" {
  description = "1 (mínimo) o 2 (HA por AZ). Por defecto 1; cada NAT tiene coste horario."
  type        = number
  default     = 1

  validation {
    condition     = contains([1, 2], var.numero_nat_gateways)
    error_message = "numero_nat_gateways sólo admite 1 o 2; no se crean NATs no auditados."
  }
}

variable "backend_security_group_id" {
  description = "SG del backend para la regla opcional de egreso 443; vacío = no se toca ningún SG."
  type        = string
  default     = ""

  validation {
    condition     = var.backend_security_group_id == "" || can(regex("^sg-[0-9a-f]+$", var.backend_security_group_id))
    error_message = "backend_security_group_id debe ser un SG existente o vacío."
  }
}

variable "habilitar_regla_egreso_backend" {
  description = <<-EOT
    false por defecto. Si true, añade egreso TCP 443 a 0.0.0.0/0 en el SG del
    backend. NAT no permite filtrar por dominio: esta regla es un riesgo
    residual que Seguridad debe aceptar o sustituir por un proxy con allowlist.
  EOT
  type        = bool
  default     = false
}

variable "etiquetas" {
  description = "Etiquetas adicionales"
  type        = map(string)
  default     = {}
}
