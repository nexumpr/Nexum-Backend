# Gate H08-E / E005 — A10 IAM + normalización Redis

**ESTADO:** `APLICADO — VERIFICADO` (2026-11-17).

Registro de la autorización humana del plan **A10** del flujo E005. No autoriza ningún otro
plan, recurso, despliegue ni cambio de contrato.

## 1. Autorización humana

| Campo | Valor |
|---|---|
| Frase | `APRUEBO_PLAN_A10_SHA_FDF4FE1A10C451AB8379AA2A5BBDD4D1FDCC9876145894FC399BEBDDF9E05FA8` |
| Alcance | Aplicar **únicamente** el plan guardado `a10-e005.tfplan` |
| Excepciones | Normalización del usuario Redis `default` (`no-password` → `no-password-required`) incluida en el plan |
| Aprobador | Responsable del proyecto (instrucción de sesión) |

## 2. Plan autorizado

| Campo | Valor |
|---|---|
| Archivo | `C:\Nexum-Custodia\H08E-E005-20261117\a10-e005.tfplan` |
| **SHA-256** | **`FDF4FE1A10C451AB8379AA2A5BBDD4D1FDCC9876145894FC399BEBDDF9E05FA8`** |
| Tamaño | 184 519 bytes |
| Fecha del archivo | `2026-11-17T02:45:31Z` |
| Terraform | 1.16.6 · provider `hashicorp/aws` 5.100.0 |
| Backend | `s3://nexum-h08i-tfstate-713797602090/envs/h08i/terraform.tfstate` |
| Resultado JSON | `applyable=true`, `complete=true`, `errored=false` |
| Acciones | **1 create · 2 update · 0 delete · 0 replace · 0 import** |

## 3. Cambios exactos verificados

1. **Crear** `module.iam.aws_iam_role_policy.runtime_ses[0]` (`nexum-h08i-runtime-ses`):
   `ses:SendEmail` sobre `arn:aws:ses:us-east-2:713797602090:identity/nexumproyectomov@gmail.com`,
   con `ses:FromAddress=nexumproyectomov@gmail.com` y `aws:SecureTransport=true`.
2. **Actualizar** `module.iam.aws_iam_role_policy.runtime` (`nexum-h08i-runtime-approved-data-access`):
   añade `RESET#*` y `QUEUE#*` a las `LeadingKeys` de DynamoDB en la tabla y en `GSI1`, sin retirar
   ninguna clave previa y sin cambiar acciones, recursos ni condiciones.
3. **Actualizar** `module.redis.aws_elasticache_user.default_deshabilitado`:
   `authentication_mode.type` `no-password` → `no-password-required`; `access_string` permanece
   `off ~* -@all`; usuario de aplicación, grupo de usuarios y replication group sin cambios.

## 4. Comprobaciones de seguridad

- `0 delete`, `0 replace`, `0 import`: no hay operaciones destructivas ni recreaciones.
- No se modifican DynamoDB, Redis (más allá de la normalización del usuario deshabilitado), ALB,
  CloudFront, VPC, Launch Template, ASG, buckets, secretos ni E001–E004.
- El cambio Redis no habilita autenticación sin contraseña ni acceso: el usuario `default` sigue
  con `off ~* -@all`; el usuario de aplicación conserva su contraseña y TLS.
- Coste incremental del plan: **0.0000 USD** (IAM inline policies y modificación de usuario
  ElastiCache no generan cargos nuevos).
- El drift detectado en `module.iam.aws_iam_role.runtime` (8 → 10 ARNs de artefactos) está
  reconciliado como `no-op`: el plan no modifica el rol.

## 5. Ejecución

| Campo | Valor |
|---|---|
| Comando | `terraform apply C:\Nexum-Custodia\H08E-E005-20261117\a10-e005.tfplan` |
| Perfil | `nexum-tf-bootstrap` (credenciales temporales, sin claves en Git) |
| Resultado | **`Apply complete! Resources: 1 added, 2 changed, 0 destroyed.`** |
| Hora | `2026-11-17T03:02Z` (aprox.) |

### Verificación posterior (`terraform show -json` sobre el state remoto)

- `module.iam.aws_iam_role_policy.runtime_ses[0]` **existe** con `ses:SendEmail` acotado a
  `arn:aws:ses:us-east-2:713797602090:identity/nexumproyectomov@gmail.com`, condición
  `ses:FromAddress=nexumproyectomov@gmail.com` y `aws:SecureTransport=true`.
- `module.iam.aws_iam_role_policy.runtime` contiene `RESET#*` y `QUEUE#*` en la tabla y en `GSI1`,
  junto con las claves previas; el resto de acciones, recursos y condiciones no cambió.
- `module.redis.aws_elasticache_user.default_deshabilitado`: el proveedor sigue reportando
  `authentication_mode.type = no-password` tras la modificación; `access_string` permanece
  `off ~* -@all`. Es una normalización no persistente del proveedor, **sin cambio efectivo** de
  autenticación: el usuario `default` sigue deshabilitado.
- `module.redis.aws_elasticache_user.aplicacion`: `authentication_mode.type = password`,
  `password_count = 1`, `access_string` intacto; `user_group` conserva `nexum-app` y
  `nexum-default-off`; replication group con TLS y cifrado en reposo.
- Sin cambios en DynamoDB, ALB, CloudFront, VPC, Launch Template, ASG, buckets, secretos ni
  E001–E004. Coste incremental del apply: **0.0000 USD**.
