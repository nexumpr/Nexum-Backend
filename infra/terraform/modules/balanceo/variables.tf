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
  description = "Región AWS; us-east-2 está soportada por CloudFront VPC Origins"
  type        = string
  default     = "us-east-2"
}

variable "vpc_id" {
  description = "VPC de I-010"
  type        = string

  validation {
    condition     = can(regex("^vpc-[0-9a-f]{8,17}$", var.vpc_id))
    error_message = "vpc_id debe tener formato de VPC real (vpc-...)."
  }
}

variable "subnet_ids" {
  description = "Mapa AZ -> subred privada (exactamente 2 zonas distintas)"
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

variable "sg_alb_id" {
  description = "Security group del ALB, propiedad de I-010 (ingress 80 desde prefix list de CloudFront)"
  type        = string

  validation {
    condition     = can(regex("^sg-[0-9a-f]{8,17}$", var.sg_alb_id))
    error_message = "sg_alb_id debe tener formato de security group real (sg-...)."
  }
}

variable "sg_backend_id" {
  description = "Security group del backend (ingress 8080 desde el ALB)"
  type        = string

  validation {
    condition     = can(regex("^sg-[0-9a-f]{8,17}$", var.sg_backend_id))
    error_message = "sg_backend_id debe tener formato de security group real (sg-...)."
  }
}

variable "asg_name" {
  description = "Auto Scaling Group de I-020 (min=desired=max=2) al que se asocian los dos destinos"
  type        = string

  validation {
    condition     = can(regex("^[a-zA-Z0-9-]{1,255}$", var.asg_name)) && !startswith(var.asg_name, "PENDIENTE")
    error_message = "asg_name debe ser el nombre real del ASG de I-020, no un marcador."
  }
}

variable "puerto_backend" {
  description = "Puerto HTTP del proceso Go en las instancias"
  type        = number
  default     = 8080

  validation {
    condition     = var.puerto_backend >= 1024 && var.puerto_backend <= 65535
    error_message = "puerto_backend debe ser no privilegiado."
  }
}

variable "puerto_alb" {
  description = "Puerto HTTP del listener interno del ALB (D2: 80, sin 443)"
  type        = number
  default     = 80

  validation {
    condition     = var.puerto_alb == 80
    error_message = "D2 fija el listener interno en HTTP:80; 443 en el ALB exigiría ACM y otra ratificación."
  }
}

variable "health_check_path" {
  description = <<-EOT
    Ruta de liveness del target group. Debe ser la de A-090
    (`/api/v1/health/live`). `/api/v1/health/ready` devuelve 503 por diseño
    mientras no existan DynamoDB/Redis registrados (I-040/I-050) y marcaría
    unhealthy todo el par.
  EOT
  type        = string
  default     = "/api/v1/health/live"

  validation {
    condition     = startswith(var.health_check_path, "/api/v1/health/")
    error_message = "health_check_path debe ser una ruta normativa de A-090."
  }

  validation {
    condition     = !endswith(var.health_check_path, "/ready")
    error_message = "No se puede usar readiness como health check: 503 por diseño hasta I-040/I-050."
  }
}

variable "health_check_interval" {
  description = "Segundos entre comprobaciones de salud"
  type        = number
  default     = 30

  validation {
    condition     = var.health_check_interval >= 5 && var.health_check_interval <= 300
    error_message = "health_check_interval debe estar entre 5 y 300 segundos."
  }
}

variable "health_check_timeout" {
  description = "Segundos de espera por comprobación (menor que el intervalo)"
  type        = number
  default     = 5

  validation {
    condition     = var.health_check_timeout >= 2 && var.health_check_timeout <= 120
    error_message = "health_check_timeout debe estar entre 2 y 120 segundos."
  }

  validation {
    condition     = var.health_check_timeout < var.health_check_interval
    error_message = "health_check_timeout debe ser menor que health_check_interval."
  }
}

variable "healthy_threshold" {
  description = "Comprobaciones correctas consecutivas para marcar healthy"
  type        = number
  default     = 2

  validation {
    condition     = var.healthy_threshold >= 2 && var.healthy_threshold <= 10
    error_message = "healthy_threshold debe estar entre 2 y 10."
  }
}

variable "unhealthy_threshold" {
  description = "Comprobaciones fallidas consecutivas para retirar un destino"
  type        = number
  default     = 3

  validation {
    condition     = var.unhealthy_threshold >= 2 && var.unhealthy_threshold <= 10
    error_message = "unhealthy_threshold debe estar entre 2 y 10."
  }
}

variable "deregistration_delay" {
  description = "Segundos de drenado al retirar un destino (<= 60)"
  type        = number
  default     = 30

  validation {
    condition     = var.deregistration_delay >= 0 && var.deregistration_delay <= 60
    error_message = "deregistration_delay debe estar entre 0 y 60 segundos."
  }
}

variable "cloudfront_price_class" {
  description = "Price class de CloudFront (la más barata por defecto)"
  type        = string
  default     = "PriceClass_100"

  validation {
    condition     = contains(["PriceClass_100", "PriceClass_200", "PriceClass_All"], var.cloudfront_price_class)
    error_message = "cloudfront_price_class debe ser PriceClass_100, PriceClass_200 o PriceClass_All."
  }
}

variable "ipv6_habilitado" {
  description = "IPv6 para visitantes. Se deja en false: el VPC origin IPv4 no lo requiere y el SDD no lo exige"
  type        = bool
  default     = false
}

variable "etiquetas" {
  description = "Etiquetas adicionales"
  type        = map(string)
  default     = {}
}
