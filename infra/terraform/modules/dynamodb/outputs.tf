output "table_name" {
  description = "Nombre físico de la tabla única (NEXUM_CORE_<ENV>)"
  value       = aws_dynamodb_table.core.name
}

output "table_arn" {
  description = "ARN de la tabla; lo consumirá la política IAM acotada de I-060"
  value       = aws_dynamodb_table.core.arn
}

output "hash_key" {
  description = "Clave de partición (S) del single-table design"
  value       = aws_dynamodb_table.core.hash_key
}

output "range_key" {
  description = "Clave de ordenación (S)"
  value       = aws_dynamodb_table.core.range_key
}

output "billing_mode" {
  description = "Modo de capacidad; on-demand para no dejar capacidad ociosa"
  value       = aws_dynamodb_table.core.billing_mode
}

output "indices" {
  description = "Índices secundarios creados con sus claves (sin GSI especulativos)"
  value = {
    for i in aws_dynamodb_table.core.global_secondary_index : i.name => {
      hash_key  = i.hash_key
      range_key = i.range_key
    }
  }
}

output "ttl" {
  description = "Atributo TTL declarado (eventual; no sustituye la expiración en aplicación)"
  value = {
    atributo = aws_dynamodb_table.core.ttl[0].attribute_name
    enabled  = aws_dynamodb_table.core.ttl[0].enabled
  }
}

output "cifrado_en_reposo" {
  description = "SSE con clave administrada por AWS"
  value       = aws_dynamodb_table.core.server_side_encryption[0].enabled
}

output "pitr" {
  description = "Point-in-time recovery (presupuestado en I-000)"
  value       = aws_dynamodb_table.core.point_in_time_recovery[0].enabled
}

output "streams" {
  description = "Streams deshabilitados: no hay consumidor aprobado"
  value       = aws_dynamodb_table.core.stream_enabled
}

output "proteccion_borrado" {
  description = "Deletion protection aplicada en este entorno"
  value       = aws_dynamodb_table.core.deletion_protection_enabled
}

output "estado_despliegue" {
  description = "I-040 se implementa localmente; ningún recurso AWS ha sido creado"
  value       = "NOT_RUN_AWS"
}
