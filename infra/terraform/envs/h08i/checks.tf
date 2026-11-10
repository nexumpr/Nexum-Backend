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
