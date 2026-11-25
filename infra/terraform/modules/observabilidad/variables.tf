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

variable "redis_replication_group_id" {
  description = "ID del replication group de I-050 para la alarma de conexiones; vac?o = no crear alarma"
  type        = string
  default     = ""
}

variable "dynamodb_table_name" {
  description = "Nombre de la tabla de I-040 para la alarma de throttling; vac?o = no crear alarma"
  type        = string
  default     = ""
}

variable "umbral_429" {
  description = "Suma de 429 en 5 minutos que dispara la alarma de rate limiting"
  type        = number
  default     = 50
  validation {
    condition     = var.umbral_429 >= 1
    error_message = "umbral_429 debe ser >= 1."
  }
}

variable "umbral_throttles" {
  description = "ThrottledRequests de DynamoDB en 5 minutos que dispara la alarma"
  type        = number
  default     = 5
  validation {
    condition     = var.umbral_throttles >= 1
    error_message = "umbral_throttles debe ser >= 1."
  }
}

variable "emails_presupuesto" {
  description = "Destinatarios externos para AWS Budgets (sin SNS). Vac?o por defecto; nunca se versionan direcciones reales."
  type        = list(string)
  default     = []
  validation {
    condition     = alltrue([for email in var.emails_presupuesto : can(regex("^[^@\\s]+@[^@\\s]+\\.[^@\\s]+$", email))])
    error_message = "emails_presupuesto debe contener direcciones con formato b?sico."
  }
}

# --- Observabilidad FCM/scheduler: deshabilitada por defecto ----------------
#
# Los filtros y alarmas existen en el módulo, pero nacen con count=0. No se
# crea ninguna métrica ni alarma hasta que un Gate active estas variables y el
# proceso Go emita los eventos canónicos documentados en
# `sdd/_control/H08/OBSERVABILIDAD_FCM_I110.md`. El pipeline de logs a
# CloudWatch sigue `LOG_PIPELINE_AWS_VERIFIED = NOT_RUN`.

variable "habilitar_metricas_fcm" {
  description = "Crea los log metric filters de FCM/scheduler; false por defecto."
  type        = bool
  default     = false
}

variable "habilitar_alarmas_fcm" {
  description = "Crea las alarmas de FCM/scheduler; false por defecto. Exige habilitar_metricas_fcm=true."
  type        = bool
  default     = false
}

variable "version_esperada" {
  description = "Versión efectiva esperada (vX.Y.Z+sha12). Vacío = no se crea el filtro de versión inesperada."
  type        = string
  default     = ""

  validation {
    # 11 o 12 hex: la política fija `<sha12>` para releases nuevos, pero el
    # release desplegado `v0.20.0+4bf6758d372` se publicó con 11 caracteres.
    # Aceptar ambos permite fijar la versión real del filtro sin relajar la
    # forma SemVer+sha.
    condition     = var.version_esperada == "" || can(regex("^v[0-9]+\\.[0-9]+\\.[0-9]+\\+[0-9a-f]{11,12}$", var.version_esperada))
    error_message = "version_esperada debe estar vacío o tener la forma vX.Y.Z+<sha12>."
  }
}

variable "worker_fcm" {
  description = "Valor del campo worker para el heartbeat del outbox FCM."
  type        = string
  default     = "fcm_outbox"
}

variable "worker_scheduler" {
  description = "Valor del campo worker para el heartbeat del scheduler de recordatorios."
  type        = string
  default     = "scheduler"
}

variable "umbral_fcm_errores" {
  description = "Suma de errores de envío FCM en 5 minutos que dispara la alarma."
  type        = number
  default     = 5

  validation {
    condition     = var.umbral_fcm_errores >= 1
    error_message = "umbral_fcm_errores debe ser >= 1."
  }
}

variable "umbral_access_denied" {
  description = "Suma de AccessDenied en 5 minutos que dispara la alarma."
  type        = number
  default     = 1

  validation {
    condition     = var.umbral_access_denied >= 1
    error_message = "umbral_access_denied debe ser >= 1."
  }
}

variable "umbral_lag_scheduler_segundos" {
  description = "Lag máximo aceptable del scheduler; la alarma usa Maximum durante 3 periodos de 5 min."
  type        = number
  default     = 300

  validation {
    condition     = var.umbral_lag_scheduler_segundos >= 1
    error_message = "umbral_lag_scheduler_segundos debe ser >= 1."
  }
}

variable "minutos_sin_heartbeat" {
  description = "Periodo sin heartbeat que dispara la alarma de worker/scheduler parado."
  type        = number
  default     = 15

  validation {
    condition     = contains([5, 10, 15, 30, 60], var.minutos_sin_heartbeat)
    error_message = "minutos_sin_heartbeat debe ser 5, 10, 15, 30 o 60."
  }
}
