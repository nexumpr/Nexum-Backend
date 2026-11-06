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
  description = "CIDR de la VPC: RFC1918 válido, prefijo /16 a /24"
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
  description = <<-EOT
    Bits añadidos al CIDR de la VPC para cada subred. Se derivan dos subredes con
    índices 0 y 1, por lo que siempre están contenidas y nunca se solapan.

    Límites: el prefijo final debe quedar entre /20 (capacidad holgada) y /27
    (mínimo que exige el ALB, que necesita 8 direcciones libres; AWS reserva 5
    direcciones por subred). Con la VPC por defecto /16, bits 4..11.
    EOT
  type        = number
  default     = 8

  validation {
    condition     = var.bits_subredes >= 4 && var.bits_subredes <= 11
    error_message = "bits_subredes debe estar entre 4 y 11 (con /16 produce de /20 a /27)."
  }
}

variable "zonas" {
  description = "Zonas de disponibilidad (2, distintas)"
  type        = list(string)
  default     = ["us-east-2a", "us-east-2b"]

  validation {
    condition     = length(var.zonas) == 2 && length(distinct(var.zonas)) == 2
    error_message = "Se exigen exactamente 2 zonas distintas."
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
