output "bucket_name" {
  description = "Nombre del bucket de artefactos."
  value       = aws_s3_bucket.artefactos.bucket
}

output "bucket_arn" {
  description = "ARN del bucket."
  value       = aws_s3_bucket.artefactos.arn
}

output "objetos_arn" {
  description = "ARN del prefijo de releases; lo consume el rol runtime para GetObject."
  value       = "${aws_s3_bucket.artefactos.arn}/${var.prefijo}*"
}

output "uri_base" {
  description = "URI base: el release anade '<sha>/nexum-backend-linux-arm64'."
  value       = "s3://${aws_s3_bucket.artefactos.bucket}/${var.prefijo}"
}
