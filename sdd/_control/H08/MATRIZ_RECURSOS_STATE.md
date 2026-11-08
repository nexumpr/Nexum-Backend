# I-070 — Matriz de recursos, state y coste

| Campo | Valor |
|---|---|
| Estado | `IMPLEMENTADO_LOCAL`; AWS `NOT_RUN` |
| Fuente de coste | `infra/finops/MATRIZ_COSTOS_SERVICIOS_I000.md` y `precios_us-east-2.json` |

## 1. Recursos del bootstrap

| Recurso | Configuración | Protección | Coste I-000 |
|---|---|---|---|
| `aws_s3_bucket.state` | Nombre único; `prevent_destroy` | No se destruye por ahorro | Línea `S3_STATE` 3 GB-mes |
| `aws_s3_bucket_versioning.state` | `Enabled` | Recupera versiones sobrescritas | Incluido en la línea S3 |
| `aws_s3_bucket_server_side_encryption_configuration` | SSE-S3 AES256 | Sin CMK: no hay revocación criptográfica inmediata; límite declarado | 0 (SSE-S3 sin coste) |
| `aws_s3_bucket_public_access_block` | 4 flags en `true` | Sin exposición pública | 0 |
| `aws_s3_bucket_ownership_controls` | `BucketOwnerEnforced` | Sin ACLs heredadas | 0 |
| `aws_s3_bucket_policy` | Deniega `s3:*` sin TLS | Cifrado en tránsito obligatorio | 0 |
| `aws_s3_bucket_lifecycle_configuration` | Expira versiones no actuales (30 d por defecto) | Acota coste de versiones | Reduce la línea S3 |
| `aws_dynamodb_table.lock` | `LockID` (S), on-demand, SSE, sin PITR/backups, `prevent_destroy` | Locks efímeros | Dentro de las líneas DynamoDB; consumo real **no medido** |

## 2. Qué NO se crea aquí

- No se crea la tabla de negocio `NEXUM_CORE` (I-040) ni Redis (I-050).
- No se crean roles/políticas IAM (I-060) ni workflows (I-080/I-090).
- No se crea CMK de KMS: coste fijo no presupuestado.
- No se modifica el backend del entorno: la plantilla `.example` está inactiva.

## 3. Riesgos abiertos

| Riesgo | Estado |
|---|---|
| Bucket con datos sensibles sin CMK | **ACEPTADO_CON_LIMITE** (declarado; revocación criptográfica no inmediata) |
| Lock incompatible o drift de versión de Terraform | `required_version >= 1.4,<2.0`; lock file versionado por raíz |
| Coste del state y de la tabla de lock no medido | `NOT_VERIFIED`; dentro de líneas I-000 |
| `prevent_destroy` bloquea `destroy` del bootstrap | Intencional: retirar el state exige autorización humana y edición explícita |
| SCP/IAM reales | `AWS_SCP_NOT_VERIFIED` |
