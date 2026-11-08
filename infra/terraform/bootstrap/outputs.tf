output "bucket_state" {
  description = "Nombre del bucket de state (no publicar el ID de cuenta real)"
  value       = aws_s3_bucket.state.id
}

output "bucket_state_arn" {
  description = "ARN del bucket de state; lo consumirán las políticas de I-060/I-080"
  value       = aws_s3_bucket.state.arn
}

output "lock_table" {
  description = "Tabla de locking de Terraform"
  value       = aws_dynamodb_table.lock.name
}

output "lock_table_arn" {
  description = "ARN de la tabla de locking"
  value       = aws_dynamodb_table.lock.arn
}

output "configuracion_backend" {
  description = "Bloque backend que debe usarse tras aplicar el bootstrap con Gate humano"
  value = {
    bucket         = aws_s3_bucket.state.id
    key            = "envs/${var.entorno}/terraform.tfstate"
    region         = var.region
    dynamodb_table = aws_dynamodb_table.lock.name
    encrypt        = true
    use_lockfile   = false
  }
}

output "protecciones" {
  description = "Controles declarados sobre el state"
  value = {
    versionado      = true
    cifrado         = "SSE-S3 AES256 (sin CMK propia: coste KMS no presupuestado)"
    bloqueo_publico = true
    prevent_destroy = true
    tls_obligatorio = true
  }
}

output "estado_despliegue" {
  description = "Bootstrap declarado en local; ningún recurso AWS creado"
  value       = "NOT_RUN_AWS"
}
