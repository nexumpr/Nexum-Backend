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
  description = "Entorno lógico (h08i)"
  type        = string
  default     = "h08i"

  validation {
    condition     = can(regex("^[a-z0-9-]{2,12}$", var.entorno))
    error_message = "entorno debe ser minúsculas, dígitos o guiones (2-12)."
  }
}

variable "region" {
  description = "Región AWS; debe coincidir con I-010 e I-000"
  type        = string
  default     = "us-east-2"
}

variable "subnet_ids" {
  description = "Mapa AZ -> subred privada de I-010 (2 zonas)"
  type        = map(string)

  validation {
    condition     = length(var.subnet_ids) == 2
    error_message = "Se exigen exactamente 2 subredes privadas."
  }

  validation {
    condition     = length(distinct(values(var.subnet_ids))) == 2
    error_message = "Las 2 subredes deben ser distintas."
  }
}

variable "sg_redis_id" {
  description = "Security group de Redis, propiedad de I-010 (ingress 6379 sólo desde el backend)"
  type        = string

  validation {
    condition     = can(regex("^sg-[0-9a-f]{8,17}$", var.sg_redis_id))
    error_message = "sg_redis_id debe tener formato de security group real (sg-...)."
  }
}

variable "sg_backend_id" {
  description = "Security group del backend; se compara para impedir reutilizar el mismo SG"
  type        = string

  validation {
    condition     = can(regex("^sg-[0-9a-f]{8,17}$", var.sg_backend_id))
    error_message = "sg_backend_id debe tener formato de security group real (sg-...)."
  }
}

variable "motor" {
  description = "Motor de caché; el SDD exige Redis OSS"
  type        = string
  default     = "redis"

  validation {
    condition     = var.motor == "redis"
    error_message = "Valkey u otro motor exige ADR humano explícito; I-050 implementa Redis OSS."
  }
}

variable "version_motor" {
  description = "Versión de Redis OSS verificada en I-000 (hasta 7.1, familia redis7)"
  type        = string
  default     = "7.1"

  validation {
    condition     = can(regex("^7\\.[0-9]$", var.version_motor))
    error_message = "I-000 verificó Redis OSS hasta 7.1; otra versión exige verificación previa."
  }
}

variable "tipo_nodo" {
  description = "Tipo de nodo; cache.t4g.micro es el único presupuestado en I-000"
  type        = string
  default     = "cache.t4g.micro"

  validation {
    condition     = var.tipo_nodo == "cache.t4g.micro"
    error_message = "I-000 presupuesta cache.t4g.micro; otro tipo exige recotización y Gate."
  }
}

variable "num_nodos" {
  description = "Número de nodos de caché; 1 en el presupuesto I-000"
  type        = number
  default     = 1

  validation {
    condition     = var.num_nodos == 1
    error_message = "Añadir réplicas cambia el presupuesto (17.2800 USD/45 d por nodo) y exige Gate."
  }
}

variable "habilitar_failover" {
  description = "Failover automático; sólo válido con más de un nodo (una caché no es HA)"
  type        = bool
  default     = false
}

variable "puerto" {
  description = "Puerto de Redis; I-010 ya permite 6379 desde el backend"
  type        = number
  default     = 6379

  validation {
    condition     = var.puerto == 6379
    error_message = "I-010 sólo autoriza 6379 entre backend y Redis."
  }
}

variable "cifrado_en_transito" {
  description = "TLS obligatorio (INF-050-03)"
  type        = bool
  default     = true
}

variable "cifrado_en_reposo" {
  description = "Cifrado en reposo obligatorio cuando el motor lo soporta"
  type        = bool
  default     = true
}

variable "password_aplicacion" {
  description = <<-EOT
    Contraseña del usuario RBAC de la aplicación. **Sensible**: no se versiona,
    no se escribe en `tfvars` del repositorio y en despliegue provendrá de un
    mecanismo aprobado (RFC-004, aún PROPUESTO). El valor real no se conoce en
    I-050 porque no hay despliegue.
  EOT
  type        = string
  sensitive   = true
  default     = ""

  validation {
    condition     = var.password_aplicacion == "" || length(var.password_aplicacion) >= 16
    error_message = "La contraseña RBAC debe tener al menos 16 caracteres (límite de ElastiCache)."
  }
}

variable "etiquetas" {
  description = "Etiquetas adicionales"
  type        = map(string)
  default     = {}
}
