# Gate H08-I — Migración del state del bootstrap al backend S3 (I-070)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/GATE_H08I_STATE_MIGRATION.md` |
| Fecha | 2026-11-10 |
| Spec | `sdd/02-infraestructura/I-070-terraform/` §3 del `RUNBOOK_IAC.md` |
| Estado | **PREPARADO — PENDIENTE DE AUTORIZACIÓN HUMANA** |
| Prerrequisito | Bootstrap **APLICADO** (`GATE_H08I_BOOTSTRAP_APPLY_REV2.md` §11) |
| Bucket destino | `nexum-h08i-tfstate-713797602090` (existe, verificado `2026-11-10T19:19:17Z`) |
| State a migrar | `infra/terraform/bootstrap/terraform.tfstate` · `serial 10` · SHA-256 `54D936ACD850D790B27A09B4B16E669FA55986ECE05A011D77DBC9C13A055690` |
| Custodia | copia cifrada DPAPI verificada (`terraform.tfstate.POSTAPPLY.dpapi`, SHA-256 `BA98F904…7D99`) |
| Recursos afectados | **0** creados, **0** destruidos (solo se mueve el state) |

## 1. Alcance autorizable

1. Declarar el backend S3 en la raíz `bootstrap` (`key = "bootstrap/terraform.tfstate"`,
   `use_lockfile = true`, `encrypt = true`, `region = "us-east-2"`).
2. Ejecutar **una vez** `terraform init -migrate-state` en `infra/terraform/bootstrap`,
   respondiendo `yes` a la copia del state local al backend.
3. Verificar que el state remoto contiene **las mismas 8 direcciones** que el local.
4. Declarar el backend en `envs/h08i` con `key = "envs/h08i/terraform.tfstate"` (clave distinta,
   mismo bucket) e inicializar **sin** recursos (el entorno aún no tiene state).

**No autoriza**: `terraform apply` del entorno, `destroy`, `import`, cambios de recursos AWS,
`state push` manual, borrar el state local antes de verificar el remoto, ni tocar I-010..I-150.

## 2. Permisos requeridos (hoy DENEGADOS)

La política deniega `terraform init -migrate-state*` y `terraform state*`. Excepción mínima a
registrar en `RFC-002`, **después** de los `DENY` y por esta operación:

```json
{"action": "shell", "resource": "terraform init -migrate-state", "effect": "ask"}
{"action": "tool.use", "resource": "shell:terraform init -migrate-state", "effect": "allow"}
```

`terraform state list` (verificación) seguirá **denegado**. La comprobación del contenido remoto
se hará con `aws s3api list-objects-v2 --bucket nexum-h08i-tfstate-713797602090 --prefix bootstrap/`
(requiere añadir esa lectura) y con el `terraform plan` de control posterior; si no se concede
ninguna de las dos, la verificación será `MIGRATION_UNVERIFIED` y **no** se declarará completada.

## 3. Procedimiento

1. Comprobar préstamos: bucket existe (`list-buckets`), state local SHA-256 sin cambios.
2. Escribir `infra/terraform/bootstrap/backend.tf` a partir de la plantilla.
3. `terraform init -migrate-state` (excepción activa) → confirmar `yes` a la copia.
4. Custodiar el nuevo state remoto: descargar `bootstrap/terraform.tfstate` a un directorio
   temporal **fuera del repositorio**, calcular SHA-256 y comparar direcciones con el local.
5. Verificar que el local **no** se borra hasta que el remoto esté confirmado; si difieren,
   restaurar desde la copia cifrada y **no** borrar el bucket.
6. Declarar backend de `envs/h08i` y `terraform init -backend=false` (permitido) para no
   conectarlo todavía; la conexión real ocurre en el Gate de la fase base.
7. Registrar `lineage`/`serial` remotos y retirar la excepción.

## 4. Rollback

- Si la migración falla: el state local sigue intacto (no se mueve antes de tiempo).
- Si el remoto queda corrupto: restaurar desde `terraform.tfstate.POSTAPPLY.dpapi` y
  **no** borrar el bucket ni el prefijo.
- Prohibido `destroy`, `force-unlock` sin Gate y reescribir el histórico del bucket.

## 5. Riesgos

| Riesgo | Mitigación |
|---|---|
| Pérdida del state al mover | Se conserva el local + copia cifrada; no se borra nada hasta verificar |
| Lock nativo S3 no probado en runtime | `use_lockfile = true`; dos escritores concurrentes son imposibles con el lock presente |
| Lectura AWS del objeto remoto no autorizada | Se pide explícitamente; sin ella, `MIGRATION_UNVERIFIED` |
| Confusión de claves | `bootstrap/…` y `envs/h08i/…` son claves distintas, con `.tflock` independiente |

## 6. Autorización requerida

```
APRUEBO_GATE_I070_MIGRACION_STATE
```

Además, si se concede la lectura del objeto remoto, añadir:

```
APRUEBO_PERMISO_S3_LIST_STATE_PREFIX
```

## 7. Después de este Gate (orden obligatorio)

1. **Gate de identidades IAM + RFC-004**: rol de gestión IaC (`gestor_iac_arn`) y rol runtime ya
   definidos en I-060/RFC-004; sin inventar ARNs ni contraseñas; el bundle de secretos se publica
   al bucket cuando exista el rol.
2. **Gate del plan de fase base** (`desplegar_aplicacion = false`): red, DynamoDB, Redis,
   secretos y roles; **sin** EC2 ni CloudFront; plan exacto revisado y su propio SHA-256.
3. **Gate de TLS/H-01, RFC-006 (WAF) y RFC-007 (SNS/CMK)** antes de la fase de cómputo.
4. **Gate de fase de aplicación** (`desplegar_aplicacion = true`): 2 × EC2, ALB interno,
   CloudFront, targets sanos, readiness con DynamoDB y Redis.
5. **Release E001**: publicar el artefacto `0.1.0-e001` (SHA-256 `50b366ed…eebe4`) mediante CD
   asistido y comprobar `POST /api/v1/auth/register` → 201, persistencia en DynamoDB e
   idempotencia sobre HTTPS público.

Este documento no se autoaprueba.
