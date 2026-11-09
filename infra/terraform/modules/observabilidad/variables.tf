variable "proyecto" {
  description = "Prefijo de nombres de recursos"
  type        = string
  default     = "nexum"
}

variable "entorno" {
  description = "Entorno lógico (h08i, dev, prod...)"
  type        = string
}

variable "region" {
  description = "Región AWS, usada en el dashboard"
  type        = string
  default     = "us-east-2"
}

variable "alb_arn" {
  description = "ARN del ALB interno (I-030) para la dimensión LoadBalancer"
  type        = string
}

variable "target_group_arn" {
  description = "ARN del target group (I-030) para la dimensión TargetGroup"
  type        = string
}

variable "retencion_dias" {
  description = "Retención finita de CloudWatch Logs; nunca infinita"
  type        = number
  default     = 7
  validation {
    condition = contains(
      [1, 3, 5, 7, 14, 30, 60, 90, 120, 150, 180, 365, 400, 545, 731, 1096, 1827, 2192, 2557, 2922, 3288, 3653],
      var.retencion_dias
    )
    error_message = "retencion_dias debe ser un valor admitido por CloudWatch Logs (1,3,5,7,14,30,...)."
  }
}

variable "umbral_5xx" {
  description = "Suma de errores 5xx en 5 minutos que dispara la alarma"
  type        = number
  default     = 5
  validation {
    condition     = var.umbral_5xx >= 1
    error_message = "umbral_5xx debe ser >= 1."
  }
}

variable "umbral_auth_401" {
  description = "Suma de 401 en 5 minutos que dispara la alarma de autenticación anómala"
  type        = number
  default     = 20
  validation {
    condition     = var.umbral_auth_401 >= 1
    error_message = "umbral_auth_401 debe ser >= 1."
  }
}

variable "presupuesto_usd" {
  description = "Límite de presupuesto mensual declarativo (alarma, no límite de gasto)"
  type        = number
  default     = 1
  validation {
    condition     = var.presupuesto_usd > 0
    error_message = "presupuesto_usd debe ser > 0."
  }
}

variable "habilitar_dashboard" {
  description = "Crea el dashboard de observabilidad (CloudWatch permite 3 gratuitos)"
  type        = bool
  default     = true
}

variable "etiquetas" {
  description = "Etiquetas comunes"
  type        = map(string)
  default     = {}
}
