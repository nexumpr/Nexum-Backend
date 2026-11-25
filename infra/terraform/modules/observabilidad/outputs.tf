output "log_group_backend" {
  description = "Grupo de logs del backend"
  value       = aws_cloudwatch_log_group.backend.name
}

output "retencion_dias" {
  description = "Retención efectiva declarada"
  value       = aws_cloudwatch_log_group.backend.retention_in_days
}

output "namespace_metricas" {
  description = "Namespace de las métricas derivadas por metric filter"
  value       = local.namespace
}

output "notificacion" {
  description = "Canal de notificaci?n de alarmas (bloqueado por coste de CMK SNS)"
  value       = "NOT_CONFIGURED_BLOCKED_RFC007_SNS_KMS"
}

output "alarmas" {
  description = "Alarmas declaradas y su significado"
  value = {
    targets_sanos    = aws_cloudwatch_metric_alarm.targets_sanos.alarm_name
    errores_5xx      = aws_cloudwatch_metric_alarm.errores_5xx.alarm_name
    auth_401         = aws_cloudwatch_metric_alarm.auth_401.alarm_name
    readiness_no_200 = aws_cloudwatch_metric_alarm.readiness_no_200.alarm_name
    logs_sin_eventos = aws_cloudwatch_metric_alarm.logs_sin_eventos.alarm_name
  }
}

output "presupuesto" {
  description = "Presupuesto declarativo (alarma, no límite de gasto)"
  value = {
    nombre       = aws_budgets_budget.mensual.name
    limite       = aws_budgets_budget.mensual.limit_amount
    notificacion = "BLOCKED_RFC007_SNS_KMS"
  }
}

output "dashboard" {
  description = "Nombre del dashboard si está habilitado"
  value       = var.habilitar_dashboard ? aws_cloudwatch_dashboard.observabilidad[0].dashboard_name : "NO_HABILITADO"
}

output "observabilidad_fcm" {
  description = "Filtros y alarmas FCM/scheduler; deshabilitados por defecto"
  value = {
    metricas_habilitadas = var.habilitar_metricas_fcm
    alarmas_habilitadas  = var.habilitar_alarmas_fcm
    version_esperada     = var.version_esperada
    filtros = {
      arranques                   = var.habilitar_metricas_fcm ? aws_cloudwatch_log_metric_filter.arranques_proceso[0].name : "NO_HABILITADO"
      arranque_version_esperada   = var.habilitar_metricas_fcm && var.version_esperada != "" ? aws_cloudwatch_log_metric_filter.arranque_version_esperada[0].name : "NO_HABILITADO"
      arranque_version_inesperada = var.habilitar_metricas_fcm && var.version_esperada != "" ? aws_cloudwatch_log_metric_filter.arranque_version_inesperada[0].name : "NO_HABILITADO"
      fcm_worker                  = var.habilitar_metricas_fcm ? aws_cloudwatch_log_metric_filter.fcm_worker_heartbeat[0].name : "NO_HABILITADO"
      scheduler                   = var.habilitar_metricas_fcm ? aws_cloudwatch_log_metric_filter.scheduler_heartbeat[0].name : "NO_HABILITADO"
      fcm_errores                 = var.habilitar_metricas_fcm ? aws_cloudwatch_log_metric_filter.fcm_errores_envio[0].name : "NO_HABILITADO"
      access_denied               = var.habilitar_metricas_fcm ? aws_cloudwatch_log_metric_filter.access_denied[0].name : "NO_HABILITADO"
      scheduler_lag               = var.habilitar_metricas_fcm ? aws_cloudwatch_log_metric_filter.scheduler_lag[0].name : "NO_HABILITADO"
    }
    alarmas = {
      fcm_worker_sin_latido       = var.habilitar_alarmas_fcm ? aws_cloudwatch_metric_alarm.fcm_worker_sin_latido[0].alarm_name : "NO_HABILITADO"
      scheduler_sin_latido        = var.habilitar_alarmas_fcm ? aws_cloudwatch_metric_alarm.scheduler_sin_latido[0].alarm_name : "NO_HABILITADO"
      fcm_errores_envio           = var.habilitar_alarmas_fcm ? aws_cloudwatch_metric_alarm.fcm_errores_envio[0].alarm_name : "NO_HABILITADO"
      access_denied               = var.habilitar_alarmas_fcm ? aws_cloudwatch_metric_alarm.access_denied[0].alarm_name : "NO_HABILITADO"
      scheduler_lag               = var.habilitar_alarmas_fcm ? aws_cloudwatch_metric_alarm.scheduler_lag[0].alarm_name : "NO_HABILITADO"
      arranque_version_inesperada = var.habilitar_alarmas_fcm && var.version_esperada != "" ? aws_cloudwatch_metric_alarm.arranque_version_inesperada[0].alarm_name : "NO_HABILITADO"
    }
    coste_estimado_mensual_usd = "3.00 si se habilitan los 8 filtros y 6 alarmas (2.20 sin el par de version); DOC_ONLY, no medido"
    estado                     = var.habilitar_metricas_fcm ? "HABILITADO_EN_PLAN_SIN_APLICAR" : "DESHABILITADO_SIN_RECURSOS"
  }
}

output "estado_despliegue" {
  description = "I-110 se implementa localmente; ningún recurso AWS ha sido creado"
  value       = "NOT_RUN_AWS"
}
