variable "proyecto" {
  description = "Prefijo de nombres"
  type        = string
  default     = "nexum"

  validation {
    condition     = can(regex("^[a-z0-9-]{2,20}$", var.proyecto))
    error_message = "proyecto invalido."
  }
}

variable "entorno" {
  description = "Entorno logico"
  type        = string
  default     = "h08i"

  validation {
    condition     = can(regex("^[a-z0-9-]{2,12}$", var.entorno))
    error_message = "entorno invalido."
  }
}

variable "region" {
  description = "Region AWS"
  type        = string
  default     = "us-east-2"

  validation {
    condition     = var.region == "us-east-2"
    error_message = "I-000/I-010 fijan us-east-2."
  }
}

variable "instance_type" {
  description = "Tipo de instancia (ARM64). I-020 fija t4g.micro"
  type        = string
  default     = "t4g.micro"

  validation {
    condition     = var.instance_type == "t4g.micro"
    error_message = "Solo t4g.micro esta presupuestado (2 vCPU, 1 GiB, ARM64)."
  }
}

variable "asg_capacidad" {
  description = "Capacidad del ASG: min = desired = max"
  type        = number
  default     = 2

  validation {
    condition     = var.asg_capacidad >= 2 && var.asg_capacidad <= 2
    error_message = "Se exigen exactamente 2 instancias simultaneas en esta fase."
  }
}

variable "subnet_ids" {
  description = "Mapa AZ -> id de subred privada, proveniente de I-010"
  type        = map(string)

  validation {
    condition     = length(var.subnet_ids) == 2
    error_message = "I-010 debe aportar exactamente 2 subredes privadas."
  }
}

variable "sg_backend_id" {
  description = "Security group del backend de I-010 (ingress 8080 desde ALB)"
  type        = string

  validation {
    # Estricto: no se acepta cualquier cadena no vacia.
    condition     = can(regex("^sg-[0-9a-f]{8,17}$", var.sg_backend_id))
    error_message = "sg_backend_id debe tener el formato de un security group real (sg-...)."
  }
}

variable "ami_ssm_parameter" {
  description = "Parametro publico de SSM con la AMI ARM64 (Amazon Linux 2023)"
  type        = string
  default     = "/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-arm64"

  validation {
    condition     = startswith(var.ami_ssm_parameter, "/aws/service/ami-amazon-linux-latest/")
    error_message = "La AMI debe provenir del parametro publico oficial de SSM."
  }
}

variable "iam_instance_profile_name" {
  description = "Instance profile definido por I-060 (no se crea aqui)"
  type        = string
  default     = "PENDIENTE-I-060"
}

variable "root_volume_gb" {
  description = "Tamano del volumen raiz (presupuesto I-000: 15 GB por instancia)"
  type        = number
  default     = 15

  validation {
    condition     = var.root_volume_gb >= 8 && var.root_volume_gb <= 15
    error_message = "El presupuesto cubre 30 GB totales: 15 GB por instancia como maximo."
  }
}

variable "artefacto_s3_uri" {
  description = "URI S3 del binario publicado (s3://bucket/clave)"
  type        = string
  default     = "s3://PENDIENTE-I080/nexum-backend"

  validation {
    condition     = startswith(var.artefacto_s3_uri, "s3://")
    error_message = "El artefacto debe servirse desde S3 (gateway endpoint, sin NAT)."
  }
}

variable "artefacto_sha256" {
  description = "SHA-256 del binario, generado por scripts/build-backend.ps1"
  type        = string
  default     = ""
}

variable "servicio_nombre" {
  description = "Nombre del servicio systemd"
  type        = string
  default     = "nexum-backend"
}

variable "health_check_type" {
  description = "Tipo de health check del ASG (EC2 en I-020; ELB en I-030)"
  type        = string
  default     = "EC2"

  validation {
    condition     = contains(["EC2", "ELB"], var.health_check_type)
    error_message = "health_check_type debe ser EC2 o ELB."
  }
}

variable "health_check_grace_period" {
  description = "Segundos de gracia para el arranque"
  type        = number
  default     = 120

  validation {
    condition     = var.health_check_grace_period >= 60 && var.health_check_grace_period <= 300
    error_message = "grace period entre 60 y 300 s."
  }
}

variable "puerto_backend" {
  description = "Puerto HTTP interno del backend Go (ALB -> EC2 en I-030)"
  type        = number
  default     = 8080

  validation {
    condition     = var.puerto_backend >= 1024 && var.puerto_backend <= 65535
    error_message = "puerto_backend debe ser no privilegiado."
  }
}

variable "unidad_s3_uri" {
  description = "URI S3 de la unidad systemd publicada junto al binario"
  type        = string
  default     = "s3://PENDIENTE-I080/nexum-backend.service"

  validation {
    condition     = startswith(var.unidad_s3_uri, "s3://")
    error_message = "La unidad debe servirse desde S3 (gateway endpoint, sin NAT)."
  }
}

variable "unidad_sha256" {
  description = "SHA-256 del archivo de unidad systemd"
  type        = string
  default     = ""
}
