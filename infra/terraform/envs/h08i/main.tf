terraform {
  required_version = ">= 1.4.0, < 2.0.0"

  required_providers {
    aws = {
      source = "hashicorp/aws"
      # `aws_cloudfront_vpc_origin` existe desde el proveedor 5.72.0.
      version = "~> 5.72"
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

# --- I-030: balanceo interno + publicación HTTPS por CloudFront (D2) -------

module "balanceo" {
  source = "../../modules/balanceo"

  proyecto = "nexum"
  entorno  = var.entorno
  region   = var.region
  vpc_id   = module.red.vpc_id

  subnet_ids    = module.red.subredes_privadas
  sg_alb_id     = module.red.security_groups["alb"]
  sg_backend_id = module.red.security_groups["backend"]
  asg_name      = module.computo.asg_name

  puerto_backend         = var.puerto_backend
  health_check_path      = var.health_check_path
  cloudfront_price_class = var.cloudfront_price_class
}

output "balanceo" {
  description = "ALB interno, target group, VPC origin y dominio CloudFront"
  value = {
    alb_arn           = module.balanceo.alb_arn
    alb_es_interno    = module.balanceo.alb_es_interno
    target_group_arn  = module.balanceo.target_group_arn
    listener          = module.balanceo.listener_http_privado
    health_check      = module.balanceo.health_check
    vpc_origin_id     = module.balanceo.vpc_origin_id
    cloudfront_domain = module.balanceo.cloudfront_domain_name
    cloudfront_id     = module.balanceo.cloudfront_distribution_id
    certificado       = module.balanceo.cloudfront_certificado
    sin_cache         = module.balanceo.sin_cache
    estado_despliegue = module.balanceo.estado_despliegue
  }
}
