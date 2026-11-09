terraform {
  required_version = ">= 1.4.0, < 2.0.0"
}

# Modulo de guardas operativas de I-120. NO crea recursos AWS: solo clasifica el
# inventario y detiene el plan si alguien intenta operar sobre el core, sobre un
# recurso sin etiqueta o sobre las capacidades de destruccion de datos/estado.

locals {
  clasificacion = {
    for nombre, recurso in var.recursos : nombre => merge(recurso, { nombre = nombre })
  }

  criticiades_validas = ["core", "auxiliary"]

  core = [
    for nombre, recurso in local.clasificacion : nombre if recurso.criticidad == "core"
  ]

  auxiliares = [
    for nombre, recurso in local.clasificacion : nombre if recurso.criticidad == "auxiliary"
  ]

  # Deny por defecto: cualquier anomalia clasifica el recurso como denegado.
  denegados = sort([
    for nombre, recurso in local.clasificacion : nombre
    if !contains(local.criticiades_validas, recurso.criticidad)
    || recurso.etiqueta_criticidad != recurso.criticidad
    || (recurso.criticidad == "core" && recurso.suspendible)
    || (recurso.criticidad == "auxiliary" && !recurso.suspendible)
    || (recurso.criticidad == "auxiliary" && !contains(var.allowlist_auxiliar, nombre))
  ])

  suspendibles = sort([
    for nombre in local.auxiliares : nombre if contains(var.allowlist_auxiliar, nombre)
  ])

  auxiliares_fuera_de_allowlist = sort([
    for nombre in local.auxiliares : nombre if !contains(var.allowlist_auxiliar, nombre)
  ])

  allowlist_desconocida = sort([
    for nombre in var.allowlist_auxiliar : nombre if !contains(keys(local.clasificacion), nombre)
  ])

  # Un core suspendible es la violacion mas grave: rompe GOV-02/GOV-05.
  core_suspendibles = sort([
    for nombre, recurso in local.clasificacion : nombre if recurso.criticidad == "core" && recurso.suspendible
  ])

  componentes_core = toset([
    for nombre, recurso in local.clasificacion : recurso.componente if recurso.criticidad == "core"
  ])

  # Componentes que el core debe conservar siempre; si desaparecen del inventario,
  # suspender auxiliares pareceria no tener coste ni riesgo.
  componentes_core_obligatorios = ["computo", "datos", "estado"]

  componentes_core_faltantes = sort([
    for componente in local.componentes_core_obligatorios : componente
    if !contains(local.componentes_core, componente)
  ])
}

resource "terraform_data" "guardas_operativas" {
  input = {
    entorno              = var.entorno
    core                 = local.core
    suspendibles         = local.suspendibles
    denegados            = local.denegados
    allowlist            = var.allowlist_auxiliar
    destroy_datos        = var.destroy_datos
    destroy_estado       = var.destroy_estado
    estado_recurso       = "clasificado"
    estado_despliegue    = "NO_CREADO"
    requiere_gate_aws    = true
    max_denegados        = var.max_denegados
    etiqueta_critica     = "nexum:criticality"
    politica_core        = "24x7_intocable"
    politica_auxiliar    = "ventana_con_aprobacion"
    politica_por_defecto = "deny"
  }

  lifecycle {
    precondition {
      condition     = length(local.denegados) == 0
      error_message = "Recursos denegados: ${join(", ", local.denegados)}. Sin etiqueta nexum:criticality valida, sin allowlist o con criticidad incoherente se deniega siempre (deny por defecto)."
    }

    precondition {
      condition     = length(local.core_suspendibles) == 0
      error_message = "Recursos core marcados como suspendibles: ${join(", ", local.core_suspendibles)}. El core (2 EC2, ALB, CloudFront, Redis, DynamoDB, state) nunca se suspende."
    }

    precondition {
      condition     = length(local.allowlist_desconocida) == 0
      error_message = "Allowlist con recursos que no existen en el inventario: ${join(", ", local.allowlist_desconocida)}."
    }

    precondition {
      condition     = length(local.componentes_core_faltantes) == 0
      error_message = "El inventario no declara core para: ${join(", ", local.componentes_core_faltantes)}. Un inventario incompleto haria creer que el apagado no tiene coste."
    }

    precondition {
      condition     = var.destroy_datos == false && var.destroy_estado == false
      error_message = "destroy-data y destroy-state deben permanecer en false: son capacidades inexistentes en I-120."
    }
  }
}
