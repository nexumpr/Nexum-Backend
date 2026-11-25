# Inspección confidencial del ZIP de secretos — `BLOCKED_AUTHORIZATION`

**Estado:** `BLOCKED_AUTHORIZATION` — no se leyó el contenido.
**Fecha:** 2026-11-24. **Autorización de la tarea:** inspección confidencial de
`nexum/secrets/terraform-h08i.zip` con la identidad autorizada, sin evadir
denegaciones y sin sacar valores del entorno temporal.

## 1. Identidad y metadatos confirmados (solo lectura)

- Identidad: cuenta `713797602090`, rol
  `assumed-role/AccountFullAccessRole`, región `us-east-2`.
- Objeto: `nexum/secrets/terraform-h08i.zip` en
  `nexum-h08i-secretos-713797602090`.
- `list-objects-v2` (permitido) devuelve:
  - tamaño **92 110 bytes**;
  - `LastModified` **2026-11-23T01:06:54Z**;
  - `ETag` `80d3219256581c9547c1b50e3e5c20cd` (metadato, no contenido);
  - `ChecksumAlgorithm` `CRC64NVME`, `StorageClass` `STANDARD`.
- **SHA-256 del contenido: `NOT_COMPUTED`** (no se descargó).
- **Clasificación del contenido: `UNKNOWN`** — no se puede afirmar ni negar que
  contenga `tfstate`, `tfvars`, planes, credenciales, claves FCM u otros
  secretos.

## 2. Denegación exacta

`aws s3api head-object --bucket nexum-h08i-secretos-713797602090 --key
nexum/secrets/terraform-h08i.zip --profile nexum --region us-east-2` devolvió:

```text
permission.rejected — Permission denied: shell
```

La denegación es de la **política de herramientas de OpenCode** (la regla
`aws s3api head-object` no está autorizada y `aws *` es `deny`), no de IAM de
AWS. `aws sts get-caller-identity` sí funcionó, por lo que la sesión AWS es
válida.

No se intentó `get-object`, `s3 cp`, PowerShell/.NET HTTP, `aws s3api
select-object-content` ni ninguna ruta alternativa: sería evadir el control.

## 3. Autorización puntual solicitada

Para completar la tarea sin ampliar permisos de forma permanente, se solicita
una excepción temporal y literal:

```text
shell: aws s3api head-object *        -> ask
shell: aws s3api get-object *         -> ask
```

Uso previsto, una sola vez:

1. `head-object` para confirmar tamaño, `LastModified` y checksum disponible.
2. `get-object` a
   `%TEMP%\nexum-zip-audit\terraform-h08i.zip` (nunca al repositorio).
3. SHA-256 local con `Get-FileHash`.
4. Listado de entradas con `System.IO.Compression.ZipFile` **sin extraer ni
   ejecutar** nada; sólo nombres, tamaños y fechas.
5. Clasificación agregada: `tfstate`, `tfvars`, planes, credenciales, claves
   FCM, otros. Sin imprimir valores.
6. Borrado del temporal al terminar, o conservación cifrada si el operador lo
   pide. No se borra el objeto S3 ni se rotan credenciales.

La excepción debe retirarse al concluir y `opencode.json` debe quedar idéntico a
`HEAD` (`git diff` vacío). Sin esta autorización, el estado permanece
`BLOCKED_AUTHORIZATION`.
