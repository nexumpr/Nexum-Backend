variable "proyecto" {
  description = "Prefijo de nombres de recursos"
  type        = string
  default     = "nexum"

  validation {
    condition     = can(regex("^[a-z0-9-]{2,20}$", var.proyecto))
    error_message = "proyecto debe ser minúsculas, dígitos o guiones."
  }
}

variable "entorno" {
  description = "Entorno lógico (h08a, h08i, ...)"
  type        = string
  default     = "h08i"

  validation {
    condition     = can(regex("^[a-z0-9-]{2,12}$", var.entorno))
    error_message = "entorno debe ser minúsculas, dígitos o guiones (2-12)."
  }
}

variable "region" {
  description = "Región AWS del Backend"
  type        = string
  default     = "us-east-2"

  validation {
    condition     = var.region == "us-east-2"
    error_message = "I-000/I-010 fijan us-east-2; otro valor exigiría recotización."
  }
}

variable "cidr_vpc" {
  description = "CIDR de la VPC: debe ser RFC1918 válido"
  type        = string
  default     = "10.42.0.0/16"

  validation {
    condition = (
      can(cidrnetmask(var.cidr_vpc)) &&
      (
        can(regex("^10\\.", var.cidr_vpc)) ||
        can(regex("^172\\.(1[6-9]|2[0-9]|3[01])\\.", var.cidr_vpc)) ||
        can(regex("^192\\.168\\.", var.cidr_vpc))
      ) &&
      tonumber(split("/", var.cidr_vpc)[1]) >= 16 &&
      tonumber(split("/", var.cidr_vpc)[1]) <= 24
    )
    error_message = "cidr_vpc debe ser un CIDR RFC1918 válido con prefijo entre /16 y /24."
  }
}

variable "bits_subredes" {
  description = "Bits añadidos al CIDR de la VPC para cada subred (derivadas, sin superposición)"
  type        = number
  default     = 8

  validation {
    condition = (
      var.bits_subredes >= 4 &&
      var.bits_subredes <= 12 &&
      (tonumber(split("/", var.cidr_vpc)[1]) + var.bits_subredes) <= 28
    )
    error_message = "bits_subredes debe dar subredes de al menos /16 y como máximo /28."
  }
}

variable "zonas" {
  description = "Zonas de disponibilidad (2, distintas, de la región)"
  type        = list(string)
  default     = ["us-east-2a", "us-east-2b"]

  validation {
    condition = (
      length(var.zonas) == 2 &&
      length(distinct(var.zonas)) == 2 &&
      alltrue([for az in var.zonas : startswith(az, var.region)])
    )
    error_message = "Se exigen 2 zonas distintas y pertenecientes a var.region."
  }
}

variable "puerto_backend" {
  description = "Puerto HTTP interno del backend Go (ALB -> EC2)"
  type        = number
  default     = 8080

  validation {
    condition     = var.puerto_backend >= 1024 && var.puerto_backend <= 65535
    error_message = "puerto_backend debe ser no privilegiado."
  }
}

variable "ventana_mantenimiento_activa" {
  description = "Si es true, crea los interface endpoints de Secrets Manager, SSM y SSM Messages"
  type        = bool
  default     = false
}

variable "etiquetas" {
  description = "Etiquetas adicionales"
  type        = map(string)
  default     = {}
}
