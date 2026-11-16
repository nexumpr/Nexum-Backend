# Gate H08-E / E004 — Publicación S3 del release `v0.11.0+1e03d5cd12a7`

**ESTADO:** `PUBLICADO Y VERIFICADO` (2026-11-16T02:41Z).

| Campo | Valor |
|---|---|
| Frases recibidas | `APRUEBO_EXCEPCION_RFC002_PUBLICACION_S3_E004` · `APRUEBO_PUBLICACION_S3_E004_V0.11.0` |
| Cuenta AWS | `713797602090` |
| Región | `us-east-2` |
| Bucket | `nexum-h08i-artefactos-713797602090` (existente, sin cambios) |
| Release ID | `v0.11.0+1e03d5cd12a7` |
| Commit de código | `1e03d5cd12a77d80e4f0f478ed22c885a7522683` (`main`, merge del PR #63) |
| Run de CI de origen | `43791713302` (Go Backend CI de `main`, SUCCESS) |
| Identidad publicadora | `arn:aws:sts::713797602090:assumed-role/nexum-h08i-artefactos-publicador/e004-release-20261115204140` |
| Coste incremental | 0.0000 USD (objetos en bucket ya presupuestado) |

## 1. Objetos publicados

| # | Clave S3 | Bytes | SHA-256 | ETag (MD5) |
|---|---|---|---|---|
| 1 | `releases/v0.11.0+1e03d5cd12a7/nexum-backend-linux-arm64` | 19 144 109 | `fd67708a3a36f8f6010d2b74997b555097e224ae86ed7c8e8752f5016ebaa12d` | `a2039f6193f50484337fb97c999be597` |
| 2 | `releases/v0.11.0+1e03d5cd12a7/nexum-backend.service` | 1 357 | `6fa4d5fb051914102b1bc3b6cd2814e881bf02bb26df1d938f4f0d46e15705cc` | `40f331d10b7f4c338948d79d14f02d44` |

## 2. Controles aplicados

- **Rol dedicado**: se asumió únicamente `nexum-h08i-artefactos-publicador`. **No** se usó
  `AccountFullAccessRole` para escribir; la política del bucket lo habría denegado además.
- **Inmutabilidad**: `--if-none-match "*"` en ambos objetos. El prefijo destino estaba **vacío**
  antes de publicar (`ListObjectsV2` sin `Contents`), condición habilitante comprobada.
- **Cifrado**: `--server-side-encryption AES256`; la respuesta `PutObject` devolvió
  `ServerSideEncryption=AES256` en los dos objetos. La política del bucket **deniega**
  `PutObject` sin SSE-S3, de modo que el éxito de la operación lo confirma por segunda vía.
- **Integridad**: `--checksum-algorithm SHA256` con el digest calculado en la propia ejecución
  (no de un valor copiado). El `ChecksumSHA256` devuelto coincide con el local en ambos casos:
  `/Wdwijo2+PYBDSt0mXtVUJfiJK6G7XyOh1L1AW66oS0=` y `b6TV+wUZFBArG8O2zSgU6IG/Arsm3x2Tj08NRuFXBcw=`.
- **Sin secretos**: las credenciales STS efímeras se usaron sólo en memoria del proceso; no se
  imprimieron, no se escribieron en archivos y no se registraron.

## 3. Verificación independiente (operador, solo lectura)

`list-objects-v2 --prefix releases/v0.11.0+1e03d5cd12a7/` devuelve exactamente dos objetos con
`Size` 19 144 109 y 1 357, y `ETag` idéntico al **MD5 calculado localmente** sobre los mismos
ficheros. Dos algoritmos distintos (SHA-256 en la respuesta de escritura, MD5 en el listado)
coinciden sobre los mismos bytes.

**Limitación declarada:** el CLI instalado **no admite** `--checksum-mode` en `list-objects-v2`
(`Unknown options: --checksum-mode`), y `head-object`/`get-object-attributes` están denegados al
operador por `DenegarAPrincipalsNoAutorizados`. Por tanto el `ChecksumSHA256` **no se puede
releer desde S3** con las herramientas y permisos actuales; la evidencia de integridad es la
respuesta de `PutObject` más la coincidencia de tamaño y MD5 del listado.

## 4. Releases previos (intactos)

`releases/e001-d9f0696/` (12 452 000 B), `releases/v0.8.0+40f64b6653ec/` (18 721 843 B),
`releases/v0.9.0+7d1ebf3d5357/` (19 041 073 B) y `releases/v0.10.0+1d0c2650daab/` (19 127 165 B),
cada uno con su `nexum-backend.service` de 1 357 B. Ningún objeto borrado ni sobrescrito: el
publicador no tiene permiso de lectura ni de borrado.

## 5. Permisos temporales

- Añadidas **sólo** dos reglas `ask` (`aws sts assume-role *`, `aws s3api put-object *`) y sus
  dos `tool.use` `allow`, al final de sus arrays, es decir **después** del `deny` de
  `aws * put-*`. No se convirtió `aws *` en `allow` ni se relajó ningún `deny`.
- Precedencia comprobada empíricamente antes de usarla: reglas posteriores al `deny` de
  `aws *` (índice 76) ya se ejecutaban con normalidad (`list-objects-v2` índice 102,
  `autoscaling` índices 94-…).
- **Retiradas al concluir.** Estado final verificado: `permissions` 161 y `policies` 43, sin
  `assume-role` ni `put-object`, con el `deny` de `aws *`, `aws * delete-*` y `aws * put-*`
  intactos. Se conservan las diez excepciones de EC2/ASG/ELBv2 porque el rollout las necesita.
- La excepción **no amplió** la seguridad de AWS: sólo habilitó la llamada de la herramienta. La
  autorización real sigue en IAM y en la política del bucket (publicador único, prefijo
  `releases/`, SSE obligatorio, sin lectura ni borrado).

## 6. Fuera de alcance

`terraform apply`, Launch Template, IAM, ASG, `instance refresh`, CloudFront, ALB, DynamoDB,
Redis, secretos y cualquier prueba destructiva. Cada uno exige su propio Gate.

**No se declara** `E004_DEPLOYED` ni `E004_AWS_VERIFIED`: publicar en S3 no despliega. El
siguiente paso es el plan acotado de Launch Template + IAM runtime para que las instancias
puedan leer los dos objetos nuevos.
