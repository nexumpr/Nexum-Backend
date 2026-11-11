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

# RFC-011 — Nombres deterministas de los buckets y de las identidades de
# publicación. Se derivan de la cuenta para poder referenciar los ARN sin
# crear un ciclo entre los roles y los módulos que crean los buckets.
data "aws_caller_identity" "actual" {}

locals {
  cuenta_id = data.aws_caller_identity.actual.account_id

  bucket_artefactos_nombre = lower("nexum-${var.entorno}-artefactos-${local.cuenta_id}")
  bucket_secretos_nombre   = lower("nexum-${var.entorno}-secretos-${local.cuenta_id}")

  # Nombres de rol esperados. Se declaran aparte porque `checks.tf` los compara
  # con `module.identidades.nombres` en una **precondition bloqueante** (los
  # `check` sólo advierten y no detienen el apply).
  nombre_artefactos_publicador = lower("nexum-${var.entorno}-artefactos-publicador")
  nombre_secretos_publicador   = lower("nexum-${var.entorno}-secretos-publicador")
  nombre_secretos_gestor_iac   = lower("nexum-${var.entorno}-secretos-gestor-iac")

  # ARN derivados del ID de cuenta y del nombre de rol que crea el módulo
  # `identidades_publicacion`. No son valores inventados: son la forma canónica
  # del ARN de un rol de IAM en esta cuenta. Al ser función determinista de
  # (cuenta, nombre), la precondition sobre el nombre garantiza el ARN; el `check`
  # sobre el ARN real añade la comprobación en el apply.
  arn_artefactos_publicador = "arn:aws:iam::${local.cuenta_id}:role/${local.nombre_artefactos_publicador}"
  arn_secretos_publicador   = "arn:aws:iam::${local.cuenta_id}:role/${local.nombre_secretos_publicador}"
  arn_secretos_gestor_iac   = "arn:aws:iam::${local.cuenta_id}:role/${local.nombre_secretos_gestor_iac}"
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

# Etapa inicial: no instanciar EC2 hasta publicar artefactos y verificar SHA.
# El módulo I-020 conserva capacidad fija de DOS instancias cuando se habilita.
module "computo" {
  count = var.desplegar_aplicacion ? 1 : 0

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
  value = var.desplegar_aplicacion ? module.computo[0].asg_name : null
}

output "capacidad_computo" {
  value = var.desplegar_aplicacion ? module.computo[0].capacidad : null
}

output "cpu_credits" {
  value = var.desplegar_aplicacion ? module.computo[0].cpu_credits : null
}

output "health_endpoints" {
  value = var.desplegar_aplicacion ? module.computo[0].health_endpoints : null
}

# --- I-030: balanceo interno + publicación HTTPS por CloudFront (D2) -------

module "balanceo" {
  count = var.desplegar_aplicacion ? 1 : 0

  source = "../../modules/balanceo"

  proyecto = "nexum"
  entorno  = var.entorno
  region   = var.region
  vpc_id   = module.red.vpc_id

  subnet_ids    = module.red.subredes_privadas
  sg_alb_id     = module.red.security_groups["alb"]
  sg_backend_id = module.red.security_groups["backend"]
  asg_name      = module.computo[0].asg_name

  puerto_backend         = var.puerto_backend
  health_check_path      = var.health_check_path
  cloudfront_price_class = var.cloudfront_price_class
}

output "balanceo" {
  description = "ALB interno, target group, VPC origin y dominio CloudFront"
  value = var.desplegar_aplicacion ? {
    alb_arn           = module.balanceo[0].alb_arn
    alb_es_interno    = module.balanceo[0].alb_es_interno
    target_group_arn  = module.balanceo[0].target_group_arn
    listener          = module.balanceo[0].listener_http_privado
    health_check      = module.balanceo[0].health_check
    vpc_origin_id     = module.balanceo[0].vpc_origin_id
    cloudfront_domain = module.balanceo[0].cloudfront_domain_name
    cloudfront_id     = module.balanceo[0].cloudfront_distribution_id
    certificado       = module.balanceo[0].cloudfront_certificado
    sin_cache         = module.balanceo[0].sin_cache
    estado_despliegue = module.balanceo[0].estado_despliegue
  } : null
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


# ARTIFACT_DELIVERY: nombre determinista del bucket de artefactos. Se replica aqui
# para no crear un ciclo artefactos -> iam -> artefactos al conceder GetObject al
# runtime. Una prueba de contrato verifica que coincide con modules/artefactos.
data "aws_caller_identity" "para_artefactos" {}

locals {
  artefactos_bucket = lower("nexum-${var.entorno}-artefactos-${data.aws_caller_identity.para_artefactos.account_id}")
}

module "identidades" {
  source = "../../modules/identidades_publicacion"

  proyecto                 = "nexum"
  entorno                  = var.entorno
  etiquetas                = var.etiquetas
  cuenta_id                = local.cuenta_id
  principal_operador_arn   = var.principal_operador_arn
  bucket_artefactos_nombre = local.bucket_artefactos_nombre
  bucket_secretos_nombre   = local.bucket_secretos_nombre
  prefijo_artefactos       = "releases/"
  prefijo_secretos         = "nexum/secrets/"
  rol_runtime_arn          = module.iam.runtime_role_arn
}

module "artefactos" {
  source = "../../modules/artefactos"

  proyecto        = "nexum"
  entorno         = var.entorno
  etiquetas       = var.etiquetas
  prefijo         = "releases/"
  rol_runtime_arn = module.iam.runtime_role_arn

  # RFC-011: los ARN se derivan de la cuenta y del nombre determinista del rol.
  # Se pasan como cadena conocida (no como valor "unknown") porque las
  # preconditions de I-060 validan el formato en tiempo de plan; la raíz
  # comprueba después con `check` que coinciden con `aws_iam_role.*.arn`.
  publicador_arn = local.arn_artefactos_publicador

  # La política del bucket deniega `s3:ListBucket` a todos salvo esta identidad
  # (para que `HeadBucket` del refresh no quede denegado y Terraform no crea que
  # el bucket ha desaparecido). La identidad que ejecuta Terraform en esta cuenta
  # es el operador, no un rol OIDC: por eso la exención es el operador real.
  # El rol `secretos-gestor-iac` sigue existiendo para asumirlo en publicaciones.
  gestor_iac_arn = var.principal_operador_arn
}

module "secretos" {
  source = "../../modules/secretos"

  proyecto               = "nexum"
  entorno                = var.entorno
  etiquetas              = var.etiquetas
  rol_runtime_arn        = module.iam.runtime_role_arn
  prefijo                = "nexum/secrets/"
  retencion_version_dias = 30
  publicador_arn         = local.arn_secretos_publicador

  # Misma razón que en el bucket de artefactos: la exención de `ListBucket` debe
  # ser la identidad que ejecuta Terraform (el operador), o el refresh deniega
  # `HeadBucket` y el plan intenta recrear el bucket.
  gestor_iac_arn = var.principal_operador_arn
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
  # Objetos exactos (binario y unidad systemd) sólo se conocen tras publicar el
  # release (I-020/I-080). Un ARN con comodín NO es aceptable aquí: la validación
  # del módulo exige objeto exacto y sin '*'. En la fase base (sin cómputo) la
  # lista queda vacía y el rol runtime no obtiene acceso a artefactos todavía.
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
  count = var.desplegar_aplicacion ? 1 : 0

  source = "../../modules/observabilidad"

  proyecto = "nexum"
  entorno  = var.entorno
  region   = var.region

  alb_arn          = module.balanceo[0].alb_arn
  target_group_arn = module.balanceo[0].target_group_arn

  retencion_dias      = 7
  umbral_5xx          = 5
  umbral_auth_401     = 20
  presupuesto_usd     = 1
  habilitar_dashboard = true
}

output "observabilidad" {
  description = "Logs, metric filters, alarmas, presupuesto y dashboard (sin AWS aplicado)"
  value = var.desplegar_aplicacion ? {
    log_group         = module.observabilidad[0].log_group_backend
    retencion_dias    = module.observabilidad[0].retencion_dias
    namespace         = module.observabilidad[0].namespace_metricas
    notificacion      = module.observabilidad[0].notificacion
    alarmas           = module.observabilidad[0].alarmas
    presupuesto       = module.observabilidad[0].presupuesto
    dashboard         = module.observabilidad[0].dashboard
    estado_despliegue = module.observabilidad[0].estado_despliegue
  } : null
}
