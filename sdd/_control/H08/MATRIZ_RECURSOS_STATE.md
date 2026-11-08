# I-070 — Matriz de recursos, state y coste

| Campo | Valor |
|---|---|
| Estado | `IMPLEMENTADO_LOCAL`; AWS `NOT_RUN` |
| Locking | S3 nativo (`use_lockfile`), Terraform >= 1.10; **sin** tabla DynamoDB |
| Fuente de coste | `infra/finops/MATRIZ_COSTOS_SERVICIOS_I000.md` y `precios_us-east-2.json` |

## 1. Recursos del bootstrap

| Recurso | Configuración | Protección | Coste I-000 |
|---|---|---|---|
| `aws_s3_bucket.state` | Nombre único; `prevent_destroy` | No se destruye por ahorro | Línea `S3_STATE` 3 GB-mes |
| `aws_s3_bucket_versioning.state` | `Enabled` | Recupera versiones sobrescritas | Incluido en la línea S3 |
| `aws_s3_bucket_server_side_encryption_configuration` | SSE-S3 AES256 | Sin CMK: sin revocación criptográfica inmediata (límite declarado) | 0 (SSE-S3 sin coste) |
| `aws_s3_bucket_public_access_block` | 4 flags en `true` | Sin exposición pública | 0 |
| `aws_s3_bucket_ownership_controls` | `BucketOwnerEnforced` | Sin ACLs heredadas | 0 |
| `aws_s3_bucket_policy` | Deniega `s3:*` sin TLS | Cifrado en tránsito obligatorio | 0 |
| `aws_s3_bucket_lifecycle_configuration` | Expira versiones no actuales (30 d por defecto) | Acota coste de versiones | Reduce la línea S3 |

**No se crea** tabla DynamoDB de lock: el mecanismo `dynamodb_table` del backend S3 está deprecado por
HashiCorp y una tabla adicional sería un recurso no presupuestado. El lock vive en
`envs/h08i/terraform.tfstate.tflock` dentro del mismo bucket.

## 2. Separación de states y custodia del bootstrap

| State | Clave en el bucket | Lock | Custodia |
|---|---|---|---|
| Bootstrap (crea el bucket) | `bootstrap/terraform.tfstate` tras la migración | `bootstrap/terraform.tfstate.tflock` | Copia cifrada fuera del repo con SHA-256 y `serial`/`lineage`; sin backend hasta que el bucket exista |
| Runtime h08i | `envs/h08i/terraform.tfstate` | `envs/h08i/terraform.tfstate.tflock` | Backend S3 con `use_lockfile` |
| Pre-migración del bootstrap | `infra/terraform/bootstrap/terraform.tfstate` local | — | Cifrado, dos copias, nunca en Git ni en artifacts |

El bootstrap **no** declara `backend` mientras el bucket no exista (evita dependencia circular). Tras verificarlo, su
state se migra a la clave `bootstrap/...` con `init -migrate-state` autorizado. La pérdida del state del bootstrap se
resuelve por **importación** con un `plan` de cero cambios, nunca recreando el bucket. Procedimiento completo en
`RUNBOOK_IAC.md` §2–§4.

## 3. Permisos IAM necesarios (diseño, no aplicado)

Contrato en `infra/terraform/policies/terraform-state-access.json.tftpl` (plantilla con `${state_bucket}` y
`${state_key}`); su forma exacta se valida en `tests/infra/terraform/test_terraform_backend_contrato.py`.

| Acción | Recurso | Condición |
|---|---|---|
| `s3:ListBucket` | bucket de state | `s3:prefix` = prefijo exacto del state + TLS |
| `s3:GetObject`, `s3:PutObject` | objeto `.../terraform.tfstate` | TLS |
| `s3:GetObject`, `s3:PutObject`, `s3:DeleteObject` | objeto `.../terraform.tfstate.tflock` | TLS |
| **Deny** `s3:DeleteObject`, `s3:DeleteObjectVersion` | objeto `.../terraform.tfstate` | explícito |

No se concede `s3:*`, ni acceso al bucket completo, ni `dynamodb:*`. Estos permisos se asignarán a los roles
de plan/apply cuando I-080/I-090 los definan; I-060 sólo aporta trust OIDC y `iam:PassRole` acotado.

## 4. Qué NO se crea aquí

- No se crea la tabla de negocio `NEXUM_CORE` (I-040) ni Redis (I-050).
- No se crean roles/políticas IAM (I-060) ni workflows (I-080/I-090).
- No se crea CMK de KMS ni tabla de lock DynamoDB.
- No se activa el backend del entorno: la plantilla `.example` está inactiva.

## 5. Riesgos abiertos

| Riesgo | Estado |
|---|---|
| State sin CMK propia | **ACEPTADO_CON_LIMITE** (declarado; revocación criptográfica no inmediata) |
| Coste real del state no medido | `NOT_VERIFIED`; dentro de la línea S3 de I-000 |
| Lock huérfano tras caída de una ejecución | Procedimiento en `RUNBOOK_IAC.md` §4; requiere autorización para `force-unlock` |
| `prevent_destroy` bloquea `destroy` del bootstrap | Intencional: retirar el state exige autorización humana y edición explícita |
| SCP/IAM reales | `AWS_SCP_NOT_VERIFIED` |
| Terraform < 1.10 en CI futuro | `required_version >= 1.10.0`; CI de I-080 debe fijar versión |
