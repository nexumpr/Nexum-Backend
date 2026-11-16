# Gate H08-E / E003 — Publicación S3 del release `v0.10.0+1d0c2650daab`

**ESTADO:** `AUTORIZADO — BLOQUEADO_PERMISO_OPENCODE` (2026-11-13T19:04Z).
La frase `APRUEBO_PUBLICACION_S3_E003_V0.10.0` fue concedida, pero la política de herramientas de
OpenCode deniega la ejecución: **no se escribió ningún objeto**.

| Campo | Valor |
|---|---|
| Frase recibida | `APRUEBO_PUBLICACION_S3_E003_V0.10.0` (2026-11-13) |
| Cuenta AWS | `713797602090` |
| Región | `us-east-2` |
| Bucket | `nexum-h08i-artefactos-713797602090` (existente, sin cambios) |
| Release ID | `v0.10.0+1d0c2650daab` |
| Commit de código | `1d0c2650daab07e20a52df518d9a69cbca51cda0` (`main`) |
| Run de CI de origen | `43485092835` (Go Backend CI de `main`, SUCCESS) |
| SBOM | `sbom-spdx-1d0c2650daab07e20a52df518d9a69cbca51cda0` (`sbom.spdx.json`, 85 571 B) |
| Identidad publicadora | `arn:aws:iam::713797602090:role/nexum-h08i-artefactos-publicador` (asunción desde el operador) |
| Coste | 0.0000 USD incremental (objetos en bucket ya presupuestado) |

## 1. Objetos exactos a publicar

| # | Clave S3 | Bytes | SHA-256 |
|---|---|---|---|
| 1 | `releases/v0.10.0+1d0c2650daab/nexum-backend-linux-arm64` | 19 127 165 | `734de969570502099d452438fd606f1646d35f225459a450a51cfa567514723c` |
| 2 | `releases/v0.10.0+1d0c2650daab/nexum-backend.service` | 1 357 | `6fa4d5fb051914102b1bc3b6cd2814e881bf02bb26df1d938f4f0d46e15705cc` |

- Binario: artefacto `binario-1d0c2650…` del run `43485092835`, verificado contra su
  `dist/binario.sha256` (coincide) y contra el fichero descargado.
- Unidad: `infra/systemd/nexum-backend.service` del propio commit `1d0c265`; idéntica byte a
  byte a la que ya ejecuta E002 (`6fa4d5fb…05cc`).
- **No se publica nada más**: ni otros releases, ni el bundle de secretos, ni SBOM en S3 (el
  SBOM vive como artefacto de CI).

## 2. Operación exacta prevista

```text
aws sts assume-role --role-arn arn:aws:iam::713797602090:role/nexum-h08i-artefactos-publicador \
  --role-session-name e003-release-<ts>            # CD_ASISTIDO
aws s3api put-object --bucket nexum-h08i-artefactos-713797602090 \
  --key releases/v0.10.0+1d0c2650daab/nexum-backend-linux-arm64 \
  --body <custodia>/nexum-backend-linux-arm64 \
  --server-side-encryption AES256 --if-none-match '*' \
  --checksum-algorithm SHA256 --checksum-sha256 <base64>
aws s3api put-object ... --key releases/v0.10.0+1d0c2650daab/nexum-backend.service ...
```

Condiciones que impone la política del bucket (ya vigente, no se modifica):
`PutObject` solo al rol publicador; solo bajo `releases/`; **SSE `AES256` obligatorio**; sin
lectura ni borrado para el publicador; prohibido a cualquier otro principal.

## 3. Verificación posterior (sin depender del publicador)

El publicador **no** puede leer objetos (`DenegarLecturaAlPublicador`). La verificación se
hará con el operador mediante `s3api list-objects-v2 --prefix releases/v0.10.0+1d0c2650daab/`
comprobando `Size` y `ChecksumSHA256` de cada objeto contra la tabla del §1. Cualquier
discrepancia detiene el despliegue y **no** se aplica el Launch Template.

## 3 bis. Resultado del preflight de permisos (2026-11-13)

- `iam simulate-principal-policy` devuelve **`explicitDeny` incluso para el rol runtime
  leyendo `releases/v0.9.0+…`, permiso que en producción funciona** (las instancias activas
  descargan ese objeto en cada arranque). Conclusión: la simulación **no evalúa de forma fiel**
  la condición `aws:PrincipalArn` de `DenegarAPrincipalsNoAutorizados` y **no sirve** como
  preflight de este bucket. No se declara permiso verificado por simulación.
- El operador **sí** tiene `ListBucket` (listado de `releases/` correcto), pero la política del
  bucket le deniega escritura por diseño.
- Por tanto, la prueba real de `PutObject` **es la propia publicación**: si el rol no pudiera
  escribir, AWS devuelve `AccessDenied` y **nada cambia** (no hay efecto parcial). El script de
  publicación aborta al primer error y no continúa con el segundo objeto.

## 4. Rollback

- Revertir el despliegue es volver el Launch Template a su versión previa (v3, release
  `v0.9.0+7d1ebf3d5357`) y repetir el `instance refresh`; el objeto de E002 permanece intacto.
- El release nuevo es inmutable (`If-None-Match: *`): si ya existiera con otro digest, la
  operación falla y se detiene todo.
- No se borra ningún objeto: el publicador no puede y no se solicita permiso de borrado.

## 5. Fuera de alcance

`terraform apply`, Launch Template, ASG, instance refresh, IAM, Redis, DynamoDB, CloudFront,
ALB, VPC, bundle de secretos y cualquier prueba destructiva. Cada uno exige su propio Gate.

## 6. Criterio de cierre de este Gate

Los dos objetos existen con tamaño y digest exactos, y `memoria.md` registra el resultado
real. Sólo entonces se prepara el Gate del Launch Template (que incluirá el cambio IAM mínimo
para que el rol runtime pueda leer los dos ARN nuevos, conservando los de E001, v0.8.0 y
v0.9.0 para rollback).

## 7. Bloqueo real registrado (2026-11-13T19:04Z)

**Estado: `BLOQUEADO_PERMISO_OPENCODE`. Cero objetos escritos.**

Comprobaciones realizadas antes de escribir (todas correctas):

| Comprobación | Resultado |
|---|---|
| Artefacto local | 19 127 165 B, SHA-256 `734de969…723c` (coincide con el autorizado) |
| Unidad local | 1 357 B, SHA-256 `6fa4d5fb…05cc` (coincide con el autorizado) |
| Commit de `main` | `1d0c2650daab07e20a52df518d9a69cbca51cda0` |
| Run canónico | `43485092835` (binario + SBOM descargados y verificados) |
| Prefijo destino `releases/v0.10.0+1d0c2650daab/` | **vacío** (`ListObjectsV2` sin `Contents`) |
| Releases previos | `e001-d9f0696`, `v0.8.0+40f64b6653ec`, `v0.9.0+7d1ebf3d5357` intactos |
| Identidad AWS | cuenta `713797602090`; una llamada a `sts` devolvió `429 CreateOAuth2Token rate exceeded` (transitorio); el listado S3 funcionó |

Bloqueo exacto: en `opencode.json`, la regla base `shell` **`aws *` = `deny`** y las reglas
**`aws * put-*` = `deny`** cubren `s3api put-object`; `aws sts assume-role` fue rechazado con
`permission.rejected`. El rol publicador **no puede asumirse** desde OpenCode con la política
vigente. Conforme a las reglas del proyecto **no** se editó `opencode.json`, no se usó
`AccountFullAccessRole` para escribir (además la política del bucket lo deniega) y no se
cambió de identidad.

Excepción mínima solicitada por el procedimiento de RFC-002 (añadir tras los DENY, retirar al
concluir):

- `shell` `aws sts assume-role *` → `ask`
- `shell` `aws s3api put-object *` → `ask`

Para el Gate siguiente (LT/IAM) harán falta además, como excepciones literales y temporales:
`terraform init -backend=true*`, `terraform plan *` y `terraform show *` en `ask`.

**No se declara** `E003_ARTIFACT_PUBLISHED`.
