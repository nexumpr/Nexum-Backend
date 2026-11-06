###############################################################################
# Nexum-Backend — I-010 red VPC (revisión local, H08-I)
#
# Diseño aprobado (D2): cliente --HTTPS--> CloudFront --HTTP 80 privado--> ALB
# interno --HTTP 8080--> EC2 Go. Sin NAT. Un único interface endpoint permanente
# (`logs`); `secretsmanager`, `ssm` y `ssmmessages` sólo durante ventanas.
#
# Nada de este directorio ha sido aplicado. No contiene backend remoto, estado,
# credenciales ni identificadores de cuenta.
###############################################################################

locals {
  nombre = "${var.proyecto}-${var.entorno}"

  # Endpoints de interfaz permanentes (los gateway no generan ENI propia).
  endpoints_permanentes = {
    logs = "com.amazonaws.${var.region}.logs"
  }

  # Endpoints de interfaz que sólo existen durante una ventana de mantenimiento.
  endpoints_ventana = {
    secretsmanager = "com.amazonaws.${var.region}.secretsmanager"
    ssm            = "com.amazonaws.${var.region}.ssm"
    ssmmessages    = "com.amazonaws.${var.region}.ssmmessages"
  }

  subredes = { for indice, az in var.zonas : az => {
    cidr = var.cidrs_subredes[indice]
    az   = az
  } }
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
    Nota = "Requerido por VPC Origins; sin rutas 0.0.0.0/0"
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

# Tabla de rutas privada: sólo las rutas de los gateway endpoints. No hay ruta
# por defecto a internet ni NAT.
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
# Security groups de mínimo privilegio
###############################################################################

# Prefijo administrado por AWS con las IP de salida de CloudFront hacia orígenes.
data "aws_ec2_managed_prefix_list" "cloudfront_origen" {
  name = "com.amazonaws.global.cloudfront.origin-facing"
}

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
  description = "Instancias Go: ingress 8080 desde ALB; egress a Redis y endpoints"
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
# NACL: stateless, por eso se declaran los puertos efimeros de retorno.
###############################################################################

resource "aws_network_acl" "alb" {
  vpc_id     = aws_vpc.principal.id
  subnet_ids = [for az, s in aws_subnet.privada : s.id]

  tags = {
    Name = "${local.nombre}-nacl-privada"
  }
}

resource "aws_network_acl_rule" "entrada_api" {
  network_acl_id = aws_network_acl.alb.id
  rule_number    = 100
  egress         = false
  protocol       = "tcp"
  rule_action    = "allow"
  cidr_block     = "0.0.0.0/0"
  from_port      = 80
  to_port        = 80
}

resource "aws_network_acl_rule" "entrada_interna" {
  network_acl_id = aws_network_acl.alb.id
  rule_number    = 110
  egress         = false
  protocol       = "tcp"
  rule_action    = "allow"
  cidr_block     = var.cidr_vpc
  from_port      = 8080
  to_port        = 8080
}

resource "aws_network_acl_rule" "entrada_redis" {
  network_acl_id = aws_network_acl.alb.id
  rule_number    = 120
  egress         = false
  protocol       = "tcp"
  rule_action    = "allow"
  cidr_block     = var.cidr_vpc
  from_port      = 6379
  to_port        = 6379
}

resource "aws_network_acl_rule" "salida_https" {
  network_acl_id = aws_network_acl.alb.id
  rule_number    = 100
  egress         = true
  protocol       = "tcp"
  rule_action    = "allow"
  cidr_block     = var.cidr_vpc
  from_port      = 443
  to_port        = 443
}

resource "aws_network_acl_rule" "salida_efimeros" {
  network_acl_id = aws_network_acl.alb.id
  rule_number    = 110
  egress         = true
  protocol       = "tcp"
  rule_action    = "allow"
  cidr_block     = "0.0.0.0/0"
  from_port      = 1024
  to_port        = 65535
}
