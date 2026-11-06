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
  description = "CIDR RFC1918 de la VPC"
  type        = string
  default     = "10.42.0.0/16"
}

variable "bits_subredes" {
  description = "Bits para derivar las subredes privadas del CIDR de la VPC"
  type        = number
  default     = 8
}

variable "zonas" {
  description = "Zonas de disponibilidad de la región"
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
