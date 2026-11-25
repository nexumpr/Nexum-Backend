output "habilitado" {
  description = "Refleja la variable; false significa cero recursos creados"
  value       = var.habilitar
}

output "nat_gateway_id" {
  description = "ID del NAT Gateway (vacío si habilitar=false)"
  value       = try(aws_nat_gateway.nat[0].id, "")
}

output "nat_gateway_ids" {
  description = "Lista de IDs de NAT Gateway; contiene 0 o 1 elemento (1 AZ)"
  value       = aws_nat_gateway.nat[*].id
}

output "eip" {
  description = "EIP del NAT como objeto; null si habilitar=false"
  value = try({
    id            = aws_eip.nat[0].id
    allocation_id = aws_eip.nat[0].allocation_id
    public_ip     = aws_eip.nat[0].public_ip
  }, null)
}

output "private_route_table_ids" {
  description = "Tablas privadas que reciben la ruta por defecto al NAT"
  value       = var.private_route_table_ids
}

output "public_subnet_id" {
  description = "Subred pública nueva y dedicada al NAT; vacío si habilitar=false"
  value       = try(aws_subnet.publica_nat[0].id, "")
}

output "regla_egreso_id" {
  description = "Regla de egreso TCP 443 creada (CIDR o prefix list); vacío si no se habilitó"
  value = try(
    aws_vpc_security_group_egress_rule.backend_https_cidr[0].id,
    try(aws_vpc_security_group_egress_rule.backend_https_prefix_list[0].id, "")
  )
}

output "cidr_egreso_https" {
  description = "Destino del egreso 443 aplicado; refleja la variable aunque habilitar=false"
  value       = var.cidr_egreso_https
}

output "prefix_list_egreso_https_id" {
  description = "Prefix list real aplicada, si se aportó"
  value       = var.prefix_list_egreso_https_id
}

output "estado" {
  description = "Estado de preparación; no acredita recursos AWS"
  value       = var.habilitar ? "HABILITADO_EN_PLAN_SIN_APLICAR" : "DESHABILITADO_SIN_RECURSOS"
}

output "punto_unico_de_fallo" {
  description = "Un NAT en una AZ: el egreso FCM/OAuth depende de esa AZ"
  value       = var.habilitar
}
