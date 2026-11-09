# --- I-120: guardas de encendido/apagado sobre el inventario declarativo ----

# Fuente unica de verdad del inventario: el mismo JSON que consume
# `herramientas/nexum_operacion.py`. Se lee del repositorio, no de AWS.
locals {
  inventario_operacion = jsondecode(file("${path.module}/../../operacion/inventario.json"))

  recursos_operacion = {
    for item in local.inventario_operacion.items : item.nombre => {
      tipo                = item.tipo
      componente          = item.componente
      criticidad          = item.criticidad
      etiqueta_criticidad = item.etiqueta_criticidad
      suspendible         = item.suspendible
      estado_recurso      = item.estado_recurso
    }
  }
}

module "operacion" {
  source = "../../modules/operacion"

  proyecto = "nexum"
  entorno  = var.entorno

  recursos = local.recursos_operacion

  # Allowlist revisada y separada del inventario: ampliar auxiliares exige
  # editar este archivo y pasar revision, no solo tocar la fuente de datos.
  allowlist_auxiliar = [
    "interface-endpoint-secretsmanager",
    "interface-endpoint-ssm",
    "interface-endpoint-ssmmessages",
  ]

  max_denegados  = 0
  destroy_datos  = false
  destroy_estado = false

  # Sin plan aplicado en I-120 LOCAL: no se declara alcance ni se asume nada.
  cambios_propuestos = []
}

output "operacion" {
  description = "Clasificacion core/auxiliar, denegados y politica de encendido/apagado (sin AWS aplicado)"
  value = {
    core                     = module.operacion.core
    suspendibles             = module.operacion.suspendibles
    denegados                = module.operacion.denegados
    auxiliares_sin_allowlist = module.operacion.auxiliares_fuera_de_allowlist
    politica                 = module.operacion.politica
    alcance_plan             = module.operacion.alcance_plan
    umbrales_credito_usd     = local.inventario_operacion.umbrales_credito_usd
    estado_despliegue        = module.operacion.estado_despliegue
  }
}
