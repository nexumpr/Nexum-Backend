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

# --- I-020: cómputo ---------------------------------------------------------

variable "iam_instance_profile_name" {
  description = "Instance profile real definido por I-060 (no se crea aquí)"
  type        = string
  default     = "PENDIENTE-I-060"
}

variable "root_volume_gb" {
  description = "Volumen raíz por instancia (presupuesto: 15 GB x2)"
  type        = number
  default     = 15
}

variable "artefacto_s3_uri" {
  description = "URI S3 del binario linux/arm64"
  type        = string
  default     = "s3://PENDIENTE-I080/nexum-backend"
}

variable "artefacto_sha256" {
  description = "SHA-256 del binario (lo emite scripts/build-backend.ps1)"
  type        = string
  default     = ""
}
