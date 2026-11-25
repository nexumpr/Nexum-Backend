# Egreso HTTPS para FCM/OAuth (propuesta, DESHABILITADO por defecto).
#
# Este módulo existe para que un Gate FinOps/Seguridad pueda revisar el cambio
# exacto sin que el `terraform plan` actual cree recursos. `habilitar = false`
# deja todos los `count` en 0: no hay NAT Gateway, EIP, subred pública, ruta ni
# regla de security group. NO se instancia desde `envs/h08i` todavía.
#
# Topología cuando se habilite:
#   backend privado -> route table privada -> NAT Gateway -> IGW -> HTTPS 443
#   -> oauth2.googleapis.com / fcm.googleapis.com
#
# Limitación declarada: el NAT Gateway no filtra por dominio. La regla opcional
# de egreso 443 a 0.0.0.0/0 sólo se crea con `habilitar_regla_egreso_backend`;
# para allowlist por host hace falta un proxy de egreso (ver propuesta).

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
  }, var.etiquetas)
}

resource "terraform_data" "preconditions" {
  input = {
    habilitar        = var.habilitar
    nat_gateways     = var.numero_nat_gateways
    public_cidrs     = length(var.public_subnet_cidrs)
    private_tables   = length(var.private_route_table_ids)
    regla_backend    = var.habilitar_regla_egreso_backend
    backend_sg       = var.backend_security_group_id
    internet_gateway = var.internet_gateway_id
  }

  lifecycle {
    precondition {
      condition     = !var.habilitar || var.internet_gateway_id != ""
      error_message = "Para habilitar el egreso FCM hace falta el IGW existente de la VPC."
    }

    precondition {
      condition     = !var.habilitar || length(var.public_subnet_cidrs) >= var.numero_nat_gateways
      error_message = "Cada NAT Gateway exige su propia subred pública y CIDR; faltan CIDR."
    }

    precondition {
      condition     = !var.habilitar || length(var.availability_zones) >= var.numero_nat_gateways
      error_message = "Cada NAT Gateway exige una AZ; faltan zonas de disponibilidad."
    }

    precondition {
      condition     = !var.habilitar || length(var.private_route_table_ids) >= 1
      error_message = "Sin al menos una route table privada no se puede enrutar el egreso."
    }

    precondition {
      condition     = !var.habilitar || var.numero_nat_gateways == 1 || length(var.private_route_table_ids) >= var.numero_nat_gateways
      error_message = "Con 2 NAT Gateway se necesita una route table privada por AZ; con una sola tabla el segundo NAT quedaría sin uso."
    }

    precondition {
      condition     = !var.habilitar_regla_egreso_backend || (var.habilitar && var.backend_security_group_id != "")
      error_message = "La regla de egreso 443 exige habilitar=true y el SG real del backend."
    }
  }
}

# Subred pública dedicada al NAT. No aloja EC2 ni asigna IP pública al lanzar
# instancias: `map_public_ip_on_launch = false`; el EIP es exclusivo del NAT.
resource "aws_subnet" "publica_nat" {
  count = local.activo ? var.numero_nat_gateways : 0

  vpc_id                  = var.vpc_id
  cidr_block              = var.public_subnet_cidrs[count.index]
  availability_zone       = var.availability_zones[count.index]
  map_public_ip_on_launch = false

  tags = merge(local.tags, {
    Name = "${local.nombre}-publica-nat-${count.index + 1}"
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
  count = local.activo ? var.numero_nat_gateways : 0

  subnet_id      = aws_subnet.publica_nat[count.index].id
  route_table_id = aws_route_table.publica_nat[0].id
}

resource "aws_eip" "nat" {
  count = local.activo ? var.numero_nat_gateways : 0

  domain = "vpc"

  tags = merge(local.tags, {
    Name = "${local.nombre}-eip-nat-${count.index + 1}"
  })
}

resource "aws_nat_gateway" "nat" {
  count = local.activo ? var.numero_nat_gateways : 0

  allocation_id = aws_eip.nat[count.index].id
  subnet_id     = aws_subnet.publica_nat[count.index].id

  depends_on = [
    aws_route.publica_internet,
    aws_route_table_association.publica_nat,
  ]

  tags = merge(local.tags, {
    Name = "${local.nombre}-nat-${count.index + 1}"
  })
}

# Ruta por defecto de las tablas privadas al NAT. Con 2 NAT y una tabla por AZ,
# la tabla i usa el NAT i; el precondition impide el caso ambiguo.
resource "aws_route" "privada_nat" {
  count = local.activo ? length(var.private_route_table_ids) : 0

  route_table_id         = var.private_route_table_ids[count.index]
  destination_cidr_block = "0.0.0.0/0"
  nat_gateway_id         = aws_nat_gateway.nat[count.index % var.numero_nat_gateways].id
}

# Regla opcional y explícitamente peligrosa: NAT no filtra por dominio. Sin ella,
# el backend seguirá sin poder salir a Internet aunque exista la ruta. Se deja
# desactivada por defecto; un Gate de Seguridad debe aceptar el 0.0.0.0/0:443 o
# sustituirla por un proxy con allowlist de hosts.
resource "aws_vpc_security_group_egress_rule" "backend_https_internet" {
  count = local.activo && var.habilitar_regla_egreso_backend && var.backend_security_group_id != "" ? 1 : 0

  security_group_id = var.backend_security_group_id
  description       = "HTTPS de salida a Internet para FCM/OAuth (riesgo residual; Sin allowlist de dominio)"
  ip_protocol       = "tcp"
  from_port         = 443
  to_port           = 443
  cidr_ipv4         = "0.0.0.0/0"
}
