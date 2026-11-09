terraform {
  # 1.10 es el mínimo que soporta locking nativo de S3 (`use_lockfile`), que es el
  # mecanismo vigente; el locking con DynamoDB está deprecado. Terraform instalado
  # en el entorno: 1.16.6.
  required_version = ">= 1.10.0, < 2.0.0"

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

  iam_instance_profile_name = module.iam.runtime_instance_profile_name
  secretos_s3_uri           = var.secretos_version == "" ? "" : "${module.secretos.secrets_uri_base}${var.secretos_version}/secrets.json"
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

# --- I-040: DynamoDB (tabla única A-060) -----------------------------------

module "dynamodb" {
  source = "../../modules/dynamodb"

  proyecto = "nexum"
  entorno  = var.entorno
  region   = var.region

  nombre_tabla       = var.dynamodb_nombre_tabla
  atributo_ttl       = var.dynamodb_atributo_ttl
  pitr_habilitado    = var.dynamodb_pitr_habilitado
  proteccion_borrado = var.dynamodb_proteccion_borrado
  habilitar_gsi2     = var.dynamodb_habilitar_gsi2
}

# --- I-060: IAM runtime + trust-only OIDC contracts ------------------------

module "secretos" {
  source = "../../modules/secretos"

  proyecto               = "nexum"
  entorno                = var.entorno
  etiquetas              = var.etiquetas
  rol_runtime_arn        = module.iam.runtime_role_arn
  prefijo                = "nexum/secrets/"
  retencion_version_dias = 30
}

module "iam" {
  source = "../../modules/iam"

  proyecto = "nexum"
  entorno  = var.entorno

  dynamodb_table_arn = module.dynamodb.table_arn
  dynamodb_index_arns = [
    for index_name in keys(module.dynamodb.indices) :
    "${module.dynamodb.table_arn}/index/${index_name}"
  ]
  dynamodb_gsi2_enabled = var.dynamodb_habilitar_gsi2

  # Los objetos exactos esperan I-080; los log groups/sink los define I-110.
  # Listas vacías significan que NO se concede acceso S3/CloudWatch.
  artifact_object_arns      = var.iam_artifact_object_arns
  secretos_object_arns      = [module.secretos.secrets_prefix_arn]
  cloudwatch_log_group_arns = var.iam_cloudwatch_log_group_arns

  # Provider OIDC existente, environment y SCP no se verifican ni mutan en I-060.
  github_oidc_provider_arn    = var.iam_github_oidc_provider_arn
  enable_ci_validation_role   = var.iam_enable_ci_validation_role
  ci_environment_protected    = var.iam_ci_environment_protected
  enable_plan_oidc_role       = var.iam_enable_plan_oidc_role
  enable_apply_oidc_role      = var.iam_enable_apply_oidc_role
  apply_environment_protected = var.iam_apply_environment_protected
}

output "iam" {
  description = "Instance profile runtime, permisos mínimos, y límites SCP/OIDC/secretos"
  value = {
    runtime_role_name        = module.iam.runtime_role_name
    runtime_instance_profile = module.iam.runtime_instance_profile_name
    runtime_permissions      = module.iam.runtime_permissions
    oidc                     = module.iam.oidc
    scp_state                = module.iam.scp_state
    secret_state             = module.iam.secret_state
    state_and_workflows      = module.iam.state_and_workflows
    estado_despliegue        = module.iam.estado_despliegue
  }
}

output "dynamodb" {
  description = "Tabla única, claves, índices, TTL, cifrado y PITR"
  value = {
    table_name         = module.dynamodb.table_name
    table_arn          = module.dynamodb.table_arn
    hash_key           = module.dynamodb.hash_key
    range_key          = module.dynamodb.range_key
    billing_mode       = module.dynamodb.billing_mode
    indices            = module.dynamodb.indices
    ttl                = module.dynamodb.ttl
    cifrado_en_reposo  = module.dynamodb.cifrado_en_reposo
    pitr               = module.dynamodb.pitr
    streams            = module.dynamodb.streams
    proteccion_borrado = module.dynamodb.proteccion_borrado
    estado_despliegue  = module.dynamodb.estado_despliegue
  }
}

# --- I-050: Redis OSS compartido -------------------------------------------

module "redis" {
  source = "../../modules/redis"

  proyecto = "nexum"
  entorno  = var.entorno
  region   = var.region

  subnet_ids    = module.red.subredes_privadas
  sg_redis_id   = module.red.security_groups["redis"]
  sg_backend_id = module.red.security_groups["backend"]

  password_aplicacion = var.redis_password_aplicacion
}

output "redis" {
  description = "Endpoint compartido, topología, seguridad y política de caché"
  value = {
    replication_group_id = module.redis.replication_group_id
    endpoint             = module.redis.endpoint_primario
    puerto               = module.redis.puerto
    motor                = module.redis.motor
    topologia            = module.redis.topologia
    seguridad            = module.redis.seguridad
    cache                = module.redis.cache
    red                  = module.redis.red
    estado_despliegue    = module.redis.estado_despliegue
  }
}


# --- I-110: observabilidad, metricas, alarmas y FinOps -----------------------

module "observabilidad" {
  source = "../../modules/observabilidad"

  proyecto = "nexum"
  entorno  = var.entorno
  region   = var.region

  alb_arn          = module.balanceo.alb_arn
  target_group_arn = module.balanceo.target_group_arn

  retencion_dias      = 7
  umbral_5xx          = 5
  umbral_auth_401     = 20
  presupuesto_usd     = 1
  habilitar_dashboard = true
}

output "observabilidad" {
  description = "Logs, metric filters, alarmas, presupuesto y dashboard (sin AWS aplicado)"
  value = {
    log_group         = module.observabilidad.log_group_backend
    retencion_dias    = module.observabilidad.retencion_dias
    namespace         = module.observabilidad.namespace_metricas
    notificacion      = module.observabilidad.notificacion
    alarmas           = module.observabilidad.alarmas
    presupuesto       = module.observabilidad.presupuesto
    dashboard         = module.observabilidad.dashboard
    estado_despliegue = module.observabilidad.estado_despliegue
  }
}
