output "alb_arn" {
  description = "ARN del ALB interno (no publicable como URL de servicio)"
  value       = aws_lb.principal.arn
}

output "alb_dns_name" {
  description = "DNS interno del ALB; sólo alcanzable desde la VPC"
  value       = aws_lb.principal.dns_name
}

output "alb_es_interno" {
  description = "Debe ser siempre true (ALB nunca internet-facing)"
  value       = aws_lb.principal.internal
}

output "target_group_arn" {
  description = "ARN del target group HTTP:8080"
  value       = aws_lb_target_group.backend.arn
}

output "listener_http_privado" {
  description = "Puerto del listener interno (80) y su protocolo (HTTP)"
  value = {
    puerto     = aws_lb_listener.http_privado.port
    protocolo  = aws_lb_listener.http_privado.protocol
    target_arn = aws_lb_listener.http_privado.default_action[0].target_group_arn
  }
}

output "health_check" {
  description = "Contrato de health check aplicado al target group"
  value = {
    ruta                 = var.health_check_path
    intervalo            = var.health_check_interval
    timeout              = var.health_check_timeout
    healthy              = var.healthy_threshold
    unhealthy            = var.unhealthy_threshold
    matcher              = "200"
    deregistration_delay = var.deregistration_delay
  }
}

output "vpc_origin_id" {
  description = "ID del VPC origin de CloudFront"
  value       = aws_cloudfront_vpc_origin.alb.id
}

output "cloudfront_distribution_id" {
  description = "ID de la distribución CloudFront"
  value       = aws_cloudfront_distribution.api.id
}

output "cloudfront_domain_name" {
  description = "Dominio HTTPS público (*.cloudfront.net) del Backend"
  value       = aws_cloudfront_distribution.api.domain_name
}

output "cloudfront_certificado" {
  description = "Certificado en uso y security policy implícita (sin ACM ni dominio propio)"
  value = {
    certificado_por_defecto = true
    security_policy         = "TLSv1 (fijada por AWS con certificado por defecto, no configurable)"
    tls_app_hasta_alb       = false
  }
}

output "sin_cache" {
  description = "Política de caché aplicada: AWS Managed-CachingDisabled (TTL 0)"
  value       = data.aws_cloudfront_cache_policy.sin_cache.id
}

output "estado_despliegue" {
  description = "I-030 se implementa localmente; ningún recurso AWS ha sido creado"
  value       = "NOT_RUN_AWS"
}
