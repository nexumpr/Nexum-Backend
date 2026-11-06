output "vpc_id" {
  description = "VPC del Backend"
  value       = aws_vpc.principal.id
}

output "subredes_privadas" {
  description = "Mapa AZ -> id de subred privada"
  value       = { for az, s in aws_subnet.privada : az => s.id }
}

output "cidr_vpc" {
  description = "CIDR efectivo de la VPC"
  value       = aws_vpc.principal.cidr_block
}

output "route_table_privada" {
  description = "Tabla de rutas privada (sin ruta por defecto)"
  value       = aws_route_table.privada.id
}

output "internet_gateway_id" {
  description = "IGW exigido por CloudFront VPC Origins (no enruta)"
  value       = aws_internet_gateway.principal.id
}

output "gateway_endpoints" {
  description = "Gateway endpoints S3 y DynamoDB (sin cargo)"
  value = {
    s3       = aws_vpc_endpoint.s3.id
    dynamodb = aws_vpc_endpoint.dynamodb.id
  }
}

output "interface_endpoint_permanente" {
  description = "Interface endpoints permanentes (logs)"
  value       = { for k, e in aws_vpc_endpoint.permanente : k => e.id }
}

output "interface_endpoints_ventana" {
  description = "Interface endpoints activos sólo en ventana de mantenimiento"
  value       = { for k, e in aws_vpc_endpoint.ventana : k => e.id }
}

output "security_groups" {
  description = "IDs de los security groups de I-010 (titularidad del módulo)"
  value = {
    alb       = aws_security_group.alb.id
    backend   = aws_security_group.backend.id
    redis     = aws_security_group.redis.id
    endpoints = aws_security_group.endpoints.id
  }
}

output "nacl_id" {
  description = "NACL de las subredes privadas"
  value       = aws_network_acl.alb.id
}

output "ventana_activa" {
  description = "Refleja si la ventana de mantenimiento está habilitada"
  value       = var.ventana_mantenimiento_activa
}
