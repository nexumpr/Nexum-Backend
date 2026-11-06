variable "proyecto" {
  description = "Prefijo de nombres de recursos"
  type        = string
  default     = "nexum"
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
  description = "CIDR de la VPC (RFC1918, sin superposición conocida)"
  type        = string
  default     = "10.42.0.0/16"

  validation {
    condition     = can(cidrhost(var.cidr_vpc, 0)) && can(regex("^(10|172|192)\\.", var.cidr_vpc))
    error_message = "cidr_vpc debe ser un CIDR privado válido."
  }
}

variable "cidrs_subredes" {
  description = "Un CIDR privado por zona (dos zonas por requisito de ALB/AZ)"
  type        = list(string)
  default     = ["10.42.10.0/24", "10.42.11.0/24"]

  validation {
    condition     = length(var.cidrs_subredes) == 2 && alltrue([for c in var.cidrs_subredes : can(cidrhost(c, 0))])
    error_message = "Se exigen exactamente 2 CIDR de subred válidos."
  }
}

variable "zonas" {
  description = "Zonas de disponibilidad (2, distintas)"
  type        = list(string)
  default     = ["us-east-2a", "us-east-2b"]

  validation {
    condition     = length(var.zonas) == 2 && length(distinct(var.zonas)) == 2
    error_message = "Se exigen 2 zonas distintas."
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
