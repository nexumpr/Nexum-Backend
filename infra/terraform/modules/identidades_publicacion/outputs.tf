# RFC-011 — Outputs del módulo de identidades de publicación.

output "artefactos_publicador_arn" {
  description = "ARN del rol que publica binarios y SBOM"
  value       = aws_iam_role.artefactos_publicador.arn
}

output "secretos_publicador_arn" {
  description = "ARN del rol que publica el bundle de secretos"
  value       = aws_iam_role.secretos_publicador.arn
}

output "secretos_gestor_iac_arn" {
  description = "ARN del rol gestor IaC del bucket de secretos"
  value       = aws_iam_role.secretos_gestor_iac.arn
}

output "nombres" {
  description = "Nombres deterministas de las tres identidades"
  value = {
    artefactos_publicador = aws_iam_role.artefactos_publicador.name
    secretos_publicador   = aws_iam_role.secretos_publicador.name
    secretos_gestor_iac   = aws_iam_role.secretos_gestor_iac.name
  }
}
