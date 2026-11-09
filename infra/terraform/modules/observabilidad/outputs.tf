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

output "estado_despliegue" {
  description = "I-110 se implementa localmente; ningún recurso AWS ha sido creado"
  value       = "NOT_RUN_AWS"
}
