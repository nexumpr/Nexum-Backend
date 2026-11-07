# I-050 — Redis OSS compartido (ElastiCache) para las dos instancias Go.
#
# Alternativa implementada (ver `sdd/_control/H08/ADR_REDIS_PROVEEDOR.md`):
# ElastiCache **Redis OSS** node-based, 1 nodo `cache.t4g.micro`, sin réplicas,
# tal como queda trazado en I-000 (SKU `VERIFIED_AWS` 0.016 USD/nodo-hora,
# escenario C: 1080 nodo-hora = 17.2800 USD/45 d).
#
# Requisitos aplicados (INF-050-01..07, A-080):
#  - Servicio administrado Redis OSS (nunca Valkey ni serverless sin ADR).
#  - Red privada: subnet group con las 2 subredes privadas de I-010 y el SG
#    `redis` de I-010, que sólo admite 6379 desde el SG del backend.
#  - Cifrado en tránsito (TLS) + en reposo, y usuario RBAC dedicado con
#    contraseña rotable que **nunca** se versiona (llega por variable sensible).
#  - Política de evicción `volatile-lru` en un parameter group `redis7`: sólo se
#    expulsan claves con TTL, nunca datos sin expiración.
#  - Sin snapshots (`snapshot_retention_limit = 0`): Redis es caché y DynamoDB es
#    la fuente de verdad (INF-050-05); no se paga retención de un dato recuperable.
#  - Un solo nodo = punto único de fallo declarado, no alta disponibilidad.
#
# Nada de esto se aplica en I-050: `terraform plan/apply` están denegados.

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
  nombre   = "${var.proyecto}-${var.entorno}"
  subredes = [for az in sort(keys(var.subnet_ids)) : var.subnet_ids[az]]
}

resource "terraform_data" "validacion_redis" {
  input = {
    nodos    = var.num_nodos
    subredes = length(local.subredes)
    motor    = var.motor
    tls      = var.cifrado_en_transito
  }

  lifecycle {
    precondition {
      condition     = length(local.subredes) == 2
      error_message = "El subnet group de ElastiCache exige las 2 subredes privadas de I-010 (GOV-02)."
    }

    precondition {
      condition     = var.sg_redis_id != var.sg_backend_id
      error_message = "El SG de Redis y el del backend deben ser distintos (mínimo privilegio)."
    }

    precondition {
      condition     = var.motor == "redis"
      error_message = "El SDD exige Redis OSS; Valkey u otro motor requiere ADR humano explícito."
    }

    precondition {
      condition     = var.cifrado_en_transito && var.cifrado_en_reposo
      error_message = "INF-050-03 exige TLS en tránsito y cifrado en reposo; no son opcionales."
    }

    precondition {
      condition     = var.habilitar_failover == (var.num_nodos > 1)
      error_message = "El failover automático sólo puede activarse con más de un nodo; una sola caché no es HA."
    }

    precondition {
      condition     = var.num_nodos == 1
      error_message = "I-000 presupuesta exactamente 1 nodo; añadir réplicas exige recotización y Gate."
    }

    precondition {
      condition     = length(var.password_aplicacion) >= 16
      error_message = "La contraseña RBAC debe tener al menos 16 caracteres y no se versiona (RFC-004 en despliegue)."
    }

    precondition {
      condition     = var.sg_redis_id != var.sg_backend_id && var.cifrado_en_transito
      error_message = "Redis debe ser privado y usar TLS; el usuario default sustituido no debe quedar accesible."
    }
  }
}

resource "aws_elasticache_subnet_group" "redis" {
  name        = "${local.nombre}-redis-subredes"
  description = "Subredes privadas de I-010 para Redis OSS compartido"
  subnet_ids  = local.subredes

  tags = merge(
    {
      Name = "${local.nombre}-redis-subredes"
      Spec = "I-050-redis-distribuido"
    },
    var.etiquetas
  )
}

# Sólo se expulsan claves con TTL: la caché no puede quedarse sin espacio y
# expulsar datos sin expiración (A-080: claves con versión + TTL).
resource "aws_elasticache_parameter_group" "redis" {
  name        = "${local.nombre}-redis7-cache-ttl"
  family      = "redis7"
  description = "Politica de cache Nexum (eviccion solo de claves con TTL)"

  parameter {
    name  = "maxmemory-policy"
    value = "volatile-lru"
  }

  tags = merge(
    {
      Name = "${local.nombre}-redis7-cache-ttl"
      Spec = "I-050-redis-distribuido"
    },
    var.etiquetas
  )
}

# Usuario RBAC dedicado (INF-050-03): sin usuario por defecto, sin comandos
# administrativos ni peligrosos, y limitado al espacio de claves `nexum:` de A-080.
#
# Política: se parte de `-@all` y se conceden sólo las categorías que A-080 exige
# (lecturas, escrituras, TTL/keyspace, scripts Lua para rate limiting atómico,
# transacciones y comandos de conexión). Se bloquean además KEYS/SCAN/RANDOMKEY,
# que permiten recorrer el espacio de claves sin necesidad funcional.
resource "aws_elasticache_user" "aplicacion" {
  user_id       = "${var.proyecto}-app"
  user_name     = "${var.proyecto}-app"
  engine        = "redis"
  access_string = "on ~nexum:* -@all +@read +@write +@keyspace +@scripting +@transactions +@connection -keys -scan -randomkey"

  authentication_mode {
    type      = "password"
    passwords = [var.password_aplicacion]
  }

  tags = merge(
    {
      Name = "${var.proyecto}-redis-app"
      Spec = "I-050-redis-distribuido"
    },
    var.etiquetas
  )
}

# ElastiCache **añade automáticamente** un usuario `default` con `on ~* +@all` a
# todos los grupos de usuarios y no permite borrarlo (RBAC, doc oficial). No basta
# con no crearlo: hay que SUSTITUIRLO creando otro usuario cuyo `user_name` sea
# `default`, deshabilitado (`off ~* -@all`) y sin contraseña, y añadirlo al grupo.
resource "aws_elasticache_user" "default_deshabilitado" {
  user_id       = "${var.proyecto}-default-off"
  user_name     = "default"
  engine        = "redis"
  access_string = "off ~* -@all"

  authentication_mode {
    type = "no-password-required"
  }

  tags = merge(
    {
      Name = "${var.proyecto}-redis-default-off"
      Spec = "I-050-redis-distribuido"
    },
    var.etiquetas
  )
}

resource "aws_elasticache_user_group" "aplicacion" {
  engine        = "redis"
  user_group_id = "${var.proyecto}-app"

  # El grupo contiene exactamente dos identidades: la aplicación (con contraseña)
  # y el `default` deshabilitado que sustituye al permisivo automático.
  user_ids = [
    aws_elasticache_user.aplicacion.user_id,
    aws_elasticache_user.default_deshabilitado.user_id,
  ]

  tags = merge(
    {
      Name = "${var.proyecto}-redis-app-group"
      Spec = "I-050-redis-distribuido"
    },
    var.etiquetas
  )
}

resource "aws_elasticache_replication_group" "redis" {
  replication_group_id = "${local.nombre}-redis"
  description          = "Redis OSS compartido por las 2 instancias Go (cache y rate limiting)"

  engine         = var.motor
  engine_version = var.version_motor
  node_type      = var.tipo_nodo
  port           = var.puerto

  # Un único nodo compartido: punto único de fallo declarado (INF-050-02).
  num_cache_clusters         = var.num_nodos
  automatic_failover_enabled = var.habilitar_failover
  multi_az_enabled           = var.habilitar_failover

  subnet_group_name    = aws_elasticache_subnet_group.redis.name
  security_group_ids   = [var.sg_redis_id]
  parameter_group_name = aws_elasticache_parameter_group.redis.name

  # Autenticación gestionada por RBAC; sin `auth_token` en texto en el grupo.
  user_group_ids = [aws_elasticache_user_group.aplicacion.user_group_id]

  at_rest_encryption_enabled = var.cifrado_en_reposo
  transit_encryption_enabled = var.cifrado_en_transito

  # Sin backups: Redis es caché y DynamoDB es la fuente de verdad (INF-050-05).
  snapshot_retention_limit = 0

  apply_immediately          = false
  auto_minor_version_upgrade = true

  tags = merge(
    {
      Name = "${local.nombre}-redis"
      Spec = "I-050-redis-distribuido"
    },
    var.etiquetas
  )
}
