# RFC-011 — Identidades de publicación (artefactos, secretos y gestor IaC).
#
# Motivo: `envs/h08i` exige tres ARN reales (`artefactos_publicador_arn`,
# `secretos_publicador_arn`, `secretos_gestor_iac_arn`) y vacíos hacen fallar el
# plan cerrado. OIDC está denegado por SCP (RFC-010), así que estas identidades
# se asumen dentro de la propia cuenta desde un principal operador autorizado.
#
# Reglas de diseño:
#  - Sin usuarios IAM, sin claves de acceso y sin proveedor OIDC.
#  - Trust restringido a UN principal operador explícito (`aws:PrincipalArn`),
#    nunca a la cuenta raíz ni a un comodín.
#  - Políticas de mínimo privilegio: sólo el prefijo exacto del bucket que cada
#    identidad necesita, más `ListBucket`/`GetBucketLocation` acotados.
#  - Sin ciclo en el grafo: los módulos de bucket reciben los ARN de estos roles
#    y este módulo recibe los NOMBRES de los buckets (no sus ARN), de modo que
#    no hay dependencia mutua.
#  - Los nombres son deterministas (`<proyecto>-<entorno>-<rol>-<cuenta>`), que
#    es exactamente lo que valida la raíz con `check`.

terraform {
  required_version = ">= 1.10.0, < 2.0.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.72"
    }
  }
}

locals {
  nombre_artefactos_publicador = lower("${var.proyecto}-${var.entorno}-artefactos-publicador")
  nombre_secretos_publicador   = lower("${var.proyecto}-${var.entorno}-secretos-publicador")
  nombre_secretos_gestor_iac   = lower("${var.proyecto}-${var.entorno}-secretos-gestor-iac")

  arn_artefactos_publicador = "arn:aws:iam::${var.cuenta_id}:role/${local.nombre_artefactos_publicador}"
  arn_secretos_publicador   = "arn:aws:iam::${var.cuenta_id}:role/${local.nombre_secretos_publicador}"
  arn_secretos_gestor_iac   = "arn:aws:iam::${var.cuenta_id}:role/${local.nombre_secretos_gestor_iac}"

  arn_bucket_artefactos = "arn:aws:s3:::${var.bucket_artefactos_nombre}"
  arn_bucket_secretos   = "arn:aws:s3:::${var.bucket_secretos_nombre}"
}

# ---------------------------------------------------------------------------
# Documento de confianza: sólo el principal operador autorizado puede asumir.
#
# Regla dura de AWS: una *trust policy* de rol **no admite `Resource` ni
# `NotResource`**; el servicio responde `MalformedPolicyDocument` y el rol no se
# crea. Por eso este documento:
#   - no declara `resources`/`not_resources` (y una precondition lo verifica
#     sobre el JSON ya renderizado, para que no pueda reaparecer por descuido);
#   - no usa un `Deny` con principal ajeno: el mínimo privilegio ya lo garantiza
#     el `Principal` exacto más la condición, y un `Deny` en una trust policy no
#     añadiría confianza negativa válida sin `Resource`;
#   - sólo autoriza `sts:AssumeRole` al ARN IAM exacto del operador
#     (`arn:aws:iam::<cuenta>:role|user/<nombre>`), nunca `:root` ni `"*"`.
#
# `aws:PrincipalArn` devuelve el ARN del rol (no el ARN STS de la sesión) cuando
# la petición la hace una sesión de rol; se documenta la referencia oficial en
# `RFC-011`. Se usa `ArnEquals` (operador de ARN) en lugar de `StringEquals`,
# como recomienda AWS para claves de tipo ARN.
# ---------------------------------------------------------------------------
data "aws_iam_policy_document" "trust_operador" {
  statement {
    sid     = "SoloPrincipalOperadorAutorizado"
    effect  = "Allow"
    actions = ["sts:AssumeRole"]

    principals {
      type        = "AWS"
      identifiers = [var.principal_operador_arn]
    }

    condition {
      test     = "ArnEquals"
      variable = "aws:PrincipalArn"
      values   = [var.principal_operador_arn]
    }
  }
}

# Verificación estructural del documento de confianza ya renderizado por el
# proveedor. No es una búsqueda de texto: se decodifica el JSON y se inspeccionan
# las claves de cada enunciado, de modo que un atributo añadido en el bloque
# equivocado no pueda pasar inadvertido.
locals {
  trust_politica   = jsondecode(data.aws_iam_policy_document.trust_operador.json)
  trust_enunciados = try(local.trust_politica.Statement, [])

  trust_sin_campos_prohibidos = length(local.trust_enunciados) > 0 && alltrue([
    for e in local.trust_enunciados :
    !contains(keys(e), "Resource") &&
    !contains(keys(e), "NotResource") &&
    !contains(keys(e), "NotPrincipal")
  ])

  trust_solo_allow = alltrue([
    for e in local.trust_enunciados : try(e.Effect, "Deny") == "Allow"
  ])

  trust_principal_exacto = alltrue([
    for e in local.trust_enunciados :
    try(keys(e.Principal), []) == ["AWS"] && (
      try(tostring(e.Principal.AWS), "") == var.principal_operador_arn ||
      try(join(",", e.Principal.AWS), "-") == var.principal_operador_arn
    )
  ])

  trust_condicion_exacta = alltrue([
    for e in local.trust_enunciados :
    try(keys(e.Condition), []) == ["ArnEquals"] &&
    try(tostring(e.Condition.ArnEquals["aws:PrincipalArn"]), "") == var.principal_operador_arn
  ])
}

# ---------------------------------------------------------------------------
# 1) Publicador de artefactos (binarios y SBOM).
# ---------------------------------------------------------------------------
resource "aws_iam_role" "artefactos_publicador" {
  name                 = local.nombre_artefactos_publicador
  description          = "Publica binarios y SBOM en el prefijo autorizado del bucket de artefactos (RFC-011)."
  assume_role_policy   = data.aws_iam_policy_document.trust_operador.json
  max_session_duration = 3600

  tags = merge(var.etiquetas, {
    Spec     = "RFC-011-identidades-publicacion"
    Funcion  = "publicador-artefactos"
    Entorno  = var.entorno
    Proyecto = "Nexum-Backend"
  })
}

data "aws_iam_policy_document" "artefactos_publicador" {
  statement {
    sid    = "PublicarSoloPrefijoAutorizado"
    effect = "Allow"
    actions = [
      "s3:PutObject",
      "s3:AbortMultipartUpload",
    ]
    resources = ["${local.arn_bucket_artefactos}/${var.prefijo_artefactos}*"]
  }

  statement {
    sid       = "ListarSoloSuPrefijo"
    effect    = "Allow"
    actions   = ["s3:ListBucket", "s3:GetBucketLocation"]
    resources = [local.arn_bucket_artefactos]

    condition {
      test     = "StringLike"
      variable = "s3:prefix"
      values   = ["${var.prefijo_artefactos}*"]
    }
  }

  statement {
    sid       = "DenegarBorradoYFueraDePrefijo"
    effect    = "Deny"
    actions   = ["s3:DeleteObject", "s3:DeleteObjectVersion", "s3:PutBucketPolicy", "s3:PutBucketAcl"]
    resources = [local.arn_bucket_artefactos, "${local.arn_bucket_artefactos}/*"]
  }
}

resource "aws_iam_role_policy" "artefactos_publicador" {
  name   = "publicar-artefactos-prefijo"
  role   = aws_iam_role.artefactos_publicador.id
  policy = data.aws_iam_policy_document.artefactos_publicador.json
}

# ---------------------------------------------------------------------------
# 2) Publicador del bundle de secretos (RFC-004).
# ---------------------------------------------------------------------------
resource "aws_iam_role" "secretos_publicador" {
  name                 = local.nombre_secretos_publicador
  description          = "Publica el bundle de secretos en el prefijo autorizado (RFC-004/RFC-011)."
  assume_role_policy   = data.aws_iam_policy_document.trust_operador.json
  max_session_duration = 3600

  tags = merge(var.etiquetas, {
    Spec     = "RFC-011-identidades-publicacion"
    Funcion  = "publicador-secretos"
    Entorno  = var.entorno
    Proyecto = "Nexum-Backend"
  })
}

data "aws_iam_policy_document" "secretos_publicador" {
  statement {
    sid       = "PublicarSoloPrefijoAutorizado"
    effect    = "Allow"
    actions   = ["s3:PutObject"]
    resources = ["${local.arn_bucket_secretos}/${var.prefijo_secretos}*"]
  }

  statement {
    sid       = "ListarSoloSuPrefijo"
    effect    = "Allow"
    actions   = ["s3:ListBucket", "s3:GetBucketLocation"]
    resources = [local.arn_bucket_secretos]

    condition {
      test     = "StringLike"
      variable = "s3:prefix"
      values   = ["${var.prefijo_secretos}*"]
    }
  }

  statement {
    sid       = "DenegarLecturaYFueraDePrefijo"
    effect    = "Deny"
    actions   = ["s3:GetObject", "s3:DeleteObject", "s3:DeleteObjectVersion", "s3:PutBucketPolicy", "s3:PutBucketAcl"]
    resources = [local.arn_bucket_secretos, "${local.arn_bucket_secretos}/*"]
  }
}

resource "aws_iam_role_policy" "secretos_publicador" {
  name   = "publicar-secretos-prefijo"
  role   = aws_iam_role.secretos_publicador.id
  policy = data.aws_iam_policy_document.secretos_publicador.json
}

# ---------------------------------------------------------------------------
# 3) Gestor IaC del bucket de secretos: refresco de Terraform y publicación
#    controlada. Distinto del runtime y del publicador (I-060/RFC-004).
# ---------------------------------------------------------------------------
resource "aws_iam_role" "secretos_gestor_iac" {
  name                 = local.nombre_secretos_gestor_iac
  description          = "Gestor IaC del bucket de secretos: refresco y lectura/escritura acotada (RFC-004/RFC-011)."
  assume_role_policy   = data.aws_iam_policy_document.trust_operador.json
  max_session_duration = 3600

  tags = merge(var.etiquetas, {
    Spec     = "RFC-011-identidades-publicacion"
    Funcion  = "gestor-iac-secretos"
    Entorno  = var.entorno
    Proyecto = "Nexum-Backend"
  })
}

data "aws_iam_policy_document" "secretos_gestor_iac" {
  statement {
    sid       = "RefrescoDelBucket"
    effect    = "Allow"
    actions   = ["s3:ListBucket", "s3:GetBucketLocation", "s3:GetBucketVersioning"]
    resources = [local.arn_bucket_secretos]
  }

  statement {
    sid       = "GestionSoloPrefijoAutorizado"
    effect    = "Allow"
    actions   = ["s3:GetObject", "s3:PutObject", "s3:GetObjectVersion"]
    resources = ["${local.arn_bucket_secretos}/${var.prefijo_secretos}*"]
  }

  statement {
    sid       = "DenegarDestruccionYAdministracion"
    effect    = "Deny"
    actions   = ["s3:DeleteObject", "s3:DeleteObjectVersion", "s3:DeleteBucket", "s3:PutBucketPolicy", "s3:PutBucketAcl"]
    resources = [local.arn_bucket_secretos, "${local.arn_bucket_secretos}/*"]
  }
}

resource "aws_iam_role_policy" "secretos_gestor_iac" {
  name   = "gestor-iac-secretos-prefijo"
  role   = aws_iam_role.secretos_gestor_iac.id
  policy = data.aws_iam_policy_document.secretos_gestor_iac.json
}

# ---------------------------------------------------------------------------
# El principal operador debe EXISTIR en IAM.
#
# Un ARN con el path equivocado (por ejemplo sin `/managed/`) tiene exactamente
# el mismo aspecto que uno correcto, pero AWS lo rechaza al crear el rol con
# `MalformedPolicyDocument: Invalid principal in policy`, es decir, ya en el
# apply. El ARN de una sesión STS (`assumed-role/<rol>/<sesión>`) tampoco muestra
# el path, así que no sirve como comprobación. Se resuelve el rol real y se
# compara el ARN completo.
# ---------------------------------------------------------------------------
data "aws_iam_role" "operador" {
  count = can(regex(":role/", var.principal_operador_arn)) ? 1 : 0

  # `arn:aws:iam::<cuenta>:role/<path>/<nombre>` → el nombre es el último segmento.
  name = reverse(split("/", split(":role/", var.principal_operador_arn)[1]))[0]
}

# ---------------------------------------------------------------------------
# Guardas fail-closed de la propia unidad.
# ---------------------------------------------------------------------------
resource "terraform_data" "identidades_preconditions" {
  input = join("|", [
    var.cuenta_id,
    var.principal_operador_arn,
    var.bucket_artefactos_nombre,
    var.bucket_secretos_nombre,
    var.prefijo_artefactos,
    var.prefijo_secretos,
    var.rol_runtime_arn,
  ])

  lifecycle {
    precondition {
      condition     = can(regex("^[0-9]{12}$", var.cuenta_id))
      error_message = "cuenta_id debe ser un identificador de cuenta de 12 digitos."
    }

    precondition {
      condition     = can(regex("^arn:aws:iam::[0-9]{12}:(role|user)/[A-Za-z0-9+=,.@_/-]{1,64}$", var.principal_operador_arn))
      error_message = "principal_operador_arn debe ser un ARN de rol o usuario IAM concreto; la cuenta raiz y los comodines no se admiten."
    }

    precondition {
      condition     = !endswith(var.principal_operador_arn, ":root")
      error_message = "El trust no puede apuntar a la cuenta raiz."
    }

    # El ARN debe ser el del rol/usuario IAM real, no el ARN STS de la sesión
    # (`arn:aws:sts::...:assumed-role/<rol>/<sesión>`), que cambia en cada login y
    # no es un principal estable al que anclar la confianza.
    precondition {
      condition     = !startswith(var.principal_operador_arn, "arn:aws:sts::")
      error_message = "principal_operador_arn no puede ser un ARN STS de sesion (arn:aws:sts::...:assumed-role/...); use el ARN IAM del rol real (arn:aws:iam::...:role/...)."
    }

    precondition {
      condition     = startswith(var.principal_operador_arn, "arn:aws:iam::")
      error_message = "principal_operador_arn debe ser un ARN IAM (arn:aws:iam::<cuenta>:role|user/<nombre>)."
    }

    # El ARN debe resolver a un rol IAM existente. Si el principal es un usuario
    # (count = 0 en el data source) la comparación se resuelve con el propio valor
    # y no añade requisitos nuevos.
    precondition {
      condition     = try(data.aws_iam_role.operador[0].arn, var.principal_operador_arn) == var.principal_operador_arn
      error_message = "principal_operador_arn no corresponde a un rol IAM existente: revisar el path del ARN (por ejemplo /managed/)."
    }

    precondition {
      condition = !contains(
        [local.arn_artefactos_publicador, local.arn_secretos_publicador, local.arn_secretos_gestor_iac],
        var.principal_operador_arn
      )
      error_message = "El principal operador no puede ser una de las tres identidades de publicacion creadas aqui (confianza circular)."
    }

    # Guardas de la trust policy: AWS rechaza `Resource`/`NotResource` en una
    # politica de confianza y un principal comodin ampliaria la confianza.
    precondition {
      condition     = local.trust_sin_campos_prohibidos
      error_message = "La trust policy no puede contener Resource, NotResource ni NotPrincipal: AWS la rechaza con MalformedPolicyDocument."
    }

    precondition {
      condition     = local.trust_solo_allow
      error_message = "La trust policy debe limitarse a un unico enunciado Allow; un Deny en una politica de confianza no restringe y confunde la auditoria."
    }

    precondition {
      condition     = local.trust_principal_exacto
      error_message = "La trust policy debe declarar exactamente Principal AWS = principal_operador_arn, sin comodines ni otros tipos de principal."
    }

    precondition {
      condition     = local.trust_condicion_exacta
      error_message = "La trust policy debe exigir ArnEquals sobre aws:PrincipalArn igual a principal_operador_arn."
    }

    precondition {
      condition     = length(distinct([local.arn_artefactos_publicador, local.arn_secretos_publicador, local.arn_secretos_gestor_iac, var.rol_runtime_arn])) == 4
      error_message = "Las tres identidades de publicacion y el rol runtime deben ser distintas entre si."
    }

    precondition {
      condition     = endswith(var.prefijo_artefactos, "/") && endswith(var.prefijo_secretos, "/")
      error_message = "Los prefijos deben terminar en '/' para acotar los objetos correctamente."
    }

    precondition {
      condition     = var.cuenta_id == element(split(":", var.principal_operador_arn), 4)
      error_message = "El principal operador debe pertenecer a la misma cuenta que las identidades."
    }
  }
}
