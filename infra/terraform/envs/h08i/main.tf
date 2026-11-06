terraform {
  required_version = ">= 1.4.0, < 2.0.0"

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
  bits_subredes                = var.bits_subredes
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

module "computo" {
  source = "../../modules/computo"

  proyecto      = "nexum"
  entorno       = var.entorno
  region        = var.region
  subnet_ids    = module.red.subredes_privadas
  sg_backend_id = module.red.security_groups["backend"]

  puerto_backend = var.puerto_backend

  iam_instance_profile_name = var.iam_instance_profile_name
  root_volume_gb            = var.root_volume_gb
  artefacto_s3_uri          = var.artefacto_s3_uri
  artefacto_sha256          = var.artefacto_sha256
  unidad_s3_uri             = var.unidad_s3_uri
  unidad_sha256             = var.unidad_sha256
}

output "asg_name" {
  value = module.computo.asg_name
}

output "capacidad_computo" {
  value = module.computo.capacidad
}

output "cpu_credits" {
  value = module.computo.cpu_credits
}

output "health_endpoints" {
  value = module.computo.health_endpoints
}
