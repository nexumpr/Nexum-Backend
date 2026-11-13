# Gate H08-E / E003 — Publicación S3 del release `v0.10.0+1d0c2650daab`

**ESTADO:** `PREPARADO — NO EJECUTADO`. Requiere frase humana explícita.

| Campo | Valor |
|---|---|
| Frase requerida | `APRUEBO_PUBLICACION_S3_E003_V0.10.0` |
| Fecha de preparación | 2026-11-13 |
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
