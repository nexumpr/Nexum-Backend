# Permite preparar red, IAM, buckets, DynamoDB y Redis sin lanzar instancias
# que aún carecen de binario, unidad systemd y bundle de configuración.
# En producción se activa solo después de publicar y verificar los artefactos.
variable "desplegar_aplicacion" {
  description = "true: 2 EC2 + ALB + CloudFront + observabilidad; false: solo infraestructura base"
  type        = bool
  default     = false
}

variable "entorno" {
  description = "Entorno lógico"
  type        = string
  default     = "h08i"
}

variable "region" {
  description = "Región AWS"
  type        = string
  default     = "us-east-2"
}

variable "cidr_vpc" {
  description = "CIDR RFC1918 de la VPC"
  type        = string
  default     = "10.42.0.0/16"
}

variable "bits_subredes" {
  description = "Bits para derivar las subredes privadas del CIDR de la VPC"
  type        = number
  default     = 8
}

variable "zonas" {
  description = "Zonas de disponibilidad de la región"
  type        = list(string)
  default     = ["us-east-2a", "us-east-2b"]
}

variable "puerto_backend" {
  description = "Puerto interno del backend Go"
  type        = number
  default     = 8080
}

variable "ventana_mantenimiento_activa" {
  description = "Activa los interface endpoints de mantenimiento (Secrets Manager, SSM)"
  type        = bool
  default     = false
}

variable "habilitar_endpoint_ses" {
  description = "Crea el interface endpoint de SES API v2 (email) para E005; 0.02 USD/h (2 AZ). Activado en produccion para que los planes futuros no propongan destruirlo."
  type        = bool
  default     = true
}

variable "etiquetas" {
  description = "Etiquetas adicionales para default_tags"
  type        = map(string)
  default     = {}
}

# --- Observabilidad FCM/scheduler (deshabilitada por defecto) ---------------

variable "observabilidad_fcm_habilitar_metricas" {
  description = "Crea los metric filters FCM/scheduler en I-110; false por defecto."
  type        = bool
  default     = false
}

variable "observabilidad_fcm_habilitar_alarmas" {
  description = "Crea las alarmas FCM/scheduler en I-110; false por defecto. Exige métricas habilitadas."
  type        = bool
  default     = false
}

variable "observabilidad_version_esperada" {
  description = "Versión efectiva esperada (vX.Y.Z+sha12) para el filtro de versión inesperada; vacío = no se crea."
  type        = string
  default     = ""

  validation {
    # 11 o 12 hex: la política de releases fija `<sha12>` para releases nuevos,
    # pero el release desplegado `v0.20.0+4bf6758d372` se publicó con 11
    # caracteres. Aceptar ambos evita que el filtro de versión sea inutilizable
    # contra la versión real sin relajar la forma SemVer+sha.
    condition     = var.observabilidad_version_esperada == "" || can(regex("^v[0-9]+\\.[0-9]+\\.[0-9]+\\+[0-9a-f]{11,12}$", var.observabilidad_version_esperada))
    error_message = "observabilidad_version_esperada debe estar vacío o tener la forma vX.Y.Z+<sha12>."
  }
}

# --- Egreso FCM/OAuth: NAT Gateway 1 AZ (deshabilitado por defecto) ---------

variable "habilitar_egreso_fcm" {
  description = <<-EOT
    false por defecto: no se crea NAT, EIP, subred pública, ruta ni regla de
    egreso. Al pasar a true, `module.egreso_fcm` sale de count=0 y aplica la
    topología NAT 1 AZ con el IGW existente de I-010. Exige Gate FinOps y
    Seguridad; no es una autorización de gasto por sí misma.
  EOT
  type        = bool
  default     = false
}

variable "egreso_fcm_az" {
  description = "AZ única del NAT Gateway y de su subred pública dedicada."
  type        = string
  default     = "us-east-2a"

  validation {
    condition     = can(regex("^[a-z]{2}-[a-z]+-[0-9][a-z]$", var.egreso_fcm_az))
    error_message = "egreso_fcm_az debe tener forma de zona de disponibilidad, p. ej. us-east-2a."
  }
}

variable "egreso_fcm_cidr_publica" {
  description = <<-EOT
    CIDR de la subred pública nueva del NAT. Por defecto 10.42.16.0/24, fuera
    de las subredes privadas 10.42.0.0/24 y 10.42.1.0/24 de I-010. Debe ser
    /27 o mayor y no solaparse con las subredes existentes.
  EOT
  type        = string
  default     = "10.42.16.0/24"

  validation {
    condition     = can(cidrhost(var.egreso_fcm_cidr_publica, 0))
    error_message = "egreso_fcm_cidr_publica debe ser un bloque IPv4 válido."
  }
}

variable "egreso_fcm_cidr_https" {
  description = <<-EOT
    Destino IPv4 del egreso TCP 443 cuando no se aporta una prefix list real de
    Google. No existe prefix list administrada por AWS para FCM/OAuth; el
    valor por defecto 0.0.0.0/0 es el único honesto si no hay lista propia.
  EOT
  type        = string
  default     = "0.0.0.0/0"

  validation {
    condition     = can(cidrhost(var.egreso_fcm_cidr_https, 0))
    error_message = "egreso_fcm_cidr_https debe ser un bloque IPv4 válido."
  }
}

variable "egreso_fcm_prefix_list_id" {
  description = "Opcional: ID de una prefix list REAL de rangos de Google; vacío = se usa el CIDR."
  type        = string
  default     = ""

  validation {
    condition     = var.egreso_fcm_prefix_list_id == "" || can(regex("^pl-[0-9a-f]+$", var.egreso_fcm_prefix_list_id))
    error_message = "egreso_fcm_prefix_list_id debe estar vacío o ser un ID pl-* existente."
  }
}

# --- I-020: cómputo ---------------------------------------------------------

variable "root_volume_gb" {
  description = "Volumen raíz por instancia (presupuesto: 15 GB x2)"
  type        = number
  default     = 15
}

variable "artefacto_s3_uri" {
  description = "URI S3 del binario linux/arm64"
  type        = string
  default     = "s3://PENDIENTE-I080/nexum-backend"
}

variable "artefacto_sha256" {
  description = "SHA-256 del binario (lo emite scripts/build-backend.ps1)"
  type        = string
  default     = ""
}

variable "unidad_s3_uri" {
  description = "URI S3 de la unidad systemd"
  type        = string
  default     = "s3://PENDIENTE-I080/nexum-backend.service"
}

variable "unidad_sha256" {
  description = "SHA-256 de la unidad systemd"
  type        = string
  default     = ""
}

# --- I-030: balanceo y publicación HTTPS --------------------------------

variable "health_check_path" {
  description = "Ruta de liveness del target group (A-090); readiness queda excluida por 503 de diseño"
  type        = string
  default     = "/api/v1/health/live"
}

variable "cloudfront_price_class" {
  description = "Price class de CloudFront; PriceClass_100 es la más barata"
  type        = string
  default     = "PriceClass_100"
}

# --- I-040: DynamoDB ---------------------------------------------------------

variable "dynamodb_nombre_tabla" {
  description = "Nombre base de la tabla única de A-060 (el módulo añade el entorno)"
  type        = string
  default     = "NEXUM_CORE"
}

variable "dynamodb_atributo_ttl" {
  description = "Atributo TTL epoch seconds fijado por la matriz física H05"
  type        = string
  default     = "expiresAt"
}

variable "dynamodb_pitr_habilitado" {
  description = "PITR presupuestado en I-000 (DDB_PITR)"
  type        = bool
  default     = true
}

variable "dynamodb_proteccion_borrado" {
  description = "Deletion protection activa (H09-90). Un teardown autorizado exige ponerla en false con Gate explicito."
  type        = bool
  default     = true
}

variable "dynamodb_habilitar_gsi2" {
  description = "GSI2 para workers programados (E057–E060); desactivado si no hay scheduler confirmado"
  type        = bool
  default     = false
}

# --- I-050: Redis OSS compartido --------------------------------------------

variable "redis_password_aplicacion" {
  description = <<-EOT
    Contraseña del usuario RBAC de Redis. Sensible y sin valor por defecto: en
    I-050 no hay despliegue y ningún secreto se versiona. En despliegue llegará
    por el mecanismo aprobado (RFC-004, aún PROPUESTO) o por variable de entorno
    fuera del repositorio.
  EOT
  type        = string
  sensitive   = true
  default     = ""
}

# --- I-060: IAM / OIDC ------------------------------------------------------

# La lista efectiva se inyecta desde el tfvars de custodia (fuera del repositorio).
# `ARTEFACTOS_RELEASE.md` documenta los ARN exactos que deben conservarse para el
# release vigente y para el rollback. En particular, `v0.19.2+3098f379d7ab` estuvo
# desplegado y debe seguir autorizado aunque ya no sea el release activo; omitirlo
# rompe el rollback a LT v17.
variable "iam_artifact_object_arns" {
  description = <<-EOT
    ARN exactos de objetos S3 de releases (binario y unidad systemd). La lista
    efectiva vive en el tfvars de custodia; `ARTEFACTOS_RELEASE.md` documenta los
    ARN de v0.19.2 y v0.20.0. Sin comodines de bucket o prefijo: el módulo IAM
    exige un objeto exacto por ARN.
  EOT
  type        = list(string)
  default     = []

  validation {
    condition = alltrue([
      for arn in var.iam_artifact_object_arns :
      can(regex("^arn:aws:s3:::[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]/.+$", arn)) && !strcontains(arn, "*") && !endswith(arn, "/")
    ])
    error_message = "Cada ARN debe ser un objeto S3 exacto (sin '*', sin terminar en '/') para no ampliar el permiso al bucket o prefijo."
  }
}

variable "release_objetivo" {
  description = <<-EOT
    Release que se va a desplegar, con la forma `vX.Y.Z+<sha12>` (por ejemplo
    `v0.21.0+abcdef123456`). Vacío = no se exige un release nuevo en la lista.
    Si no está vacío, la precondition bloqueante exige al menos un ARN exacto
    de ese release en `iam_artifact_object_arns`; en la práctica el custodio
    debe añadir los dos (binario y unidad) para que el rollout funcione.
  EOT
  type        = string
  default     = ""

  validation {
    # 11 o 12 hex: la política de releases fija `<sha12>` para releases nuevos,
    # pero el release desplegado `v0.20.0+4bf6758d372` se publicó con 11
    # caracteres. Aceptar ambos permite fijar el release objetivo real sin
    # relajar la forma SemVer+sha.
    condition     = var.release_objetivo == "" || can(regex("^v[0-9]+\\.[0-9]+\\.[0-9]+\\+[0-9a-f]{11,12}$", var.release_objetivo))
    error_message = "release_objetivo debe estar vacío o tener la forma vX.Y.Z+<sha12>."
  }
}

variable "iam_cloudwatch_log_group_arns" {
  description = "ARN exactos de log groups de I-110; vacío hasta que esa spec defina un sink"
  type        = list(string)
  default     = []
}

variable "iam_github_oidc_provider_arn" {
  description = "ARN existente/verificado de token.actions.githubusercontent.com; no se consulta ni crea en I-060"
  type        = string
  default     = ""
}

variable "iam_enable_ci_validation_role" {
  description = "Activa role trust-only CI tras verificar provider y environment protegido"
  type        = bool
  default     = false
}

variable "iam_ci_environment_protected" {
  description = "Confirmación externa de protection del environment ci-validation en GitHub"
  type        = bool
  default     = false
}

variable "iam_enable_plan_oidc_role" {
  description = "Activa role trust-only plan; permisos de state/backend pertenecen a I-070"
  type        = bool
  default     = true
}

variable "iam_enable_apply_oidc_role" {
  description = "Activa role trust-only apply; permanece desactivado hasta Gate de despliegue"
  type        = bool
  default     = false
}

variable "iam_apply_environment_protected" {
  description = "Confirmación externa de protection del environment aws-deploy en GitHub"
  type        = bool
  default     = false
}

variable "secretos_version" {
  description = "Version del bundle de secretos a fijar en el bootstrap (RFC-004). Vacio = sin bundle."
  type        = string
  default     = ""
}

variable "principal_operador_arn" {
  description = <<-EOT
    ARN del principal humano/operador autorizado a asumir las tres identidades de
    publicación creadas por RFC-011 (artefactos, secretos y gestor IaC).
    Sin valor por defecto: si falta, el plan falla cerrado. No se admiten la
    cuenta raíz, el ARN STS de una sesión ni comodines.
  EOT
  type        = string

  validation {
    condition     = can(regex("^arn:aws:iam::[0-9]{12}:(role|user)/[A-Za-z0-9+=,.@_/-]{1,64}$", var.principal_operador_arn))
    error_message = "principal_operador_arn debe ser un ARN IAM concreto de rol o usuario (fail-closed)."
  }

  validation {
    condition     = !startswith(var.principal_operador_arn, "arn:aws:sts::")
    error_message = "principal_operador_arn no puede ser un ARN STS de sesión; use el ARN IAM del rol real."
  }
}

// AMI fijada para la release. Vacio = resolver la ultima del parametro SSM publico.
// Se pasa al modulo de computo para que un despliegue de aplicacion no cambie la AMI.
variable "ami_id" {
  description = "AMI ARM64 explicita para el Launch Template. Vacio = ultima de SSM."
  type        = string
  default     = ""

  validation {
    condition     = var.ami_id == "" || can(regex("^ami-[0-9a-f]{8,17}$", var.ami_id))
    error_message = "ami_id debe estar vacio o ser un identificador ami- valido."
  }
}
