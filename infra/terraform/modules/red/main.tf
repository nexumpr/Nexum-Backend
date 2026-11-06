###############################################################################
# Nexum-Backend — I-010 red VPC (revisión local 2, H08-I)
#
# Diseño aprobado (D2): cliente --HTTPS--> CloudFront --HTTP 80 privado--> ALB
# interno --HTTP 8080--> EC2 Go. Sin NAT. Un único interface endpoint permanente
# (`logs`); `secretsmanager`, `ssm` y `ssmmessages` sólo durante ventanas.
#
# Correcciones de esta revisión:
#  - Egress del backend a S3 y DynamoDB mediante las prefix lists administradas
#    por AWS (un gateway endpoint no tiene security group propio, pero el SG de
#    la instancia sí evalúa el destino).
#  - NACL reescrita como flujo completo (ingress y egress, incluidos efímeros de
#    retorno y tráfico intra-VPC) porque es stateless.
#  - Subredes derivadas del CIDR de la VPC con `cidrsubnet`: contención y no
#    superposición garantizadas por construcción.
#
# Nada de este directorio ha sido aplicado.
###############################################################################

locals {
  nombre = "${var.proyecto}-${var.entorno}"

  # Subredes derivadas: mismas bits para todas, índices distintos -> nunca se
  # solapan y siempre están contenidas en la VPC.
  subredes = { for indice, az in var.zonas : az => {
    cidr = cidrsubnet(var.cidr_vpc, var.bits_subredes, 10 + indice)
    az   = az
  } }

  endpoints_permanentes = {
    logs = "com.amazonaws.${var.region}.logs"
  }

  endpoints_ventana = {
    secretsmanager = "com.amazonaws.${var.region}.secretsmanager"
    ssm            = "com.amazonaws.${var.region}.ssm"
    ssmmessages    = "com.amazonaws.${var.region}.ssmmessages"
  }
}

resource "aws_vpc" "principal" {
  cidr_block           = var.cidr_vpc
  enable_dns_support   = true
  enable_dns_hostnames = true

  tags = {
    Name      = "${local.nombre}-vpc"
    Proposito = "I-010-red"
  }
}

# Requisito oficial de CloudFront VPC Origins: el Internet Gateway debe estar
# adjunto a la VPC, pero NO se usa para enrutar y NO habilita rutas públicas.
resource "aws_internet_gateway" "principal" {
  vpc_id = aws_vpc.principal.id

  tags = {
    Name = "${local.nombre}-igw"
    Nota = "Requerido por VPC Origins; sin ruta por defecto"
  }
}

resource "aws_subnet" "privada" {
  for_each = local.subredes

  vpc_id                  = aws_vpc.principal.id
  cidr_block              = each.value.cidr
  availability_zone       = each.value.az
  map_public_ip_on_launch = false

  tags = {
    Name = "${local.nombre}-privada-${each.key}"
    Tipo = "privada"
  }
}

resource "aws_route_table" "privada" {
  vpc_id = aws_vpc.principal.id

  tags = {
    Name = "${local.nombre}-rt-privada"
    Nota = "Sin ruta por defecto a internet"
  }
}

resource "aws_route_table_association" "privada" {
  for_each = aws_subnet.privada

  subnet_id      = each.value.id
  route_table_id = aws_route_table.privada.id
}

# Gateway endpoints: sin cargo por hora ni por GB. Cubren DynamoDB y S3.
resource "aws_vpc_endpoint" "s3" {
  vpc_id            = aws_vpc.principal.id
  service_name      = "com.amazonaws.${var.region}.s3"
  vpc_endpoint_type = "Gateway"
  route_table_ids   = [aws_route_table.privada.id]

  tags = {
    Name = "${local.nombre}-gw-s3"
  }
}

resource "aws_vpc_endpoint" "dynamodb" {
  vpc_id            = aws_vpc.principal.id
  service_name      = "com.amazonaws.${var.region}.dynamodb"
  vpc_endpoint_type = "Gateway"
  route_table_ids   = [aws_route_table.privada.id]

  tags = {
    Name = "${local.nombre}-gw-dynamodb"
  }
}

###############################################################################
# Prefix lists administradas por AWS
###############################################################################

data "aws_ec2_managed_prefix_list" "cloudfront_origen" {
  name = "com.amazonaws.global.cloudfront.origin-facing"
}

# Necesarias para el egress del backend a los gateway endpoints: el gateway no
# tiene SG, pero el SG de la instancia evalúa el destino (IP públicas de AWS).
data "aws_ec2_managed_prefix_list" "s3" {
  name = "com.amazonaws.${var.region}.s3"
}

data "aws_ec2_managed_prefix_list" "dynamodb" {
  name = "com.amazonaws.${var.region}.dynamodb"
}

###############################################################################
# Security groups de mínimo privilegio
###############################################################################

resource "aws_security_group" "alb" {
  name        = "${local.nombre}-sg-alb"
  description = "Ingress HTTP 80 solo desde CloudFront; egress al backend en 8080"
  vpc_id      = aws_vpc.principal.id

  tags = {
    Name = "${local.nombre}-sg-alb"
  }
}

resource "aws_vpc_security_group_ingress_rule" "alb_cloudfront_80" {
  security_group_id = aws_security_group.alb.id
  description       = "HTTP 80 desde la prefix list de CloudFront (tramo privado de VPC Origins)"
  ip_protocol       = "tcp"
  from_port         = 80
  to_port           = 80
  prefix_list_id    = data.aws_ec2_managed_prefix_list.cloudfront_origen.id
}

resource "aws_vpc_security_group_egress_rule" "alb_backend_8080" {
  security_group_id            = aws_security_group.alb.id
  description                  = "HTTP 8080 hacia las instancias Go"
  ip_protocol                  = "tcp"
  from_port                    = 8080
  to_port                      = 8080
  referenced_security_group_id = aws_security_group.backend.id
}

resource "aws_security_group" "backend" {
  name        = "${local.nombre}-sg-backend"
  description = "Instancias Go: ingress desde ALB; egress a Redis, S3, DynamoDB y endpoints"
  vpc_id      = aws_vpc.principal.id

  tags = {
    Name = "${local.nombre}-sg-backend"
  }
}

resource "aws_vpc_security_group_ingress_rule" "backend_desde_alb" {
  security_group_id            = aws_security_group.backend.id
  description                  = "API y health checks desde el ALB"
  ip_protocol                  = "tcp"
  from_port                    = var.puerto_backend
  to_port                      = var.puerto_backend
  referenced_security_group_id = aws_security_group.alb.id
}

resource "aws_vpc_security_group_egress_rule" "backend_redis" {
  security_group_id            = aws_security_group.backend.id
  description                  = "Redis OSS en 6379"
  ip_protocol                  = "tcp"
  from_port                    = 6379
  to_port                      = 6379
  referenced_security_group_id = aws_security_group.redis.id
}

# Corrección A: S3 y DynamoDB por gateway endpoint. El SG del gateway no existe;
# el de la instancia debe permitir 443 hacia las IP de AWS del servicio.
resource "aws_vpc_security_group_egress_rule" "backend_s3_443" {
  security_group_id = aws_security_group.backend.id
  description       = "HTTPS a S3 por gateway endpoint (prefix list administrada)"
  ip_protocol       = "tcp"
  from_port         = 443
  to_port           = 443
  prefix_list_id    = data.aws_ec2_managed_prefix_list.s3.id
}

resource "aws_vpc_security_group_egress_rule" "backend_dynamodb_443" {
  security_group_id = aws_security_group.backend.id
  description       = "HTTPS a DynamoDB por gateway endpoint (prefix list administrada)"
  ip_protocol       = "tcp"
  from_port         = 443
  to_port           = 443
  prefix_list_id    = data.aws_ec2_managed_prefix_list.dynamodb.id
}

resource "aws_vpc_security_group_egress_rule" "backend_endpoints_443" {
  security_group_id            = aws_security_group.backend.id
  description                  = "Interface endpoints (logs y ventana) en 443"
  ip_protocol                  = "tcp"
  from_port                    = 443
  to_port                      = 443
  referenced_security_group_id = aws_security_group.endpoints.id
}

resource "aws_security_group" "redis" {
  name        = "${local.nombre}-sg-redis"
  description = "Redis OSS: solo acepta 6379 desde el backend; sin exposicion publica"
  vpc_id      = aws_vpc.principal.id

  tags = {
    Name = "${local.nombre}-sg-redis"
  }
}

resource "aws_vpc_security_group_ingress_rule" "redis_desde_backend" {
  security_group_id            = aws_security_group.redis.id
  description                  = "Redis 6379 desde el backend"
  ip_protocol                  = "tcp"
  from_port                    = 6379
  to_port                      = 6379
  referenced_security_group_id = aws_security_group.backend.id
}

resource "aws_security_group" "endpoints" {
  name        = "${local.nombre}-sg-endpoints"
  description = "Interface endpoints: ingress 443 desde el backend"
  vpc_id      = aws_vpc.principal.id

  tags = {
    Name = "${local.nombre}-sg-endpoints"
  }
}

resource "aws_vpc_security_group_ingress_rule" "endpoints_desde_backend" {
  security_group_id            = aws_security_group.endpoints.id
  description                  = "443 desde las instancias Go"
  ip_protocol                  = "tcp"
  from_port                    = 443
  to_port                      = 443
  referenced_security_group_id = aws_security_group.backend.id
}

###############################################################################
# Interface endpoints: 1 permanente + 3 de ventana
###############################################################################

resource "aws_vpc_endpoint" "permanente" {
  for_each = local.endpoints_permanentes

  vpc_id              = aws_vpc.principal.id
  service_name        = each.value
  vpc_endpoint_type   = "Interface"
  subnet_ids          = [for s in aws_subnet.privada : s.id]
  security_group_ids  = [aws_security_group.endpoints.id]
  private_dns_enabled = true

  tags = {
    Name = "${local.nombre}-vpce-${each.key}"
    Modo = "permanente"
  }
}

resource "aws_vpc_endpoint" "ventana" {
  for_each = var.ventana_mantenimiento_activa ? local.endpoints_ventana : {}

  vpc_id              = aws_vpc.principal.id
  service_name        = each.value
  vpc_endpoint_type   = "Interface"
  subnet_ids          = [for s in aws_subnet.privada : s.id]
  security_group_ids  = [aws_security_group.endpoints.id]
  private_dns_enabled = true

  tags = {
    Name = "${local.nombre}-vpce-${each.key}"
    Modo = "ventana-mantenimiento"
  }
}

###############################################################################
# NACL: stateless. Se declara el flujo completo por sentido.
#
# Ingress (destino = puerto en la instancia del subnet):
#   80          CloudFront -> ALB              0.0.0.0/0 (CloudFront no tiene CIDR VPC)
#   8080        ALB -> EC2                     CIDR VPC
#   6379        EC2 -> Redis                   CIDR VPC
#   1024-65535  retorno de servicios          0.0.0.0/0 (efímeros)
# Egress (destino):
#   443         AWS APIs (S3, DynamoDB,      0.0.0.0/0 (destinos públicos de AWS)
#               endpoints, secrets, ssm)
#   8080        ALB -> EC2                    CIDR VPC
#   6379        EC2 -> Redis                  CIDR VPC
#   1024-65535  retorno (ALB->CloudFront,     0.0.0.0/0
#               EC2->ALB, endpoints->EC2)
#
# Las prefix lists administradas no pueden usarse en NACL; el mínimo privilegio
# fino se aplica en los security groups (documentado en el README).
###############################################################################

resource "aws_network_acl" "privada" {
  vpc_id     = aws_vpc.principal.id
  subnet_ids = [for az, s in aws_subnet.privada : s.id]

  tags = {
    Name = "${local.nombre}-nacl-privada"
    Nota = "Stateless: incluye puertos efimeros de retorno"
  }
}

resource "aws_network_acl_rule" "ingress_alb_80" {
  network_acl_id = aws_network_acl.privada.id
  rule_number    = 100
  egress         = false
  protocol       = "tcp"
  rule_action    = "allow"
  cidr_block     = "0.0.0.0/0"
  from_port      = 80
  to_port        = 80
}

resource "aws_network_acl_rule" "ingress_backend_8080" {
  network_acl_id = aws_network_acl.privada.id
  rule_number    = 110
  egress         = false
  protocol       = "tcp"
  rule_action    = "allow"
  cidr_block     = var.cidr_vpc
  from_port      = var.puerto_backend
  to_port        = var.puerto_backend
}

resource "aws_network_acl_rule" "ingress_redis_6379" {
  network_acl_id = aws_network_acl.privada.id
  rule_number    = 120
  egress         = false
  protocol       = "tcp"
  rule_action    = "allow"
  cidr_block     = var.cidr_vpc
  from_port      = 6379
  to_port        = 6379
}

resource "aws_network_acl_rule" "ingress_efimeros" {
  network_acl_id = aws_network_acl.privada.id
  rule_number    = 130
  egress         = false
  protocol       = "tcp"
  rule_action    = "allow"
  cidr_block     = "0.0.0.0/0"
  from_port      = 1024
  to_port        = 65535
}

resource "aws_network_acl_rule" "egress_https" {
  network_acl_id = aws_network_acl.privada.id
  rule_number    = 100
  egress         = true
  protocol       = "tcp"
  rule_action    = "allow"
  cidr_block     = "0.0.0.0/0"
  from_port      = 443
  to_port        = 443
}

resource "aws_network_acl_rule" "egress_backend_8080" {
  network_acl_id = aws_network_acl.privada.id
  rule_number    = 110
  egress         = true
  protocol       = "tcp"
  rule_action    = "allow"
  cidr_block     = var.cidr_vpc
  from_port      = var.puerto_backend
  to_port        = var.puerto_backend
}

resource "aws_network_acl_rule" "egress_redis_6379" {
  network_acl_id = aws_network_acl.privada.id
  rule_number    = 120
  egress         = true
  protocol       = "tcp"
  rule_action    = "allow"
  cidr_block     = var.cidr_vpc
  from_port      = 6379
  to_port        = 6379
}

resource "aws_network_acl_rule" "egress_efimeros" {
  network_acl_id = aws_network_acl.privada.id
  rule_number    = 130
  egress         = true
  protocol       = "tcp"
  rule_action    = "allow"
  cidr_block     = "0.0.0.0/0"
  from_port      = 1024
  to_port        = 65535
}
