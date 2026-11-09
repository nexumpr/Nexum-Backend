output "politica" {
  description = "Politica de respaldo, restauracion y retencion aplicada"
  value = {
    pitr                      = var.pitr_habilitado
    retencion_dias            = var.retencion_dias
    backup_on_demand          = "PROCEDIMIENTO_FUERA_DE_TERRAFORM"
    patron_restauracion       = var.patron_tabla_destino
    tabla_activa              = var.tabla_origen
    restauracion_sobre_activa = "PROHIBIDA"
    destroy_datos             = "PROHIBIDO"
    borrado_copias            = "BLOQUEADO_EN_LOCAL"
    rto_rpo                   = "OBJETIVO_PROPUESTO_NO_MEDIDO"
    restauracion_aws          = "RESTORE_AWS_NO_VERIFICADO"
  }
}

output "estado_despliegue" {
  description = "Sin AWS aplicado: nada creado"
  value       = "NO_CREADO"
}
