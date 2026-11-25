output "habilitado" {
  description = "Refleja la variable; false significa cero recursos creados"
  value       = var.habilitar
}

output "nat_gateway_ids" {
  description = "IDs de NAT Gateway; vacío si habilitar=false"
  value       = aws_nat_gateway.nat[*].id
}

output "public_subnet_ids" {
  description = "Subredes públicas dedicadas al NAT; vacío si habilitar=false"
  value       = aws_subnet.publica_nat[*].id
}

output "egress_rule_id" {
  description = "Regla opcional de egreso 443; vacío si no se habilitó"
  value       = try(aws_vpc_security_group_egress_rule.backend_https_internet[0].id, "")
}

output "estado" {
  description = "Estado de preparación; no acredita recursos AWS"
  value       = var.habilitar ? "HABILITADO_EN_PLAN_SIN_APLICAR" : "DESHABILITADO_SIN_RECURSOS"
}
