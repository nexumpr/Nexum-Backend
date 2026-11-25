# Egreso HTTPS para FCM/OAuth — NAT Gateway administrado en 1 AZ.
#
# DESHABILITADO por defecto (`habilitar = false`): todos los recursos tienen
# `count = 0` y no se crea NAT, EIP, subred pública, ruta ni regla de SG.
#
# Topología cuando se habilita:
#   EC2 privadas (I-010, sin IPv4 pública)
#     -> route table privada (0.0.0.0/0)
#     -> NAT Gateway en subred pública NUEVA y dedicada (1 AZ)
#     -> IGW existente (adjunto por VPC Origins, sin rutas públicas previas)
#     -> HTTPS 443 -> oauth2.googleapis.com / fcm.googleapis.com
#
# La subred pública dedicada NO aloja EC2: `map_public_ip_on_launch = false`.
# El único EIP es el del NAT. No se abre SSH ni se asigna IPv4 pública a las
# instancias. El ALB sigue siendo interno.
#
# Límites declarados:
#  - Un solo NAT en una sola AZ es punto único de fallo de egreso.
#  - NAT no filtra por dominio. Sin una prefix list real de Google, el egreso
#    TCP 443 se abre a `cidr_egreso_https` (0.0.0.0/0 por defecto). Los rangos
#    de FCM/OAuth cambian y no existe una prefix list administrada por AWS para
#    ellos; este módulo NO inventa una.
#  - La VPC de I-010 ya tiene `enable_dns_support` y `enable_dns_hostnames`;
#    sin ellos el endpoint público de Google no resolvería. Se documenta y se
#    prueba en `tests/infra/terraform/test_egreso_fcm_contrato.py`; no se muta
#    la VPC desde este módulo.

terraform {
  required_version = ">= 1.4.0, < 2.0.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.72"
    }
  }
}

locals {
  nombre = "${var.proyecto}-${var.entorno}"
  activo = var.habilitar
  tags = merge({
    Name       = "${local.nombre}-egreso-fcm"
    Componente = "egreso-fcm"
    Spec       = "H08-E-FCM"
    Rol        = "nat-gateway-1az"
  }, var.etiquetas)
}

resource "terraform_data" "preconditions" {
  count = local.activo ? 1 : 0

  input = {
    habilitar        = var.habilitar
    vpc              = var.vpc_id
    internet_gateway = var.internet_gateway_id
    zona             = var.zona
    subred_publica   = var.subred_publica_cidr
    private_tables   = length(var.private_route_table_ids)
    backend_sg       = var.backend_security_group_id
    prefix_list      = var.prefix_list_egreso_https_id
    cidr_https       = var.cidr_egreso_https
  }

  lifecycle {
    precondition {
      condition     = !var.habilitar || var.vpc_id != ""
      error_message = "Para habilitar el egreso FCM hace falta la VPC existente de I-010."
    }

    precondition {
      condition     = !var.habilitar || var.internet_gateway_id != ""
      error_message = "Para habilitar el egreso FCM hace falta el IGW existente de I-010 (ya adjunto por VPC Origins)."
    }

    precondition {
      condition     = !var.habilitar || var.backend_security_group_id != ""
      error_message = "Sin el SG real del backend no se puede añadir el egreso TCP 443."
    }

    precondition {
      condition     = !var.habilitar || length(var.private_route_table_ids) >= 1
      error_message = "Sin al menos una route table privada no se puede enrutar el egreso del backend."
    }

    precondition {
      condition     = !var.habilitar || startswith(var.zona, "${var.region}")
      error_message = "La AZ del NAT debe pertenecer a var.region."
    }

    precondition {
      condition     = !var.habilitar || tonumber(split("/", var.subred_publica_cidr)[1]) <= 27
      error_message = "La subred pública del NAT debe ser /27 o mayor (AWS reserva 5 direcciones)."
    }

    precondition {
      condition     = !var.habilitar || var.prefix_list_egreso_https_id != "" || can(cidrhost(var.cidr_egreso_https, 0))
      error_message = "Debe indicarse una prefix list real o un CIDR IPv4 válido para el egreso 443."
    }

    precondition {
      condition     = !var.habilitar || var.prefix_list_egreso_https_id == "" || var.cidr_egreso_https == "0.0.0.0/0"
      error_message = "Si se aporta una prefix list, deje cidr_egreso_https en 0.0.0.0/0 para no simular dos allowlists simultáneas."
    }

    precondition {
      condition     = !var.habilitar || (var.prefix_list_egreso_https_id == "" || can(regex("^pl-[0-9a-f]+$", var.prefix_list_egreso_https_id)))
      error_message = "La prefix list debe ser un ID pl-* existente; el módulo no la crea."
    }
  }
}

# Subred pública NUEVA y dedicada al NAT. No aloja EC2 ni asigna IP pública al
# lanzar instancias: el EIP es exclusivo del NAT.
#
# Desviación declarada: esta subred no recibe una NACL propia y queda asociada a
# la NACL por defecto de la VPC (allow all). No es un problema de exposición
# porque el único recurso es la ENI del NAT, que no acepta conexiones entrantes
# iniciadas desde Internet; el tráfico de retorno lo gestiona el propio NAT. Si
# en el futuro se aloja algo más aquí, hay que crear una NACL restrictiva.
resource "aws_subnet" "publica_nat" {
  count = local.activo ? 1 : 0

  vpc_id                  = var.vpc_id
  cidr_block              = var.subred_publica_cidr
  availability_zone       = var.zona
  map_public_ip_on_launch = false

  tags = merge(local.tags, {
    Name = "${local.nombre}-publica-nat-${var.zona}"
    Tipo = "publica-solo-nat"
  })
}

resource "aws_route_table" "publica_nat" {
  count = local.activo ? 1 : 0

  vpc_id = var.vpc_id

  tags = merge(local.tags, {
    Name = "${local.nombre}-rtb-publica-nat"
  })
}

resource "aws_route" "publica_internet" {
  count = local.activo ? 1 : 0

  route_table_id         = aws_route_table.publica_nat[0].id
  destination_cidr_block = "0.0.0.0/0"
  gateway_id             = var.internet_gateway_id
}

resource "aws_route_table_association" "publica_nat" {
  count = local.activo ? 1 : 0

  subnet_id      = aws_subnet.publica_nat[0].id
  route_table_id = aws_route_table.publica_nat[0].id
}

resource "aws_eip" "nat" {
  count = local.activo ? 1 : 0

  domain = "vpc"

  tags = merge(local.tags, {
    Name = "${local.nombre}-eip-nat"
  })
}

resource "aws_nat_gateway" "nat" {
  count = local.activo ? 1 : 0

  allocation_id     = aws_eip.nat[0].id
  subnet_id         = aws_subnet.publica_nat[0].id
  connectivity_type = "public"

  depends_on = [
    aws_route.publica_internet,
    aws_route_table_association.publica_nat,
  ]

  tags = merge(local.tags, {
    Name = "${local.nombre}-nat-1az"
  })
}

# Ruta por defecto de las tablas privadas del backend al NAT único. Los
# gateway endpoints S3/DynamoDB de I-010 siguen teniendo prioridad por prefijo
# más específico; esta ruta sólo se usa para destinos fuera de la VPC que no
# tengan un endpoint.
resource "aws_route" "privada_nat" {
  count = local.activo ? length(var.private_route_table_ids) : 0

  route_table_id         = var.private_route_table_ids[count.index]
  destination_cidr_block = "0.0.0.0/0"
  nat_gateway_id         = aws_nat_gateway.nat[0].id
}

# Egreso TCP 443. Si existe una prefix list real de Google (no la crea este
# módulo), se usa; si no, se abre a Internet porque no hay una lista
# administrada por AWS para FCM/OAuth. El NAT no filtra por dominio: el riesgo
# residual se documenta en `PROPUESTA_EGRESO_FCM.md`.
resource "aws_vpc_security_group_egress_rule" "backend_https_cidr" {
  count = local.activo && var.prefix_list_egreso_https_id == "" ? 1 : 0

  security_group_id = var.backend_security_group_id
  description       = "HTTPS 443 a Internet para FCM/OAuth (sin allowlist de dominio; rangos de Google cambian)"
  ip_protocol       = "tcp"
  from_port         = 443
  to_port           = 443
  cidr_ipv4         = var.cidr_egreso_https
}

resource "aws_vpc_security_group_egress_rule" "backend_https_prefix_list" {
  count = local.activo && var.prefix_list_egreso_https_id != "" ? 1 : 0

  security_group_id = var.backend_security_group_id
  description       = "HTTPS 443 a la prefix list real aportada para FCM/OAuth"
  ip_protocol       = "tcp"
  from_port         = 443
  to_port           = 443
  prefix_list_id    = var.prefix_list_egreso_https_id
}
