# Inspección confidencial del ZIP de secretos — `BLOCKED_AUTHORIZATION`

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/INSPECCION_ZIP_SECRETOS_BLOQUEADA.md` |
| Versión | 2.0.0 |
| Fecha | 2026-11-24 |
| Objeto | `s3://nexum-h08i-secretos-713797602090/nexum/secrets/terraform-h08i.zip` |
| Estado | **`BLOCKED_AUTHORIZATION`** — el contenido **no** se leyó |
| Causa | Política de herramientas de OpenCode (`shell`), no IAM de AWS |
| Contenido clasificado | **`UNKNOWN`** |
| SHA-256 del contenido | **`NOT_COMPUTED`** |

> No se descargó, extrajo, listó ni ejecutó nada del ZIP. No se borró el objeto,
> no se rotaron credenciales y no se modificó IAM, bucket ni política. Este
> documento registra el bloqueo, el comando mínimo que lo desbloquearía y el
> procedimiento seguro para cuando exista autorización.

## 1. Metadatos confirmados (sólo lectura)

Obtenidos con `list-objects-v2`, que **sí** está permitido:

| Dato | Valor |
|---|---|
| Bucket | `nexum-h08i-secretos-713797602090` |
| Clave | `nexum/secrets/terraform-h08i.zip` |
| Tamaño | **92 110 bytes** |
| `LastModified` | **2026-11-23T01:06:54Z** |
| `ETag` | `80d3219256581c9547c1b50e3e5c20cd` |
| `ChecksumAlgorithm` | `CRC64NVME` |
| `StorageClass` | `STANDARD` |
| Identidad de lectura | cuenta `713797602090`, `assumed-role/AccountFullAccessRole`, `us-east-2` |

Notas:

- El `ETag` de un objeto pequeño subido en una sola parte sin KMS suele ser su
  **MD5**; no se usa como verificación de integridad hasta descargarlo. El
  checksum anunciado es **CRC64NVME**, no SHA-256.
- Que el objeto exista y su tamaño **no** dice qué contiene. Un ZIP de 92 KB
  puede llevar un `tfstate` comprimido, un `tfvars`, un plan binario o un JSON
  de cuenta de servicio; también puede ser un archivo de trabajo sin secretos.

## 2. Intento de esta sesión y denegación exacta

Comando ejecutado **una sola vez** (sin reintentos, sin variantes):

```text
aws s3api head-object --bucket nexum-h08i-secretos-713797602090 --key nexum/secrets/terraform-h08i.zip --profile nexum --region us-east-2
```

Respuesta exacta de la herramienta:

```json
{"error":{"type":"permission.rejected","message":"Permission denied: shell"},"content":[]}
```

Interpretación:

- La denegación es de la **política de herramientas de OpenCode**: la regla
  general `aws *` es `deny` y no existe una excepción para `aws s3api
  head-object`.
- **No** es un `AccessDenied` de IAM: la sesión AWS está operativa (una lectura
  `list-objects-v2` funcionó previamente). El mensaje es de `permission.rejected`
  del entorno, no del servicio S3.
- No se intentó `get-object`, `s3 cp`, `presign`, `select-object-content`,
  PowerShell/.NET HTTP, SDK, ni ninguna ruta equivalente. Hacerlo sería eludir
  el control (R-021) y queda prohibido.
- Como el intento falló, **no** se ejecutó el `list` del contenido ni se creó
  ningún temporal.

## 3. Permiso que falta (dos capas distintas)

| Capa | Qué falta | Quién lo concede |
|---|---|---|
| Herramienta (OpenCode) | Regla `shell: aws s3api head-object *` → `ask` y `shell: aws s3api get-object *` → `ask`, después de los `deny` | Responsable, en `opencode.json`, con Gate/RFC-002; **temporal** |
| IAM de AWS | `s3:GetObject` sobre **la clave exacta** `arn:aws:s3:::nexum-h08i-secretos-713797602090/nexum/secrets/terraform-h08i.zip` para la identidad de custodia | Propietario de la cuenta / rol de custodia, por cambio IAM autorizado |

Importante:

- `s3:ListBucket` **no** basta para `HeadObject`/`GetObject`: el listado puede
  funcionar y la lectura fallar con `AccessDenied`.
- La identidad que ejecuta la custodia **no** debe ser el rol de runtime ni el
  publicador de artefactos. El bucket tiene una política que deniega acciones a
  principals no autorizados y exige TLS.
- No se propone `s3:*`, ni `Resource: "*"`, ni acceso al prefijo completo: sólo
  la clave exacta y sólo `GetObject`.
- Si el objeto está cifrado con SSE-S3 (lo está), `GetObject` requiere además
  `kms` sólo si se usara CMK; no es el caso.

## 4. Comando mínimo exacto para el custodio

Con la autorización concedida, y **sólo** entonces:

```powershell
# 1) Metadatos (sin descargar)
aws s3api head-object `
  --bucket nexum-h08i-secretos-713797602090 `
  --key nexum/secrets/terraform-h08i.zip `
  --profile nexum --region us-east-2

# 2) Descarga a un temporal privado, nunca al repositorio
$destino = Join-Path $env:TEMP 'nexum-zip-audit'
New-Item -ItemType Directory -Force -Path $destino | Out-Null
aws s3api get-object `
  --bucket nexum-h08i-secretos-713797602090 `
  --key nexum/secrets/terraform-h08i.zip `
  "$destino\terraform-h08i.zip" `
  --profile nexum --region us-east-2

# 3) Integridad
Get-FileHash "$destino\terraform-h08i.zip" -Algorithm SHA256
```

Criterios:

- El `ContentLength` de `head-object` debe ser **92 110**; si no, el objeto
  cambió y se detiene la inspección.
- El `ETag` debe ser `80d3219256581c9547c1b50e3e5c20cd`; si no, se detiene.
- Anotar el SHA-256 calculado. Sin él, la evidencia no es reproducible.
- La carpeta temporal debe tener ACL restringida al usuario que inspecciona; no
  se comparte por red ni se sincroniza.
- Al terminar: borrar el temporal o conservarlo cifrado con aprobación expresa.
  **No** se borra el objeto S3.

## 5. Procedimiento de inspección segura (sin extraer)

Objetivo: **clasificar** el contenido, no usarlo.

1. **No extraer.** Se abre el ZIP en modo lectura y se enumeran entradas.
2. **No ejecutar** ningún archivo del ZIP (ni scripts, ni binarios, ni
   `.tfstate`).
3. **No abrir** el contenido de entradas sensibles para “comprobar”: basta el
   nombre, el tamaño y la extensión.
4. **No adjuntar** el ZIP, ni capturas de su contenido, ni Fragmentos, a
   informes, PR, chat o correo.
5. **No rotar** credenciales por la mera presencia de un archivo; la rotación
   exige un Gate propio y un incidente declarado.
6. **No borrar** el objeto ni versiones anteriores: son evidencia y rollback.

Enumeración sin extraer (sólo nombres, tamaños y fechas):

```powershell
Add-Type -AssemblyName System.IO.Compression.FileSystem
$zip = [System.IO.Compression.ZipFile]::OpenRead("$env:TEMP\nexum-zip-audit\terraform-h08i.zip")
$zip.Entries |
  Select-Object FullName, Length, LastWriteTime |
  Sort-Object FullName
$zip.Dispose()
```

Clasificación agregada (salida permitida: **conteos y tipos**, nunca valores):

| Clase | Patrones de nombre/extensión | Riesgo si aparece |
|---|---|---|
| Estado Terraform | `*.tfstate`, `*.tfstate.backup`, `terraform.tfstate*` | **Crítico**: contiene valores `sensitive` en claro (p. ej. contraseña Redis, claves) |
| Variables | `*.tfvars`, `*.auto.tfvars`, `terraform.tfvars` | **Crítico**: credenciales y parámetros privados |
| Planes | `*.tfplan`, `plan*`, `*.plan` | **Crítico**: el plan binario incluye valores de variables |
| Credenciales | `credentials`, `*.aws`, `config` con perfiles, `*.pem`, `*.key`, `*.p12`, `*.jks` | **Crítico**: acceso directo |
| Claves FCM/Google | `*service-account*.json`, `*firebase*.json`, `*fcm*.json`, `private_key` | **Crítico**: envío de push y acceso al proyecto |
| Secretos de aplicación | `secrets.json`, `*.env`, `backend.env` | **Crítico**: JWT, Redis, SES, claves |
| Respaldos/copias | `*.bak`, `*.zip`, `*.7z`, `*.tar*`, `*.dpapi` | **Alto**: copia adicional sin control |
| Documentación/notas | `*.md`, `*.txt`, `*.log` | **Medio**: pueden citar valores |
| Desconocido | cualquier otro | **Medio**: revisar nombre, no contenido |

Si el ZIP contiene **cualquier** clase crítica, el estado pasa a
`CONTiene_SECRETOS_REALES` (a confirmar por el custodio) y se activa la sección
7. No se declara “no contiene secretos” sin haber enumerado todas las entradas.

## 6. Qué **no** se puede afirmar hoy

| Afirmación | Estado |
|---|---|
| El ZIP contiene o no contiene `tfstate` | **`UNKNOWN`** |
| Contiene credenciales o claves FCM | **`UNKNOWN`** |
| El contenido está cifrado de extremo a extremo | Sólo se sabe que el objeto usa SSE-S3 en reposo; el ZIP en sí **no** se ha abierto |
| El ETag corresponde al contenido actual | Metadato de S3; no verificado contra bytes descargados |
| Alguien accedió al objeto | **`UNKNOWN`**: no se ha consultado CloudTrail de datos |
| Hay copias en versiones anteriores | **`UNKNOWN`**: el bucket tiene versionado; no se listaron versiones |

## 7. Riesgos si contiene secretos reales

| Riesgo | Impacto | Mitigación (con Gate) |
|---|---|---|
| `tfstate` con contraseña Redis y claves | Acceso a caché y datos de sesión | Rotar contraseña Redis + refresh; no basta borrar el objeto |
| `tfvars` con credenciales | Reproducción de infraestructura o acceso | Rotar, revisar IAM, restringir lectura |
| `*.tfplan` | Incluye valores de variables sensibles | Tratarlo como secreto; no publicarlo; borrar versiones con Gate |
| JSON de cuenta de servicio FCM | Envío de push suplantando el backend | Revocar clave en Google Cloud, generar nueva, publicar bundle v3 |
| Claves JWT (`JWT_PRIVATE_KEY_PEM_B64`) | Forjar tokens válidos | Rotar `JWT_KEY_ID` y claves; invalidar sesiones |
| ZIP con copias de otros secretos | Amplifica la exposición | Inventario, rotación por lotes, revisión de accesos |
| Versionado del bucket | El objeto “borrado” sigue recuperable | Borrado de versiones específicas con Gate; evaluar retención |
| Sin CloudTrail de datos | No se sabe quién leyó | Habilitar data events (coste) o aceptar el riesgo declarado |

**No se propone rotar nada ahora**: no hay evidencia de que el ZIP contenga
secretos reales, y rotar sin motivo rompe dispositivos, sesiones y el bundle.
La secuencia correcta es: autorizar lectura → clasificar → decidir con Gate.

## 8. Autorización puntual solicitada (sin ampliar permisos de forma permanente)

```text
shell: aws s3api head-object *   -> ask
shell: aws s3api get-object *    -> ask
```

Uso previsto: **una vez**, para los pasos 1–3 de §4 y la enumeración de §5.
Al concluir:

1. Retirar las dos reglas.
2. Verificar `git diff` de `opencode.json` vacío contra `HEAD`.
3. Registrar el SHA-256 calculado y el resultado de la clasificación **sin
   valores**.
4. Borrar el temporal o custodirlo cifrado.

Sin esta autorización, el estado permanece **`BLOCKED_AUTHORIZATION`** y no se
explora ninguna vía alternativa.

## 9. Estado final de esta unidad

| Elemento | Estado |
|---|---|
| `head-object` intentado una vez | **Sí** (2026-11-24) |
| Resultado | **`permission.rejected` (política de herramientas)** |
| Contenido descargado | **No** |
| ZIP listado | **No** |
| SHA-256 del contenido | **`NOT_COMPUTED`** |
| Clasificación | **`UNKNOWN`** |
| IAM modificado | **No** |
| Objeto borrado | **No** |
| Credenciales rotadas | **No** |
| Estado | **`BLOCKED_AUTHORIZATION`** |
