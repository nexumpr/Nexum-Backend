# RFC-011 — Coherencia entre los ARN derivados y las identidades reales.
#
# Los módulos de bucket reciben los ARN como cadena conocida para que sus
# preconditions de formato puedan evaluarse en tiempo de plan (I-060). Este
# `check` cierra el hueco: cuando los roles existen, compara el ARN real que
# devuelve `module.identidades` con el derivado y avisa si dejan de coincidir.
#
# `check` no bloquea el plan con valores desconocidos: se evalúa al aplicar.
check "identidades_publicacion_coherentes" {
  assert {
    condition     = module.identidades.artefactos_publicador_arn == local.arn_artefactos_publicador
    error_message = "El ARN real del publicador de artefactos no coincide con el derivado: revisar el nombre del rol."
  }

  assert {
    condition     = module.identidades.secretos_publicador_arn == local.arn_secretos_publicador
    error_message = "El ARN real del publicador de secretos no coincide con el derivado: revisar el nombre del rol."
  }

  assert {
    condition     = module.identidades.secretos_gestor_iac_arn == local.arn_secretos_gestor_iac
    error_message = "El ARN real del gestor IaC de secretos no coincide con el derivado: revisar el nombre del rol."
  }

  assert {
    condition     = module.artefactos.bucket_name == local.bucket_artefactos_nombre
    error_message = "El nombre del bucket de artefactos no coincide con el derivado: las políticas apuntarían a otro bucket."
  }

  assert {
    condition     = module.secretos.bucket_name == local.bucket_secretos_nombre
    error_message = "El nombre del bucket de secretos no coincide con el derivado: las políticas apuntarían a otro bucket."
  }
}

# ---------------------------------------------------------------------------
# Invariantes BLOQUEANTES (RFC-011).
#
# Motivo: un bloque `check` sólo emite una advertencia; Terraform **no** detiene
# el `apply`. Si el nombre del rol o del bucket divergiera, las políticas S3
# quedarían apuntando a un recurso inexistente y el despliegue fallaría más
# tarde (o peor: en runtime). Estas invariantes se declaran como `precondition`
# de un `terraform_data`, que sí aborta el apply.
#
# Se comparan valores **conocidos en el plan** (`name` y `bucket` los fija la
# configuración), de modo que el fallo aparece al planificar y no se difiere a
# un valor "conocido tras apply". El ARN no se compara aquí porque es función
# determinista de (cuenta, nombre): con el nombre y la cuenta correctos el ARN
# queda determinado, y el `check` de arriba lo verifica además con el valor real.
#
# Sin ciclo: este recurso depende de los módulos y ningún módulo depende de él.
# ---------------------------------------------------------------------------
resource "terraform_data" "invariantes_identidades" {
  input = local.cuenta_id

  lifecycle {
    precondition {
      condition     = module.identidades.nombres.artefactos_publicador == local.nombre_artefactos_publicador
      error_message = "El nombre del rol publicador de artefactos no coincide con el esperado; las políticas S3 apuntarían a un rol distinto."
    }

    precondition {
      condition     = module.identidades.nombres.secretos_publicador == local.nombre_secretos_publicador
      error_message = "El nombre del rol publicador de secretos no coincide con el esperado; la política de escritura del bundle apuntaría a otro rol."
    }

    precondition {
      condition     = module.identidades.nombres.secretos_gestor_iac == local.nombre_secretos_gestor_iac
      error_message = "El nombre del rol gestor IaC de secretos no coincide con el esperado; el refresco de Terraform quedaría sin permiso."
    }

    precondition {
      condition     = module.artefactos.bucket_name == local.bucket_artefactos_nombre
      error_message = "El nombre del bucket de artefactos no coincide con el esperado; el release se publicaría en un bucket distinto."
    }

    precondition {
      condition     = module.secretos.bucket_name == local.bucket_secretos_nombre
      error_message = "El nombre del bucket de secretos no coincide con el esperado; el runtime leería el bundle de otro bucket."
    }

    precondition {
      condition     = length(var.entorno) > 0 && !strcontains(local.bucket_artefactos_nombre, "..")
      error_message = "El entorno debe estar definido y los nombres derivados no pueden contener '..'."
    }
  }
}
