output "core" {
  description = "Recursos core clasificados: intocables 24x7"
  value       = local.core
}

output "suspendibles" {
  description = "Recursos auxiliares autorizados a suspender (interseccion inventario + allowlist)"
  value       = local.suspendibles
}

output "denegados" {
  description = "Recursos que se deniegan por falta de etiqueta, allowlist o coherencia"
  value       = local.denegados
}

output "auxiliares_fuera_de_allowlist" {
  description = "Auxiliares declarados que no estan en la allowlist revisada"
  value       = local.auxiliares_fuera_de_allowlist
}

output "clasificacion" {
  description = "Inventario clasificado por recurso"
  value       = local.clasificacion
}

output "politica" {
  description = "Politica operativa aplicada, para runbook y pruebas de contrato"
  value = {
    core                   = "24x7_intocable"
    auxiliar               = "ventana_con_aprobacion"
    por_defecto            = "deny"
    etiqueta_critica       = "nexum:criticality"
    destroy_datos          = "PROHIBIDO"
    destroy_estado         = "PROHIBIDO"
    destroy_all            = "INEXISTENTE"
    minimo_nodos_publicado = 2
  }
}

output "estado_despliegue" {
  description = "Las guardas son locales: no se creo ni modifico ningun recurso AWS"
  value       = "NO_CREADO"
}

output "alcance_plan" {
  description = "Verificacion del alcance de los cambios propuestos"
  value = {
    cambios            = length(var.cambios_propuestos)
    sobre_core         = local.cambios_sobre_core
    fuera_de_allowlist = local.cambios_fuera_de_allowlist
    permite_delete     = false
    permite_destroy    = false
    minimo_nodos       = 2
  }
}
