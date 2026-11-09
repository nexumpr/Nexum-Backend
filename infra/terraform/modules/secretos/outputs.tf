output "bucket_name" {
  description = "Nombre del bucket de secretos."
  value       = aws_s3_bucket.secretos.bucket
}

output "bucket_arn" {
  description = "ARN del bucket de secretos."
  value       = aws_s3_bucket.secretos.arn
}

output "secrets_prefix_arn" {
  description = "ARN del prefijo que el rol de instancia puede leer."
  value       = "${aws_s3_bucket.secretos.arn}/${var.prefijo}*"
}

output "secrets_uri_base" {
  description = "URI base del bundle; el operador anade '<version>/secrets.json'."
  value       = "s3://${aws_s3_bucket.secretos.bucket}/${var.prefijo}"
}
