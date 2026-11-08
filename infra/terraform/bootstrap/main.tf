# I-070 — Bootstrap del backend remoto de Terraform.
#
# AWS_NOT_RUN. Este root NO se ha aplicado: sólo declara el bucket de state y la
# tabla de lock. Se ejecuta con credenciales humanas en un Gate de infraestructura
# independiente, nunca desde CI de PR.
#
# Decisiones y límites:
#  - State cifrado con SSE-S3 (AES256): evita el coste fijo de una CMK de KMS, que
#    no está presupuestado en I-000. Se documenta el límite: sin CMK no hay
#    revocación criptográfica inmediata.
#  - Versionado + lifecycle de versiones no actuales: permite recuperar un state
#    sobrescrito sin acumular coste indefinido.
#  - Bloqueo público total y política que exige TLS.
#  - Locking con tabla DynamoDB on-demand (`LockID`), PITR y backups desactivados:
#    la tabla sólo guarda locks efímeros.
#  - `prevent_destroy` en bucket y tabla: el state no se destruye por ahorro.
#  - State real jamás en Git: `.gitignore` excluye `*.tfstate*` y `.terraform/`.

terraform {
  required_version = ">= 1.4.0, < 2.0.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.72"
    }
  }

  # Sin bloque backend: este root usa state local sólo durante el bootstrap
  # humano y nunca se commitea. El state del runtime vive en el bucket creado aquí.
}

locals {
  nombre_bucket = coalesce(var.bucket_state_nombre, "${var.proyecto}-${var.entorno}-tfstate-${var.sufijo_cuenta}")
  nombre_lock   = "${var.proyecto}-${var.entorno}-tf-lock"
}

resource "terraform_data" "validacion_bootstrap" {
  input = {
    bucket = local.nombre_bucket
    lock   = local.nombre_lock
    region = var.region
  }

  lifecycle {
    precondition {
      condition     = can(regex("^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$", local.nombre_bucket))
      error_message = "El bucket de state debe cumplir el formato DNS de S3 (minúsculas, 3-63 caracteres)."
    }

    precondition {
      condition     = var.region == "us-east-2"
      error_message = "I-000/I-010 fijan us-east-2; otra región exige recotización."
    }

    precondition {
      condition     = var.sufijo_cuenta != "" && !startswith(var.sufijo_cuenta, "PENDIENTE")
      error_message = "El sufijo de cuenta debe ser un identificador real aportado por el operador, no un marcador."
    }
  }
}

resource "aws_s3_bucket" "state" {
  bucket = local.nombre_bucket

  # El state no se destruye por conveniencia ni por ahorro (INF-070-04).
  lifecycle {
    prevent_destroy = true
  }

  tags = merge({
    Name = local.nombre_bucket
    Spec = "I-070-terraform"
  }, var.etiquetas)
}

resource "aws_s3_bucket_versioning" "state" {
  bucket = aws_s3_bucket.state.id

  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "state" {
  bucket = aws_s3_bucket.state.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
    bucket_key_enabled = false
  }
}

resource "aws_s3_bucket_public_access_block" "state" {
  bucket = aws_s3_bucket.state.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_ownership_controls" "state" {
  bucket = aws_s3_bucket.state.id

  rule {
    object_ownership = "BucketOwnerEnforced"
  }
}

resource "aws_s3_bucket_policy" "state" {
  bucket = aws_s3_bucket.state.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid       = "DenyInsecureTransport"
      Effect    = "Deny"
      Principal = "*"
      Action    = "s3:*"
      Resource = [
        aws_s3_bucket.state.arn,
        "${aws_s3_bucket.state.arn}/*",
      ]
      Condition = {
        Bool = {
          "aws:SecureTransport" = "false"
        }
      }
    }]
  })
}

resource "aws_s3_bucket_lifecycle_configuration" "state" {
  bucket = aws_s3_bucket.state.id

  rule {
    id     = "expirar-versiones-no-actuales"
    status = "Enabled"

    filter {}

    noncurrent_version_expiration {
      noncurrent_days = var.dias_retencion_versiones
    }

    abort_incomplete_multipart_upload {
      days_after_initiation = 7
    }
  }
}

# Tabla de locking de Terraform (state locking). No es la tabla de negocio
# NEXUM_CORE de I-040: sólo almacena el atributo LockID.
resource "aws_dynamodb_table" "lock" {
  name         = local.nombre_lock
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "LockID"

  attribute {
    name = "LockID"
    type = "S"
  }

  # Sin PITR ni backups: los locks son efímeros y no contienen datos de negocio.
  point_in_time_recovery {
    enabled = false
  }

  server_side_encryption {
    enabled = true
  }

  lifecycle {
    prevent_destroy = true
  }

  tags = merge({
    Name = local.nombre_lock
    Spec = "I-070-terraform"
  }, var.etiquetas)
}
