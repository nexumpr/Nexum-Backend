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
  # 1. Ningun acceso sin TLS, para cualquier principal.
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

  # 2. El runtime de la instancia NUNCA escribe ni borra, aunque una policy de
  #    identidad lo concediera por error: el Deny explicito del bucket prevalece.
  statement {
    sid    = "DenegarEscrituraAlRuntime"
    effect = "Deny"

    actions = [
      "s3:PutObject",
      "s3:PutObjectAcl",
      "s3:DeleteObject",
      "s3:DeleteObjectVersion",
    ]
    resources = ["${aws_s3_bucket.secretos.arn}/${var.prefijo}*"]

    principals {
      type        = "AWS"
      identifiers = [var.rol_runtime_arn]
    }
  }

  # 3. El publicador autorizado publica SOLO dentro del prefijo y solo con SSE-S3.
  #    Su permiso efectivo requiere ademas la policy de identidad correspondiente.
  statement {
    sid    = "PermitirPublicacionAlPublicador"
    effect = "Allow"

    actions   = ["s3:PutObject"]
    resources = ["${aws_s3_bucket.secretos.arn}/${var.prefijo}*"]

    principals {
      type        = "AWS"
      identifiers = [var.publicador_arn]
    }

    condition {
      test     = "StringEquals"
      variable = "s3:x-amz-server-side-encryption"
      values   = ["AES256"]
    }
  }
}

resource "aws_s3_bucket_policy" "secretos" {
  bucket = aws_s3_bucket.secretos.id
  policy = data.aws_iam_policy_document.secretos.json

  depends_on = [aws_s3_bucket_public_access_block.secretos]
}

resource "terraform_data" "secretos_preconditions" {
  input = "${var.prefijo}|${var.retencion_version_dias}|${var.rol_runtime_arn}|${var.publicador_arn}"

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

    precondition {
      condition     = can(regex("^arn:aws:iam::[0-9]{12}:(role|user)/.+$", var.publicador_arn))
      error_message = "publicador_arn debe ser un ARN de rol o usuario IAM valido: sin publicador no hay forma autorizada de publicar el bundle."
    }
  }
}
