# E005 - envio de correo de recuperacion con Amazon SES v2.
#
# Alcance minimo: una sola accion (ses:SendEmail) sobre la identidad verificada del
# proyecto, con condicion sobre la direccion remitente. No concede SendRawEmail,
# SendBulkEmail, ni acceso al endpoint SMTP. La conectividad privada desde las EC2 se
# resuelve en el modulo de red cuando su Gate FinOps lo autorice.

variable "ses_sender_identity_arn" {
  description = "ARN de la identidad SES verificada autorizada a enviar (vacio = sin permiso SES)."
  type        = string
  default     = "arn:aws:ses:us-east-2:713797602090:identity/nexumproyectomov@gmail.com"

  validation {
    condition     = var.ses_sender_identity_arn == "" || can(regex("^arn:aws:ses:[a-z0-9-]+:[0-9]{12}:identity/.+$", var.ses_sender_identity_arn))
    error_message = "ses_sender_identity_arn debe ser un ARN de identidad SES o cadena vacia."
  }
}

variable "ses_from_email" {
  description = "Direccion remitente que debe coincidir con la identidad verificada."
  type        = string
  default     = "nexumproyectomov@gmail.com"

  validation {
    condition     = var.ses_from_email == "" || can(regex("^[^@\\s]+@[^@\\s]+\\.[a-z]{2,}$", var.ses_from_email))
    error_message = "ses_from_email debe ser una direccion de correo valida o cadena vacia."
  }
}

# SES v2 autoriza SendEmail contra la identidad del remitente y, en sandbox,
# tambien contra la identidad del destinatario verificado. El recurso se limita
# al espacio de identidades de la cuenta; la condicion ses:FromAddress mantiene
# el remitente fijo y aws:SecureTransport exige HTTPS.
locals {
  ses_identity_parts  = split("/", var.ses_sender_identity_arn)
  ses_identity_prefix = join("/", slice(local.ses_identity_parts, 0, length(local.ses_identity_parts) - 1))
}

resource "aws_iam_role_policy" "runtime_ses" {
  count = var.ses_sender_identity_arn == "" ? 0 : 1

  name = "nexum-${var.entorno}-runtime-ses"
  role = aws_iam_role.runtime.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid      = "SendPasswordResetEmailOnly"
        Effect   = "Allow"
        Action   = ["ses:SendEmail"]
        Resource = "${local.ses_identity_prefix}/*"
        Condition = {
          StringEquals = {
            "ses:FromAddress" = var.ses_from_email
          }
          Bool = {
            "aws:SecureTransport" = "true"
          }
        }
      },
    ]
  })
}
