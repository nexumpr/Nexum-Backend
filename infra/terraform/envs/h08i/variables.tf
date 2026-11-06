variable "entorno" {
  description = "Entorno lógico"
  type        = string
  default     = "h08i"
}

variable "region" {
  description = "Región AWS"
  type        = string
  default     = "us-east-2"
}

variable "cidr_vpc" {
  description = "CIDR de la VPC"
  type        = string
  default     = "10.42.0.0/16"
}

variable "cidrs_subredes" {
  description = "CIDR por zona"
  type        = list(string)
  default     = ["10.42.10.0/24", "10.42.11.0/24"]
}

variable "zonas" {
  description = "Zonas de disponibilidad"
  type        = list(string)
  default     = ["us-east-2a", "us-east-2b"]
}

variable "puerto_backend" {
  description = "Puerto interno del backend Go"
  type        = number
  default     = 8080
}

variable "ventana_mantenimiento_activa" {
  description = "Activa los interface endpoints de mantenimiento (Secrets Manager, SSM)"
  type        = bool
  default     = false
}

variable "etiquetas" {
  description = "Etiquetas adicionales para default_tags"
  type        = map(string)
  default     = {}
}
