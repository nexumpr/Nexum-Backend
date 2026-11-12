variable "proyecto" {
  description = "Prefijo del nombre del bucket."
  type        = string
}

variable "entorno" {
  description = "Entorno (h08i, prod...)."
  type        = string
}

variable "etiquetas" {
  description = "Etiquetas comunes."
  type        = map(string)
  default     = {}
}

variable "prefijo" {
  description = "Prefijo S3 del bundle de secretos (debe terminar en '/')."
  type        = string
  default     = "nexum/secrets/"

  validation {
    condition     = endswith(var.prefijo, "/")
    error_message = "El prefijo debe terminar en '/'."
  }
}

variable "retencion_version_dias" {
  description = "Dias que se conservan las versiones no actuales del bundle (RFC-004)."
  type        = number
  default     = 30
}

variable "rol_runtime_arn" {
  description = "ARN del rol de instancia; unico principal autorizado sobre el prefijo."
  type        = string
}

variable "publicador_arn" {
  description = "ARN de la identidad IAM autorizada a publicar el bundle (RFC-004). Sin valor valido, el plan falla cerrado."
  type        = string
  default     = ""
}

variable "gestor_iac_arn" {
  description = "ARN del rol IAM que ejecuta Terraform; unico exento del Deny de s3:ListBucket para permitir HeadBucket en el refresh. No concede lectura de objetos."
  type        = string
  default     = ""
}

variable "gestor_objetos_arn" {
  description = "ARN del rol gestor autorizado a leer y actualizar los objetos del bundle. Es distinto de gestor_iac_arn, que cubre ListBucket/HeadBucket del operador que ejecuta Terraform."
  type        = string

  validation {
    condition     = can(regex("^arn:aws:iam::[0-9]{12}:role/.+$", var.gestor_objetos_arn))
    error_message = "gestor_objetos_arn debe ser el ARN de un rol IAM (arn:aws:iam::<cuenta>:role/<nombre>)."
  }
}
