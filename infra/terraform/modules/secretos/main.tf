# Modulo `secretos` (RFC-004 opcion B, aprobada 2026-11-09).
#
# Bundle de configuracion en un bucket privado con SSE-S3, versionado y lifecycle.
# El objeto lo publica el operador fuera de banda; Terraform NO crea secretos y el
# repositorio no contiene ninguno. La instancia lo lee por el gateway endpoint de S3
# (sin NAT ni salida a Internet) con un rol limitado a GetObject sobre el prefijo.
#
# Limite declarado: sin CMK no hay revocacion criptografica inmediata ni rotacion
# automatica; la rotacion es operativa (nueva version + instance_refresh).

data "aws_caller_identity" "actual" {}

locals {
  nombre_bucket = lower("${var.proyecto}-${var.entorno}-secretos-${data.aws_caller_identity.actual.account_id}")
}

resource "aws_s3_bucket" "secretos" {
  bucket        = local.nombre_bucket
  force_destroy = false

  tags = merge(var.etiquetas, {
    Componente = "secretos-bootstrap"
    RFC        = "RFC-004"
  })
}

resource "aws_s3_bucket_public_access_block" "secretos" {
  bucket = aws_s3_bucket.secretos.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_ownership_controls" "secretos" {
  bucket = aws_s3_bucket.secretos.id

  rule {
    object_ownership = "BucketOwnerEnforced"
  }
}

resource "aws_s3_bucket_versioning" "secretos" {
  bucket = aws_s3_bucket.secretos.id

  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "secretos" {
  bucket = aws_s3_bucket.secretos.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
    bucket_key_enabled = false
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "secretos" {
  bucket = aws_s3_bucket.secretos.id

  rule {
    id     = "expirar-versiones-no-actuales"
    status = "Enabled"

    filter {}

    noncurrent_version_expiration {
      noncurrent_days = var.retencion_version_dias
    }

    abort_incomplete_multipart_upload {
      days_after_initiation = 1
    }
  }
}

data "aws_iam_policy_document" "secretos" {
  statement {
    sid    = "DenegarSinTLS"
    effect = "Deny"

    actions   = ["s3:*"]
    resources = [aws_s3_bucket.secretos.arn, "${aws_s3_bucket.secretos.arn}/*"]

    principals {
      type        = "*"
      identifiers = ["*"]
    }

    condition {
      test     = "Bool"
      variable = "aws:SecureTransport"
      values   = ["false"]
    }
  }

  statement {
    sid    = "DenegarFueraDelPrefijo"
    effect = "Deny"

    actions   = ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"]
    resources = ["${aws_s3_bucket.secretos.arn}/*"]

    principals {
      type        = "*"
      identifiers = ["*"]
    }

    condition {
      test     = "StringNotLike"
      variable = "aws:PrincipalArn"
      values   = [var.rol_runtime_arn == "" ? "arn:aws:iam::*:role/inexistente" : var.rol_runtime_arn]
    }
  }
}

resource "aws_s3_bucket_policy" "secretos" {
  bucket = aws_s3_bucket.secretos.id
  policy = data.aws_iam_policy_document.secretos.json

  depends_on = [aws_s3_bucket_public_access_block.secretos]
}

resource "terraform_data" "secretos_preconditions" {
  input = "${var.prefijo}|${var.retencion_version_dias}|${var.rol_runtime_arn}"

  lifecycle {
    precondition {
      condition     = endswith(var.prefijo, "/")
      error_message = "El prefijo de secretos debe terminar en '/'."
    }

    precondition {
      condition     = var.retencion_version_dias >= 1 && var.retencion_version_dias <= 365
      error_message = "La retencion de versiones no actuales debe estar entre 1 y 365 dias."
    }

    precondition {
      condition     = can(regex("^arn:aws:iam::[0-9]{12}:role/.+$", var.rol_runtime_arn))
      error_message = "rol_runtime_arn debe ser un ARN de rol valido: solo ese rol puede leer el prefijo."
    }
  }
}
