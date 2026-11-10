# I-070 — Backend remoto del entorno h08i.
#
# Gate: `GATE_H08I_STATE_MIGRATION.md` (bootstrap ya migrado) y
# `GATE_H08I_FASE_BASE.md`. Clave DISTINTA de la del bootstrap:
#   bootstrap → bootstrap/terraform.tfstate
#   runtime   → envs/h08i/terraform.tfstate
# No se importa ni se reutiliza el state del bootstrap.
#
# Locking nativo de S3 (`use_lockfile`, Terraform >= 1.10); sin tabla DynamoDB.

terraform {
  backend "s3" {
    bucket       = "nexum-h08i-tfstate-713797602090"
    key          = "envs/h08i/terraform.tfstate"
    region       = "us-east-2"
    encrypt      = true
    use_lockfile = true
  }
}
