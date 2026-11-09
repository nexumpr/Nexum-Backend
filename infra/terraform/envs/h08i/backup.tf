# --- I-130: respaldo, restauracion y retencion -----------------------------

module "backup" {
  source = "../../modules/respaldo"

  proyecto = "nexum"
  entorno  = var.entorno
  region   = var.region

  tabla_origen = module.dynamodb.table_name
  tabla_arn    = module.dynamodb.table_arn

  # PITR viene de I-040; sin el no se declara restauracion a un punto en el tiempo.
  pitr_habilitado = var.dynamodb_pitr_habilitado

  # El backup on-demand es un procedimiento autorizado fuera de Terraform:
  # el proveedor AWS no expone un recurso declarativo para el.

  retencion_dias = 30

  # Hechos declarados de I-070 (versionado, SSE-S3 y prevent_destroy).
  state_versionado         = true
  state_cifrado            = true
  state_proteccion_borrado = true

  patron_tabla_destino = "^NEXUM_CORE_RESTORE_[a-z0-9]{4,20}$"

  permitir_destroy_datos  = false
  permitir_borrado_copias = false
}

output "backup" {
  description = "Politica de respaldo, restauracion y retencion (sin AWS aplicado)"
  value = {
    politica          = module.backup.politica
    estado_despliegue = module.backup.estado_despliegue
  }
}
