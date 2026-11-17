# Gate H08-E / E006 — Apply del plan Terraform (LT v8 + IAM)

**ESTADO:** `APPLIED + AWS_DEPLOYED + USER_ACCEPTED_CONFIRMED` (2026-11-17). El plan autorizado fue aplicado con el resultado real registrado en §7; no se ejecutó ningún otro cambio de infraestructura.

## 1. Base

| Campo | Valor |
|---|---|
| `main` | `578b0db3b9bb765afc981a86b23c6ade10ed80ea` (PR #71 MERGED) |
| CI de `main` | Go Backend CI run `44116124273` **SUCCESS**; SDD `44116124463` SUCCESS |
| Release | `v0.13.0+578b0db3b9bb` |
| Artefacto | `nexum-backend-linux-arm64`, 20 240 393 B, SHA-256 `6b87752f35f335b79d2e0cf98f6ae2cda752a1bc23928ab98edbd1cbec6bb522` |
| Unidad | `nexum-backend.service`, SHA-256 `6fa4d5fb051914102b1bc3b6cd2814e881bf02bb26df1d938f4f0d46e15705cc` |
| S3 | `s3://nexum-h08i-artefactos-713797602090/releases/v0.13.0+578b0db3b9bb/` (SSE AES256, `If-None-Match`, checksums verificados) |

## 2. Plan

| Campo | Valor |
|---|---|
| Archivo | `C:\Nexum-Custodia\H08E-E006-20261117\e006-lt-iam.tfplan` |
| **SHA-256** | **`90988C7CB5D49F62498C6FDF00DF7AC5BA6349D034D392060880DD52598C2D63`** |
| Tamaño | 167 960 bytes |
| Terraform | 1.16.6 · provider `hashicorp/aws` 5.100.0 |
| Backend | `s3://nexum-h08i-tfstate-713797602090/envs/h08i/terraform.tfstate` |
| Acciones | **0 add · 4 change · 0 destroy · 0 replace · 0 import** |

### Cambios exactos

1. `module.computo[0].aws_launch_template.backend` — `user_data` con
   `BINARIO_URI=.../v0.13.0+578b0db3b9bb/nexum-backend-linux-arm64`,
   `BINARIO_SHA=6b87752f...bb522`, `UNIDAD_SHA=6fa4d5fb...05cc` (verificado decodificando el plan).
2. `module.iam.aws_iam_role_policy.runtime` — 19 ARN totales; **16 objetos S3** de artefactos
   (E001, v0.8.0, v0.9.0, v0.10.0, v0.11.0, v0.12.0, v0.12.1 y v0.13.0), conservando el rollback.
3. `module.computo[0].terraform_data.invariantes_computo` — revalidación de invariantes.
4. `module.iam.terraform_data.iam_preconditions` — revalidación de IAM.

### Sin cambios (no-op verificado en el plan)

`module.dynamodb.aws_dynamodb_table.core`, `module.iam.aws_iam_role.runtime`,
`module.iam.aws_iam_instance_profile.runtime`, los cuatro security groups, las subredes
privadas `us-east-2a/2b`, la VPC y el bucket de secretos. No se toca SES, Redis, CloudFront,
ALB, el endpoint de SES ni el bundle RFC-004.

## 3. Seguridad

- `0 destroy`, `0 replace`, `0 import`: no hay operaciones destructivas ni recreaciones.
- El plan se generó con `-target` sobre los cuatro recursos de release para excluir el drift
  conocido de normalización de Redis; no se aplica ningún cambio ajeno a E006.
- La política IAM de DynamoDB ya cubre `GetItem`/`UpdateItem`/`TransactWriteItems` sobre
  `USER#*` y `RESET#*`; no se amplía ningún permiso.
- Coste incremental del plan: **0.0000 USD** (Launch Template e IAM inline no generan cargos).

## 4. Frase autorizada y ejecutada

```
APRUEBO_APPLY_LT_IAM_E006_SHA256_90988C7CB5D49F62498C6FDF00DF7AC5BA6349D034D392060880DD52598C2D63
```

## 5. Procedimiento ejecutado tras el apply

1. Verificar `latest_version` del LT y el estado remoto.
2. Instance refresh con `MinHealthyPercentage=100`, `MaxHealthyPercentage=150`,
   `InstanceWarmup=120`; capacidad restaurada a **2/2/2**.
3. Confirmar dos instancias `InService/Healthy` en LT nuevo y dos targets `healthy`.
4. Pruebas HTTPS E006 y regresión dirigida E001–E005.

## 6. Pruebas Postman para la aceptación del responsable

Base URL: `https://d2lpmqy5w6x9es.cloudfront.net/api/v1`. No guardar tokens ni contraseñas en
Git, logs ni evidencias. Cuerpos exactos según OpenAPI.

### A — E002 login ANTES del cambio

```http
POST /auth/login
Content-Type: application/json

{"email":"leobarrera445@gmail.com","password":"CONTRASENA_ACTUAL"}
```

Guardar `accessToken` y `refreshToken` de esta sesión para comprobar su revocación.

### B — E005 solicitar recuperación

```http
POST /auth/password/forgot
Content-Type: application/json

{"email":"leobarrera445@gmail.com"}
```

Esperado `202 Accepted`. Copiar del correo NUEVO solo el token.

### C — E006 confirmar el cambio

```http
POST /auth/password/reset
Content-Type: application/json

{"token":"TOKEN_NUEVO_DEL_CORREO","newPassword":"NUEVA_CONTRASENA_DE_PRUEBA"}
```

Esperado `200 {"status":"accepted","requestId":"<uuid>"}`.

### D — Efectos reales

1. E002 con la contraseña **anterior** → `401 UNAUTHORIZED`.
2. E002 con la contraseña **nueva** → `200` (nuevos `accessToken`/`refreshToken`).
3. Repetir E006 con el **mismo token** → `409 CONFLICT`.
4. E003 con el `refreshToken` **anterior**:

```http
POST /auth/refresh
Content-Type: application/json

{"refreshToken":"REFRESH_ANTERIOR"}
```

→ `401 UNAUTHORIZED`.

5. `POST /auth/logout` con el `accessToken` **anterior** (`Authorization: Bearer ...`, sin
   cuerpo) → `401 UNAUTHORIZED` (época desalineada; no cierra la sesión nueva).
6. E003 con el `refreshToken` **nuevo** → `200` con rotación; E002 nuevo sigue funcionando.

Registrar solo códigos y `requestId`; nunca tokens ni contraseñas.

## 7. Ejecución real del apply y despliegue

| Campo | Valor |
|---|---|
| Autorización | `APRUEBO_APPLY_LT_IAM_E006_SHA256_90988C7CB5D49F62498C6FDF00DF7AC5BA6349D034D392060880DD52598C2D63` |
| Hash recalculado antes de aplicar | Idéntico al autorizado |
| Resultado | **`0 added, 4 changed, 0 destroyed`** |
| Launch Template | `lt-05445f32a7ee8a2f7` **v8** (release E006 verificado en `user_data`) |
| IAM runtime | 19 ARN totales, **16 objetos S3** (todos los releases) |
| Sin cambios | DynamoDB, SES, Redis, CloudFront, ALB, VPC, subredes, SGs, secretos |

### Instance refresh

- `1a348a2b-5248-4e5b-80f4-fb9f1019c72b` iniciado con `MinHealthyPercentage=100`,
  `MaxHealthyPercentage=150`, `InstanceWarmup=120`; quedó bloqueado >15 min en
  `WaitingForInstanceWarmup` pese a 3 instancias `Healthy` y se canceló.
- Recuperación controlada: protección del nodo v8, retirada de protección de los v7,
  descenso temporal de capacidad, terminación de ambos v7 y sustitución por un segundo v8.
- Capacidad `t4g.micro` insuficiente en `us-east-2b` (dos intentos fallidos): ASG restringido
  temporalmente a `us-east-2a` y subredes restauradas después.

### Flota final

| Instancia | AZ | LT | Estado |
|---|---|---|---|
| `i-01b1a4606bd773838` | us-east-2a | v8 | `InService/Healthy` |
| `i-07c1c2facc51aa2a1` | us-east-2a | v8 | `InService/Healthy` |

ASG **Min 2 / Desired 2 / Max 2**; subredes `us-east-2a` y `us-east-2b` restauradas;
**2 targets healthy** en `nexum-h08i-tg-backend`. Limitación declarada: sin redundancia
multi-AZ efectiva mientras `us-east-2b` no tenga capacidad.

## 8. Verificación HTTPS y aceptación

| Prueba | Resultado |
|---|---|
| E006 JSON malformado | **400** |
| E006 campo desconocido | **400** |
| E006 contraseña corta | **422** |
| E006 token inexistente | **400** |
| E006 con nodos v7 (antes del release) | 404 (demuestra el cambio de versión) |
| `GET /api/v1/health/live` | **200** |
| E001 `register` inválido | 400 |
| E002 `login` desconocido | 401 |
| E003 `refresh` inválido | 401 |
| E004 `logout` sin Bearer | 401 |
| E005 `password/forgot` inválido | 422 |

- **`E006_USER_ACCEPTED = CONFIRMED`** (2026-11-17): el responsable declaró haber comprobado
  satisfactoriamente en Postman el restablecimiento real de contraseña.
- **Pendiente de evidencia individual:** 409 de reutilización de token, 401 de access/refresh
  anteriores y concurrencia real en AWS. No se declaran PASS ni se confunden con un fallo.
- No se almacenan tokens, contraseñas ni credenciales en este documento ni en el repositorio.
