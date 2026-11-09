# Modulo `artefactos` (I-020/I-060/I-090, Gate ARTIFACT_DELIVERY 2026-11-09).
#
# Bucket privado para binarios de release y su SBOM. El binario lo publica el rol de
# CI autorizado (no el runtime, que solo descarga); las instancias lo leen por el
# gateway endpoint de S3 con el rol runtime ya existente.
#
# No contiene secretos: el bundle de configuracion vive en `modules/secretos`.

data "aws_caller_identity" "actual" {}

locals {
  nombre_bucket = lower("${var.proyecto}-${var.entorno}-artefactos-${data.aws_caller_identity.actual.account_id}")
}

resource "aws_s3_bucket" "artefactos" {
  bucket        = local.nombre_bucket
  force_destroy = false

  tags = merge(var.etiquetas, {
    Componente = "artefactos-release"
    Gate       = "ARTIFACT_DELIVERY"
  })
}

resource "aws_s3_bucket_public_access_block" "artefactos" {
  bucket = aws_s3_bucket.artefactos.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_ownership_controls" "artefactos" {
  bucket = aws_s3_bucket.artefactos.id

  rule {
    object_ownership = "BucketOwnerEnforced"
  }
}

resource "aws_s3_bucket_versioning" "artefactos" {
  bucket = aws_s3_bucket.artefactos.id

  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "artefactos" {
  bucket = aws_s3_bucket.artefactos.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
    bucket_key_enabled = false
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "artefactos" {
  bucket = aws_s3_bucket.artefactos.id

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

data "aws_iam_policy_document" "artefactos" {
  # 1. Sin TLS no hay acceso, para cualquier principal.
  statement {
    sid    = "DenegarSinTLS"
    effect = "Deny"

    actions   = ["s3:*"]
    resources = [aws_s3_bucket.artefactos.arn, "${aws_s3_bucket.artefactos.arn}/*"]

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

  # 2. El runtime descarga, nunca publica ni borra.
  statement {
    sid    = "DenegarEscrituraAlRuntime"
    effect = "Deny"

    actions = [
      "s3:PutObject",
      "s3:PutObjectAcl",
      "s3:DeleteObject",
      "s3:DeleteObjectVersion",
    ]
    resources = ["${aws_s3_bucket.artefactos.arn}/*"]

    principals {
      type        = "AWS"
      identifiers = [var.rol_runtime_arn]
    }
  }

  # 3. El publicador de CI solo escribe, y solo en el prefijo de releases.
  statement {
    sid    = "PermitirPublicacionAlPublicador"
    effect = "Allow"

    actions   = ["s3:PutObject"]
    resources = ["${aws_s3_bucket.artefactos.arn}/${var.prefijo}*"]

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

  # 4. Nadie mas toca los objetos del bucket, aunque su IAM sea amplio.
  statement {
    sid    = "DenegarAPrincipalsNoAutorizados"
    effect = "Deny"

    actions = [
      "s3:GetObject",
      "s3:GetObjectVersion",
      "s3:PutObject",
      "s3:PutObjectAcl",
      "s3:DeleteObject",
      "s3:DeleteObjectVersion",
    ]
    resources = ["${aws_s3_bucket.artefactos.arn}/*"]

    principals {
      type        = "*"
      identifiers = ["*"]
    }

    condition {
      test     = "StringNotEquals"
      variable = "aws:PrincipalArn"
      values   = [var.rol_runtime_arn, var.publicador_arn]
    }
  }

  # 5. ListBucket solo para el rol gestor de IaC (HeadBucket en el refresh).
  statement {
    sid    = "DenegarListBucket"
    effect = "Deny"

    actions   = ["s3:ListBucket"]
    resources = ["${aws_s3_bucket.artefactos.arn}"]

    principals {
      type        = "*"
      identifiers = ["*"]
    }

    condition {
      test     = "ArnNotEquals"
      variable = "aws:PrincipalArn"
      values   = [var.gestor_iac_arn]
    }
  }

  # 6. Versiones y multipart quedan denegados para todos.
  statement {
    sid    = "DenegarListadoDeVersionesYMultipart"
    effect = "Deny"

    actions = [
      "s3:ListBucketVersions",
      "s3:ListBucketMultipartUploads",
    ]
    resources = ["${aws_s3_bucket.artefactos.arn}"]

    principals {
      type        = "*"
      identifiers = ["*"]
    }
  }
}

resource "aws_s3_bucket_policy" "artefactos" {
  bucket = aws_s3_bucket.artefactos.id
  policy = data.aws_iam_policy_document.artefactos.json

  depends_on = [aws_s3_bucket_public_access_block.artefactos]
}

resource "terraform_data" "artefactos_preconditions" {
  input = "${var.prefijo}|${var.rol_runtime_arn}|${var.publicador_arn}|${var.gestor_iac_arn}"

  lifecycle {
    precondition {
      condition     = endswith(var.prefijo, "/")
      error_message = "El prefijo de artefactos debe terminar en '/'."
    }

    precondition {
      condition     = can(regex("^arn:aws:iam::[0-9]{12}:role/.+$", var.rol_runtime_arn))
      error_message = "rol_runtime_arn debe ser un ARN de rol IAM valido."
    }

    precondition {
      condition     = can(regex("^arn:aws:iam::[0-9]{12}:role/.+$", var.publicador_arn))
      error_message = "publicador_arn debe ser un ARN de rol IAM: sin publicador no hay entrega de artefactos."
    }

    precondition {
      condition     = can(regex("^arn:aws:iam::[0-9]{12}:role/.+$", var.gestor_iac_arn))
      error_message = "gestor_iac_arn debe ser un ARN de rol IAM para permitir HeadBucket."
    }

    precondition {
      condition     = length(distinct([var.rol_runtime_arn, var.publicador_arn, var.gestor_iac_arn])) == 3
      error_message = "Runtime, publicador y gestor IaC deben ser tres identidades distintas."
    }

    precondition {
      condition     = var.retencion_version_dias >= 1 && var.retencion_version_dias <= 3650
      error_message = "La retencion de versiones no actuales debe estar entre 1 y 3650 dias."
    }
  }
}
