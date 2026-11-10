# I-070 — Backend remoto del bootstrap (state del propio bootstrap).
#
# Se activa SÓLO con el Gate de migración aprobado (`GATE_H08I_STATE_MIGRATION.md`).
# El bucket ya existe: lo creó este mismo root con state local (7 recursos AWS
# verificados, 0 destrucciones) en la operación del 2026-11-10.
#
# La clave es DISTINTA de la del runtime (`envs/h08i/terraform.tfstate`): cada state
# tiene su propio objeto y su propio `.tflock`.
#
# Locking nativo de S3 (`use_lockfile = true`, Terraform >= 1.10); sin tabla DynamoDB,
# cuyo locking está deprecado por HashiCorp y no está presupuestado.
#
# No se hardcodean credenciales: el perfil/entorno las aporta.

terraform {
  backend "s3" {
    bucket       = "nexum-h08i-tfstate-713797602090"
    key          = "bootstrap/terraform.tfstate"
    region       = "us-east-2"
    encrypt      = true
    use_lockfile = true
  }
}
