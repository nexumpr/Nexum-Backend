# I-040 — DynamoDB: tabla única NEXUM_CORE_<env>.
#
# Modelo físico ratificado en H05 (`sdd/02-infraestructura/MATRIZ_FISICA_DYNAMODB_E001_E075.md`)
# y en A-060, ya implementado en Go (single-table design):
#   PK  (S)      USER#<id> | EMAIL#<sha256(email)> | SESSION#<hash>
#   SK  (S)      PROFILE | MOVEMENT#<utc>#<id> | GOAL#<id> | ...
#   GSI1PK/GSI1SK (sparse)  USER#<id>#TYPE#<movType> / <utc>#<movementId>
#   GSI2PK/GSI2SK (sparse)  DUE#<yyyy-mm-dd>#<shard> / <nextDueAt>#<reminderId>
#   expiresAt (N)           TTL eventual de tokens y elementos no críticos
#
# Decisiones y límites:
#  - PAY_PER_REQUEST (INF-040-01); sin capacidad provisionada que pueda quedar ociosa.
#  - SSE con clave administrada por AWS: cifrado en reposo sin coste de KMS propio.
#  - PITR habilitado porque I-000 lo presupuestó (DDB_PITR 3 GB-mes); backups
#    programados pertenecen a I-130 y NO se crean aquí.
#  - GSI1 está justificado por el listado de movimientos por tipo (E027–E031).
#  - GSI2 sólo se crea si `habilitar_gsi2 = true`: la matriz lo reserva para
#    «workers programados confirmados» y hoy no existe scheduler implementado;
#    crearlo por defecto sería un GSI especulativo (prohibido por INF-040-07).
#  - Sin streams (coste y consumidores no aprobados) y sin tabla auxiliar de
#    identidad: la unicidad de email vive en NEXUM_CORE vía ítem reservado.
#  - Nada de esto se aplica en I-040: `terraform plan/apply` están denegados.

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
  nombre_tabla = "${var.nombre_tabla}_${upper(var.entorno)}"

  atributos = concat(
    [
      { name = "PK", type = "S" },
      { name = "SK", type = "S" },
      { name = "GSI1PK", type = "S" },
      { name = "GSI1SK", type = "S" },
    ],
    var.habilitar_gsi2 ? [
      { name = "GSI2PK", type = "S" },
      { name = "GSI2SK", type = "S" },
    ] : []
  )

  indices = concat(
    [
      {
        name               = "GSI1"
        hash_key           = "GSI1PK"
        range_key          = "GSI1SK"
        projection_type    = "ALL"
        non_key_attributes = []
      },
    ],
    var.habilitar_gsi2 ? [
      {
        name               = "GSI2"
        hash_key           = "GSI2PK"
        range_key          = "GSI2SK"
        projection_type    = "INCLUDE"
        non_key_attributes = ["status", "dueAt", "entityType"]
      },
    ] : []
  )
}

resource "terraform_data" "validacion_tabla" {
  input = {
    tabla   = local.nombre_tabla
    indices = [for i in local.indices : i.name]
    ttl     = var.atributo_ttl
    gsi2    = var.habilitar_gsi2
  }

  lifecycle {
    precondition {
      condition     = length(distinct([for a in local.atributos : a.name])) == length(local.atributos)
      error_message = "Un atributo de clave no puede declararse dos veces en DynamoDB."
    }

    precondition {
      condition     = var.habilitar_gsi2 == contains([for i in local.indices : i.name], "GSI2")
      error_message = "GSI2 sólo puede existir cuando habilitar_gsi2 = true (prohibidos GSI especulativos)."
    }

    precondition {
      condition     = var.atributo_ttl == "expiresAt"
      error_message = "La matriz física H05 fija el atributo TTL en `expiresAt`; otro nombre rompería A-060."
    }
  }
}

resource "aws_dynamodb_table" "core" {
  name         = local.nombre_tabla
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "PK"
  range_key    = "SK"

  dynamic "attribute" {
    for_each = local.atributos
    content {
      name = attribute.value.name
      type = attribute.value.type
    }
  }

  dynamic "global_secondary_index" {
    for_each = local.indices
    content {
      name               = global_secondary_index.value.name
      hash_key           = global_secondary_index.value.hash_key
      range_key          = global_secondary_index.value.range_key
      projection_type    = global_secondary_index.value.projection_type
      non_key_attributes = length(global_secondary_index.value.non_key_attributes) > 0 ? global_secondary_index.value.non_key_attributes : null
    }
  }

  # TTL eventual: nunca sustituye la comprobación de expiración en la aplicación
  # (INF-040-06, A-060 §8).
  ttl {
    attribute_name = var.atributo_ttl
    enabled        = true
  }

  # Cifrado en reposo con clave administrada por AWS (sin coste de KMS propio).
  server_side_encryption {
    enabled = true
  }

  # DDB_PITR está presupuestado en I-000; la copia programada es de I-130.
  point_in_time_recovery {
    enabled = var.pitr_habilitado
  }

  # Sin streams: no hay consumidor aprobado y cada stream se factura aparte.
  stream_enabled = false

  # Teardown del plan Free: en h08i se deja desactivado para poder destruir con
  # un único `destroy` autorizado; en entornos persistentes debe activarse.
  deletion_protection_enabled = var.proteccion_borrado

  tags = merge(
    {
      Name = local.nombre_tabla
      Spec = "I-040-dynamodb"
    },
    var.etiquetas
  )
}
