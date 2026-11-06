terraform {
  required_version = ">= 1.6.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.60"
    }
  }

  # Sin bloque backend: la validación local se hace con
  # `terraform init -backend=false`. El backend real (S3 + DynamoDB) se define
  # en I-070 con su propio Gate.
}

provider "aws" {
  region = var.region

  default_tags {
    tags = merge(
      {
        Proyecto = "Nexum-Backend"
        Spec     = "I-010-red-vpc"
        Entorno  = var.entorno
        Gestion  = "terraform"
      },
      var.etiquetas
    )
  }
}

module "red" {
  source = "../../modules/red"

  proyecto                     = "nexum"
  entorno                      = var.entorno
  region                       = var.region
  cidr_vpc                     = var.cidr_vpc
  cidrs_subredes               = var.cidrs_subredes
  zonas                        = var.zonas
  puerto_backend               = var.puerto_backend
  ventana_mantenimiento_activa = var.ventana_mantenimiento_activa
}

output "vpc_id" {
  value = module.red.vpc_id
}

output "subredes_privadas" {
  value = module.red.subredes_privadas
}

output "security_groups" {
  value = module.red.security_groups
}

output "interface_endpoint_permanente" {
  value = module.red.interface_endpoint_permanente
}
