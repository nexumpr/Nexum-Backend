# Módulo `egreso_fcm` — egreso HTTPS controlado para FCM/OAuth de Google.
#
# La opción recomendada y única del módulo es un NAT Gateway administrado en
# UNA sola zona de disponibilidad. NO crea recursos por defecto: `habilitar =
# false` deja todos los `count` en cero. En `envs/h08i` el bloque del módulo
# también nace con `count = 0`; activarlo es un cambio de variable, no una
# reescritura.
#
# La subred pública es NUEVA y dedicada al NAT: el entorno actual (I-010) no
# tiene subredes públicas ni rutas hacia el IGW. El IGW ya existe porque
# CloudFront VPC Origins lo exige, pero no tenía ruta por defecto.

variable "habilitar" {
  description = "false (por defecto): el módulo no crea NAT, EIP, subred pública, rutas ni regla de SG."
  type        = bool
  default     = false
}

variable "proyecto" {
  description = "Prefijo lógico"
  type        = string
  default     = "nexum"
}

variable "entorno" {
  description = "Entorno lógico"
  type        = string
  default     = "h08i"
}

variable "region" {
  description = "Región AWS; se usa para validar que la AZ pertenece a la región"
  type        = string
  default     = "us-east-2"
}

variable "vpc_id" {
  description = "VPC existente de I-010/I-030. Obligatoria si habilitar=true."
  type        = string
  default     = ""

  validation {
    condition     = var.vpc_id == "" || can(regex("^vpc-[0-9a-f]+$", var.vpc_id))
    error_message = "vpc_id debe ser una VPC existente o vacío cuando habilitar=false."
  }
}

variable "internet_gateway_id" {
  description = "IGW ya adjunto a la VPC (I-010, exigido por VPC Origins). Obligatorio si habilitar=true."
  type        = string
  default     = ""

  validation {
    condition     = var.internet_gateway_id == "" || can(regex("^igw-[0-9a-f]+$", var.internet_gateway_id))
    error_message = "internet_gateway_id debe ser un IGW existente o vacío."
  }
}

variable "zona" {
  description = "AZ única del NAT Gateway y de su subred pública dedicada."
  type        = string
  default     = "us-east-2a"

  validation {
    condition     = can(regex("^[a-z]{2}-[a-z]+-[0-9][a-z]$", var.zona))
    error_message = "zona debe tener forma de zona de disponibilidad, p. ej. us-east-2a."
  }
}

variable "subred_publica_cidr" {
  description = <<-EOT
    CIDR de la subred pública NUEVA y dedicada al NAT. Debe ser un bloque IPv4
    válido, no solapado con las subredes privadas de I-010 y con al menos /27
    (el NAT necesita direcciones libres; AWS reserva 5). El valor por defecto
    (10.42.16.0/24) queda fuera de las subredes privadas 10.42.0.0/24 y
    10.42.1.0/24 derivadas por `modules/red`.
  EOT
  type        = string
  default     = "10.42.16.0/24"

  validation {
    condition     = can(cidrhost(var.subred_publica_cidr, 0))
    error_message = "subred_publica_cidr debe ser un bloque IPv4 válido."
  }
}

variable "private_route_table_ids" {
  description = "Tablas de rutas privadas del backend (I-010) que reciben 0.0.0.0/0 -> NAT."
  type        = list(string)
  default     = []

  validation {
    condition = alltrue([
      for id in var.private_route_table_ids : can(regex("^rtb-[0-9a-f]+$", id))
    ])
    error_message = "Cada route table debe ser un ID rtb-* existente."
  }
}

variable "backend_security_group_id" {
  description = "SG real del backend (I-010) al que se añade el egreso TCP 443."
  type        = string
  default     = ""

  validation {
    condition     = var.backend_security_group_id == "" || can(regex("^sg-[0-9a-f]+$", var.backend_security_group_id))
    error_message = "backend_security_group_id debe ser un SG existente o vacío."
  }
}

variable "cidr_egreso_https" {
  description = <<-EOT
    Destino IPv4 del egreso TCP 443 cuando no se dispone de una prefix list de
    Google. No existe una prefix list administrada por AWS para FCM/OAuth:
    0.0.0.0/0 es el único valor honesto si no se aporta `prefix_list_egreso_https_id`.
    Los rangos de Google cambian y el NAT no filtra por dominio.
  EOT
  type        = string
  default     = "0.0.0.0/0"

  validation {
    condition     = can(cidrhost(var.cidr_egreso_https, 0))
    error_message = "cidr_egreso_https debe ser un bloque IPv4 válido."
  }
}

variable "prefix_list_egreso_https_id" {
  description = <<-EOT
    Opcional. ID de una prefix list REAL (por ejemplo, una lista de rangos de
    Google creada por el operador). Vacío = se usa `cidr_egreso_https`. No se
    inventa ni se crea una prefix list en este módulo.
  EOT
  type        = string
  default     = ""

  validation {
    condition     = var.prefix_list_egreso_https_id == "" || can(regex("^pl-[0-9a-f]+$", var.prefix_list_egreso_https_id))
    error_message = "prefix_list_egreso_https_id debe estar vacío o ser un ID pl-* existente."
  }
}

variable "etiquetas" {
  description = "Etiquetas adicionales"
  type        = map(string)
  default     = {}
}
