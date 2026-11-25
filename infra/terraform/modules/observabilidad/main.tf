terraform {
  required_version = ">= 1.4.0, < 2.0.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.72"
    }
  }
}

locals {
  # Dimensiones de ALB/TG derivadas del ARN (sin modificar I-030).
  # LoadBalancer espera `app/<nombre>/<id>`; TargetGroup espera `targetgroup/<nombre>/<id>`.
  alb_suffix = replace(var.alb_arn, "/^arn:aws:elasticloadbalancing:[^:]+:[0-9]+:loadbalancer//", "")
  tg_suffix  = replace(var.target_group_arn, "/^arn:aws:elasticloadbalancing:[^:]+:[0-9]+:/", "")
  namespace  = "Nexum/Observabilidad"
  etiquetas = merge({
    Proyecto = var.proyecto
    Entorno  = var.entorno
    Spec     = "I-110-observabilidad-costos"
  }, var.etiquetas)
}

resource "terraform_data" "precondiciones" {
  input = "${var.alb_arn}|${var.target_group_arn}|${var.retencion_dias}|${var.presupuesto_usd}"

  lifecycle {
    precondition {
      condition     = startswith(var.alb_arn, "arn:aws:elasticloadbalancing:")
      error_message = "alb_arn debe ser un ARN de Elastic Load Balancing."
    }
    precondition {
      condition     = startswith(var.target_group_arn, "arn:aws:elasticloadbalancing:")
      error_message = "target_group_arn debe ser un ARN de Elastic Load Balancing."
    }
    precondition {
      condition     = var.retencion_dias <= 30
      error_message = "La retención local no debe superar 30 días sin revisión FinOps."
    }
    precondition {
      condition     = !var.habilitar_alarmas_fcm || var.habilitar_metricas_fcm
      error_message = "Las alarmas FCM exigen habilitar_metricas_fcm=true; de lo contrario apuntarían a métricas inexistentes."
    }
  }
}

resource "aws_cloudwatch_log_group" "backend" {
  name              = "/${var.proyecto}/${var.entorno}/backend"
  retention_in_days = var.retencion_dias
  tags              = local.etiquetas
}

resource "aws_cloudwatch_log_metric_filter" "errores_5xx" {
  name           = "${var.proyecto}-${var.entorno}-errores-5xx"
  log_group_name = aws_cloudwatch_log_group.backend.name
  pattern        = "{ $.status >= 500 }"

  metric_transformation {
    name          = "Errores5xx"
    namespace     = local.namespace
    value         = "1"
    default_value = "0"
  }
}

resource "aws_cloudwatch_log_metric_filter" "auth_401" {
  name           = "${var.proyecto}-${var.entorno}-auth-401"
  log_group_name = aws_cloudwatch_log_group.backend.name
  pattern        = "{ $.status = 401 }"

  metric_transformation {
    name          = "Auth401"
    namespace     = local.namespace
    value         = "1"
    default_value = "0"
  }
}

resource "aws_cloudwatch_log_metric_filter" "readiness_no_200" {
  name           = "${var.proyecto}-${var.entorno}-readiness-no-200"
  log_group_name = aws_cloudwatch_log_group.backend.name
  pattern        = "{ $.route = \"/api/v1/health/ready\" && $.status != 200 }"

  metric_transformation {
    name          = "ReadinessNo200"
    namespace     = local.namespace
    value         = "1"
    default_value = "0"
  }
}

resource "aws_cloudwatch_metric_alarm" "targets_sanos" {
  alarm_name        = "${var.proyecto}-${var.entorno}-targets-sanos-bajos"
  alarm_description = "Menos de 2 targets sanos en el ALB (I-030). Runbook: MATRIZ_METRICAS_ALERTAS.md#A1"
  namespace         = "AWS/ApplicationELB"
  metric_name       = "HealthyHostCount"
  dimensions = {
    LoadBalancer = local.alb_suffix
    TargetGroup  = local.tg_suffix
  }
  statistic           = "Minimum"
  period              = 60
  evaluation_periods  = 2
  threshold           = 2
  comparison_operator = "LessThanThreshold"
  treat_missing_data  = "breaching"
  tags                = local.etiquetas
}

resource "aws_cloudwatch_log_metric_filter" "rate_limit_429" {
  name           = "${var.proyecto}-${var.entorno}-rate-limit-429"
  log_group_name = aws_cloudwatch_log_group.backend.name
  pattern        = "{ $.status = 429 }"

  metric_transformation {
    name          = "RateLimit429"
    namespace     = local.namespace
    value         = "1"
    default_value = "0"
  }
}

resource "aws_cloudwatch_metric_alarm" "errores_5xx" {
  alarm_name          = "${var.proyecto}-${var.entorno}-errores-5xx"
  alarm_description   = "Errores 5xx por encima del umbral. Runbook: MATRIZ_METRICAS_ALERTAS.md#A2"
  namespace           = local.namespace
  metric_name         = "Errores5xx"
  statistic           = "Sum"
  period              = 300
  evaluation_periods  = 1
  threshold           = var.umbral_5xx
  comparison_operator = "GreaterThanOrEqualToThreshold"
  treat_missing_data  = "notBreaching"
  tags                = local.etiquetas
}

resource "aws_cloudwatch_metric_alarm" "auth_401" {
  alarm_name          = "${var.proyecto}-${var.entorno}-auth-401"
  alarm_description   = "Pico anómalo de 401. Runbook: MATRIZ_METRICAS_ALERTAS.md#A3"
  namespace           = local.namespace
  metric_name         = "Auth401"
  statistic           = "Sum"
  period              = 300
  evaluation_periods  = 1
  threshold           = var.umbral_auth_401
  comparison_operator = "GreaterThanOrEqualToThreshold"
  treat_missing_data  = "notBreaching"
  tags                = local.etiquetas
}

resource "aws_cloudwatch_metric_alarm" "readiness_no_200" {
  alarm_name          = "${var.proyecto}-${var.entorno}-readiness-no-200"
  alarm_description   = "Readiness distinto de 200. Runbook: MATRIZ_METRICAS_ALERTAS.md#A4"
  namespace           = local.namespace
  metric_name         = "ReadinessNo200"
  statistic           = "Sum"
  period              = 300
  evaluation_periods  = 1
  threshold           = 1
  comparison_operator = "GreaterThanOrEqualToThreshold"
  treat_missing_data  = "notBreaching"
  tags                = local.etiquetas
}

resource "aws_cloudwatch_metric_alarm" "rate_limit_429" {
  alarm_name          = "${var.proyecto}-${var.entorno}-rate-limit-429"
  alarm_description   = "Rate limiting 429 por encima del umbral. Runbook: MATRIZ_METRICAS_ALERTAS.md#A6"
  namespace           = local.namespace
  metric_name         = "RateLimit429"
  statistic           = "Sum"
  period              = 300
  evaluation_periods  = 1
  threshold           = var.umbral_429
  comparison_operator = "GreaterThanOrEqualToThreshold"
  treat_missing_data  = "notBreaching"
  tags                = local.etiquetas
}

resource "aws_cloudwatch_metric_alarm" "redis_conexiones" {
  count             = var.redis_replication_group_id == "" ? 0 : 1
  alarm_name        = "${var.proyecto}-${var.entorno}-redis-sin-conexiones"
  alarm_description = "Redis sin conexiones activas. Runbook: MATRIZ_METRICAS_ALERTAS.md#A7"
  namespace         = "AWS/ElastiCache"
  metric_name       = "CurrConnections"
  dimensions = {
    ReplicationGroupId = var.redis_replication_group_id
  }
  statistic           = "Maximum"
  period              = 300
  evaluation_periods  = 2
  threshold           = 1
  comparison_operator = "LessThanThreshold"
  treat_missing_data  = "breaching"
  tags                = local.etiquetas
}

resource "aws_cloudwatch_metric_alarm" "dynamodb_throttles" {
  count             = var.dynamodb_table_name == "" ? 0 : 1
  alarm_name        = "${var.proyecto}-${var.entorno}-dynamodb-throttles"
  alarm_description = "Throttling persistente en DynamoDB. Runbook: MATRIZ_METRICAS_ALERTAS.md#A8"
  namespace         = "AWS/DynamoDB"
  metric_name       = "ThrottledRequests"
  dimensions = {
    TableName = var.dynamodb_table_name
  }
  statistic           = "Sum"
  period              = 300
  evaluation_periods  = 2
  threshold           = var.umbral_throttles
  comparison_operator = "GreaterThanOrEqualToThreshold"
  treat_missing_data  = "notBreaching"
  tags                = local.etiquetas
}

resource "aws_cloudwatch_metric_alarm" "logs_sin_eventos" {
  alarm_name        = "${var.proyecto}-${var.entorno}-logs-sin-eventos"
  alarm_description = "No llegan logs al grupo en 1 hora (agente o métricas rotas). Runbook: MATRIZ_METRICAS_ALERTAS.md#A5"
  namespace         = "AWS/Logs"
  metric_name       = "IncomingLogEvents"
  dimensions = {
    LogGroupName = aws_cloudwatch_log_group.backend.name
  }
  statistic           = "Sum"
  period              = 3600
  evaluation_periods  = 1
  threshold           = 1
  comparison_operator = "LessThanThreshold"
  treat_missing_data  = "breaching"
  tags                = local.etiquetas
}

# --- Observabilidad FCM/scheduler (count=0 por defecto) ---------------------
#
# Contrato de eventos (sin PII ni tokens) documentado en
# `sdd/_control/H08/OBSERVABILIDAD_FCM_I110.md`:
#   event=startup          build_version
#   event=worker_heartbeat worker=fcm_outbox|scheduler, lease_held, processed, failed
#   event=fcm_send         status=ok|error
#   event=aws_error        error_code=AccessDenied
#   event=scheduler_tick   schedule_lag_seconds
#
# No se crea ninguna métrica ni alarma mientras las variables estén en false.

resource "aws_cloudwatch_log_metric_filter" "arranques_proceso" {
  count = var.habilitar_metricas_fcm ? 1 : 0

  name           = "${var.proyecto}-${var.entorno}-arranques-proceso"
  log_group_name = aws_cloudwatch_log_group.backend.name
  pattern        = "{ $.event = \"startup\" }"

  metric_transformation {
    name          = "ArranquesProceso"
    namespace     = local.namespace
    value         = "1"
    default_value = "0"
  }
}

resource "aws_cloudwatch_log_metric_filter" "arranque_version_esperada" {
  count = var.habilitar_metricas_fcm && var.version_esperada != "" ? 1 : 0

  name           = "${var.proyecto}-${var.entorno}-arranque-version-esperada"
  log_group_name = aws_cloudwatch_log_group.backend.name
  pattern        = "{ $.event = \"startup\" && $.build_version = \"${var.version_esperada}\" }"

  metric_transformation {
    name          = "ArranqueVersionEsperada"
    namespace     = local.namespace
    value         = "1"
    default_value = "0"
  }
}

resource "aws_cloudwatch_log_metric_filter" "arranque_version_inesperada" {
  count = var.habilitar_metricas_fcm && var.version_esperada != "" ? 1 : 0

  name           = "${var.proyecto}-${var.entorno}-arranque-version-inesperada"
  log_group_name = aws_cloudwatch_log_group.backend.name
  pattern        = "{ $.event = \"startup\" && $.build_version != \"${var.version_esperada}\" }"

  metric_transformation {
    name          = "ArranqueVersionInesperada"
    namespace     = local.namespace
    value         = "1"
    default_value = "0"
  }
}

resource "aws_cloudwatch_log_metric_filter" "fcm_worker_heartbeat" {
  count = var.habilitar_metricas_fcm ? 1 : 0

  name           = "${var.proyecto}-${var.entorno}-fcm-worker-heartbeat"
  log_group_name = aws_cloudwatch_log_group.backend.name
  pattern        = "{ $.event = \"worker_heartbeat\" && $.worker = \"${var.worker_fcm}\" }"

  metric_transformation {
    name          = "FcmWorkerHeartbeat"
    namespace     = local.namespace
    value         = "1"
    default_value = "0"
  }
}

resource "aws_cloudwatch_log_metric_filter" "scheduler_heartbeat" {
  count = var.habilitar_metricas_fcm ? 1 : 0

  name           = "${var.proyecto}-${var.entorno}-scheduler-heartbeat"
  log_group_name = aws_cloudwatch_log_group.backend.name
  pattern        = "{ $.event = \"worker_heartbeat\" && $.worker = \"${var.worker_scheduler}\" }"

  metric_transformation {
    name          = "SchedulerHeartbeat"
    namespace     = local.namespace
    value         = "1"
    default_value = "0"
  }
}

resource "aws_cloudwatch_log_metric_filter" "fcm_errores_envio" {
  count = var.habilitar_metricas_fcm ? 1 : 0

  name           = "${var.proyecto}-${var.entorno}-fcm-errores-envio"
  log_group_name = aws_cloudwatch_log_group.backend.name
  pattern        = "{ $.event = \"fcm_send\" && $.status = \"error\" }"

  metric_transformation {
    name          = "FcmErroresEnvio"
    namespace     = local.namespace
    value         = "1"
    default_value = "0"
  }
}

resource "aws_cloudwatch_log_metric_filter" "access_denied" {
  count = var.habilitar_metricas_fcm ? 1 : 0

  name           = "${var.proyecto}-${var.entorno}-access-denied"
  log_group_name = aws_cloudwatch_log_group.backend.name
  pattern        = "{ $.event = \"aws_error\" && $.error_code = \"AccessDenied\" }"

  metric_transformation {
    name          = "AccessDenied"
    namespace     = local.namespace
    value         = "1"
    default_value = "0"
  }
}

resource "aws_cloudwatch_log_metric_filter" "scheduler_lag" {
  count = var.habilitar_metricas_fcm ? 1 : 0

  name           = "${var.proyecto}-${var.entorno}-scheduler-lag"
  log_group_name = aws_cloudwatch_log_group.backend.name
  pattern        = "{ $.event = \"scheduler_tick\" }"

  metric_transformation {
    name          = "SchedulerLagSegundos"
    namespace     = local.namespace
    value         = "$.schedule_lag_seconds"
    default_value = "0"
  }
}

resource "aws_cloudwatch_metric_alarm" "fcm_worker_sin_latido" {
  count = var.habilitar_alarmas_fcm ? 1 : 0

  alarm_name          = "${var.proyecto}-${var.entorno}-fcm-worker-sin-latido"
  alarm_description   = "Sin heartbeat del worker FCM en el periodo. Runbook: OBSERVABILIDAD_FCM_I110.md#A-FCM-1"
  namespace           = local.namespace
  metric_name         = "FcmWorkerHeartbeat"
  statistic           = "Sum"
  period              = var.minutos_sin_heartbeat * 60
  evaluation_periods  = 1
  threshold           = 1
  comparison_operator = "LessThanThreshold"
  treat_missing_data  = "breaching"
  tags                = local.etiquetas
}

resource "aws_cloudwatch_metric_alarm" "scheduler_sin_latido" {
  count = var.habilitar_alarmas_fcm ? 1 : 0

  alarm_name          = "${var.proyecto}-${var.entorno}-scheduler-sin-latido"
  alarm_description   = "Sin heartbeat del scheduler de recordatorios en el periodo. Runbook: OBSERVABILIDAD_FCM_I110.md#A-FCM-2"
  namespace           = local.namespace
  metric_name         = "SchedulerHeartbeat"
  statistic           = "Sum"
  period              = var.minutos_sin_heartbeat * 60
  evaluation_periods  = 1
  threshold           = 1
  comparison_operator = "LessThanThreshold"
  treat_missing_data  = "breaching"
  tags                = local.etiquetas
}

resource "aws_cloudwatch_metric_alarm" "fcm_errores_envio" {
  count = var.habilitar_alarmas_fcm ? 1 : 0

  alarm_name          = "${var.proyecto}-${var.entorno}-fcm-errores-envio"
  alarm_description   = "Errores de envío FCM por encima del umbral. Runbook: OBSERVABILIDAD_FCM_I110.md#A-FCM-3"
  namespace           = local.namespace
  metric_name         = "FcmErroresEnvio"
  statistic           = "Sum"
  period              = 300
  evaluation_periods  = 1
  threshold           = var.umbral_fcm_errores
  comparison_operator = "GreaterThanOrEqualToThreshold"
  treat_missing_data  = "notBreaching"
  tags                = local.etiquetas
}

resource "aws_cloudwatch_metric_alarm" "access_denied" {
  count = var.habilitar_alarmas_fcm ? 1 : 0

  alarm_name          = "${var.proyecto}-${var.entorno}-access-denied"
  alarm_description   = "AccessDenied de AWS detectado en logs. Runbook: OBSERVABILIDAD_FCM_I110.md#A-FCM-4"
  namespace           = local.namespace
  metric_name         = "AccessDenied"
  statistic           = "Sum"
  period              = 300
  evaluation_periods  = 1
  threshold           = var.umbral_access_denied
  comparison_operator = "GreaterThanOrEqualToThreshold"
  treat_missing_data  = "notBreaching"
  tags                = local.etiquetas
}

resource "aws_cloudwatch_metric_alarm" "scheduler_lag" {
  count = var.habilitar_alarmas_fcm ? 1 : 0

  alarm_name          = "${var.proyecto}-${var.entorno}-scheduler-lag"
  alarm_description   = "Lag del scheduler por encima del umbral durante 3 periodos. Runbook: OBSERVABILIDAD_FCM_I110.md#A-FCM-5"
  namespace           = local.namespace
  metric_name         = "SchedulerLagSegundos"
  statistic           = "Maximum"
  period              = 300
  evaluation_periods  = 3
  threshold           = var.umbral_lag_scheduler_segundos
  comparison_operator = "GreaterThanThreshold"
  treat_missing_data  = "notBreaching"
  tags                = local.etiquetas
}

resource "aws_cloudwatch_metric_alarm" "arranque_version_inesperada" {
  count = var.habilitar_alarmas_fcm && var.version_esperada != "" ? 1 : 0

  alarm_name          = "${var.proyecto}-${var.entorno}-arranque-version-inesperada"
  alarm_description   = "El proceso arrancó con una versión distinta de la esperada. Runbook: OBSERVABILIDAD_FCM_I110.md#A-FCM-6"
  namespace           = local.namespace
  metric_name         = "ArranqueVersionInesperada"
  statistic           = "Sum"
  period              = 300
  evaluation_periods  = 1
  threshold           = 1
  comparison_operator = "GreaterThanOrEqualToThreshold"
  treat_missing_data  = "notBreaching"
  tags                = local.etiquetas
}

resource "aws_budgets_budget" "mensual" {
  name         = "${var.proyecto}-${var.entorno}-presupuesto"
  budget_type  = "COST"
  limit_amount = tostring(var.presupuesto_usd)
  limit_unit   = "USD"
  time_unit    = "MONTHLY"

  # AWS Budgets admite email directo sin SNS. S?lo se declara si hay destinatario
  # externo configurado; no se versionan direcciones reales.
  dynamic "notification" {
    for_each = length(var.emails_presupuesto) > 0 ? [85, 100] : []
    content {
      comparison_operator        = "GREATER_THAN"
      threshold                  = notification.value
      threshold_type             = "PERCENTAGE"
      notification_type          = notification.value == 100 ? "FORECASTED" : "ACTUAL"
      subscriber_email_addresses = var.emails_presupuesto
    }
  }
}

resource "aws_cloudwatch_dashboard" "observabilidad" {
  count          = var.habilitar_dashboard ? 1 : 0
  dashboard_name = "${var.proyecto}-${var.entorno}-observabilidad"

  dashboard_body = jsonencode({
    widgets = [
      {
        type   = "metric"
        x      = 0
        y      = 0
        width  = 12
        height = 6
        properties = {
          title  = "Targets sanos (ALB)"
          region = var.region
          view   = "timeSeries"
          metrics = [
            ["AWS/ApplicationELB", "HealthyHostCount", "LoadBalancer", local.alb_suffix, "TargetGroup", local.tg_suffix],
            ["AWS/ApplicationELB", "UnHealthyHostCount", "LoadBalancer", local.alb_suffix, "TargetGroup", local.tg_suffix],
          ]
          annotations = {
            horizontal = [{ label = "mínimo 2", value = 2 }]
          }
        }
      },
      {
        type   = "metric"
        x      = 12
        y      = 0
        width  = 12
        height = 6
        properties = {
          title  = "Errores y autenticación"
          region = var.region
          view   = "timeSeries"
          metrics = [
            [local.namespace, "Errores5xx"],
            [local.namespace, "Auth401"],
            [local.namespace, "ReadinessNo200"],
          ]
          stat = "Sum"
        }
      },
      {
        type   = "log"
        x      = 0
        y      = 6
        width  = 24
        height = 6
        properties = {
          title  = "Logs backend (sin PII)"
          region = var.region
          query  = "SOURCE '${aws_cloudwatch_log_group.backend.name}' | fields @timestamp, severity, route, status, latencyMs, traceId | sort @timestamp desc | limit 50"
        }
      },
    ]
  })
}
