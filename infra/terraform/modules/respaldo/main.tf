terraform {
  required_version = ">= 1.4.0, < 2.0.0"
}

# Modulo de respaldo de I-130. NO crea recursos: el proveedor AWS no expone
# `aws_dynamodb_backup`, de modo que el backup on-demand es un PROCEDIMIENTO
# autorizado fuera de Terraform y aqui solo se declaran guardas y politica.
#
# Guardas que fallan cerrado: PITR obligatorio, state con versionado + cifrado +
# proteccion de borrado, patron de restauracion que nunca alcanza la tabla
# activa, retencion dentro del limite de PITR y destroys prohibidos.

locals {
  # Si el patron de restauracion casara la tabla activa, un plan podria
  # "restaurar" sobre datos vivos.
  patron_no_alcanza_tabla_activa = !can(regex(var.patron_tabla_destino, var.tabla_origen))
}

resource "terraform_data" "guardas_backup" {
  input = {
    entorno             = var.entorno
    tabla_origen        = var.tabla_origen
    pitr_habilitado     = var.pitr_habilitado
    retencion_dias      = var.retencion_dias
    state_versionado    = var.state_versionado
    state_cifrado       = var.state_cifrado
    state_proteccion    = var.state_proteccion_borrado
    patron_restauracion = var.patron_tabla_destino
    destroy_datos       = var.permitir_destroy_datos
    borrado_copias      = var.permitir_borrado_copias
    backup_on_demand    = "PROCEDIMIENTO_FUERA_DE_TERRAFORM"
    estado_despliegue   = "NO_CREADO"
    restauracion_aws    = "RESTORE_AWS_NO_VERIFICADO"
    rto_rpo             = "OBJETIVO_PROPUESTO_NO_MEDIDO"
  }

  lifecycle {
    precondition {
      condition     = var.pitr_habilitado
      error_message = "INF-130-02: sin PITR no se declara restauracion a un punto en el tiempo."
    }

    precondition {
      condition     = var.state_versionado && var.state_cifrado && var.state_proteccion_borrado
      error_message = "INF-130-03: el state exige versionado, cifrado y proteccion de borrado simultaneamente."
    }

    precondition {
      condition     = local.patron_no_alcanza_tabla_activa
      error_message = "INF-130-04: el patron de restauracion alcanza la tabla activa; la restauracion solo puede ir a una tabla nueva aislada."
    }

    precondition {
      condition     = var.permitir_destroy_datos == false && var.permitir_borrado_copias == false
      error_message = "INF-130-05: destruir datos o borrar copias fuera de politica no se habilita en local."
    }

    precondition {
      condition     = var.retencion_dias <= 35
      error_message = "La retencion declarada excede el maximo de PITR (35 dias); no se puede prometer mas de lo que AWS conserva."
    }

    precondition {
      condition     = var.tabla_arn != "" && var.tabla_origen != ""
      error_message = "Faltan datos de la tabla activa: sin tabla no hay respaldo ni restauracion que verificar."
    }
  }
}
