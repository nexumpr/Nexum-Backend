# Gate H08-E / E005 — Despliegue AWS y verificación funcional

**ESTADO:** `DEPLOYED_AWS — E005_HTTPS_202 — SES_ACCEPTED — EMAIL_PENDING_USER_CONFIRMATION` (2026-11-17).

Registro local del despliegue. No se publicó en un PR nuevo (el PR #68 ya fue fusionado y no se
autoriza otro). El documento queda en el árbol de trabajo para el siguiente PR de la spec que lo
incorpore.

## 1. Release

| Campo | Valor |
|---|---|
| `main` | `041b406afdcd65599f0006d7de3ed702a38beaa7` |
| CI de `main` | run `44010923551` **SUCCESS** |
| Release ID | `v0.12.0+041b406afdcd` |
| Artefacto | `nexum-backend-linux-arm64`, 20 227 795 B, SHA-256 `febc9592a62cb5fcf73bd3331b63bd5a5706e67dc646038cba3f3a6fb80912b8` |
| SBOM | `sbom.spdx.json`, 88 402 B |
| S3 binario | `releases/v0.12.0+041b406afdcd/nexum-backend-linux-arm64`, VersionId `j7Ml4ikEgra0KBsUTG9bBNng6Qms9kIF`, ETag `4d452d129959eec22b6040a918ff61d8` |
| S3 unidad | `releases/v0.12.0+041b406afdcd/nexum-backend.service`, VersionId `yvMhkJ.0T7i7DO8jxZZlZAov6GVRzpG2`, ETag `40f331d10b7f4c338948d79d14f02d44` |
| Integridad | `ChecksumSHA256` de PutObject coincide con SHA-256 local; ETag coincide con MD5 local |

## 2. Terraform LT/IAM

| Campo | Valor |
|---|---|
| Plan | `C:\Nexum-Custodia\H08E-E005-20261117\e005-lt-iam.tfplan` |
| SHA-256 | `BEE63A5C5E97EBDED75EDF225D2C62F8AC38CA69C9683845C08AD960B3C35A07` |
| Acciones | **0 add · 5 update · 0 destroy/replace/import** |
| Apply | **`0 added, 5 changed, 0 destroyed`** |
| LT | `nexum-h08i-lt-20261111034054695000000001` **v6** |
| `user_data` | `BINARIO_URI=.../v0.12.0+041b406afdcd/nexum-backend-linux-arm64`; `BINARIO_SHA=febc...12b8`; unidad `6fa4d5fb...05cc` |
| IAM | `ReadOnlyExactBootstrapArtifacts` con **12 ARNs** (10 previos + 2 de v0.12.0) |
| Redis | normalización `no-password` → `no-password-required`, sin cambio efectivo |

## 3. Rollout y capacidad

- Refresh `1f06b63f-e11c-4344-83be-ae2ee2e59e04` con `MinHealthyPercentage=100`,
  `MaxHealthyPercentage=150`, `InstanceWarmup=120`, `ScaleInProtectedInstances=Refresh`.
- El primer nodo v6 (`i-0c6ec6ea929f9c53e`, us-east-2a) quedó sano. El reemplazo en **us-east-2b**
  falló **10 veces** por falta de capacidad `t4g.micro` (actividades 03:53–04:18 UTC). El refresh
  se canceló; no se ocultó el incidente.
- Recuperación controlada: `set-instance-protection` off sobre `i-09e9b8e81c8a6d054`, VPC zone
  temporal solo `us-east-2a`, reemplazo por `i-0cc15a941dbb7a685`, terminación del v5 y
  restauración de `subnet-073a7408c84371c6b,subnet-07773fedf3b74e518` con **Min 2 / Desired 2 /
  Max 2**.
- **Estado final:** `i-0c6ec6ea929f9c53e` e `i-0cc15a941dbb7a685`, ambas LT **v6**,
  `InService`/`Healthy`, en **us-east-2a**. Target group con **2 targets healthy** en 8080.
  Limitación declarada: ambas en la misma AZ por capacidad; el ASG conserva las dos subredes y
  Max 2, por lo que la redundancia multi-AZ no está garantizada mientras la capacidad de 2b no
  vuelva.
- `CD` workflow: **BLOQUEADO** por SCP (`iam:ListOpenIDConnectProviders`/`CreateOpenIDConnectProvider`
  denegados por `p-ys0pa0u8`) y ausencia de entorno `aws-deploy`/`CD_HABILITADO`. Se usó el
  procedimiento asistido de E003/E004. No se declara `CD_WORKFLOW_PASS`.

## 4. Verificación funcional

- `POST /api/v1/auth/password/forgot` con `{"email":"nexumproyectomov@gmail.com"}` →
  **HTTP 202** `{"status":"accepted","requestId":"eda91f76-bc61-4ffd-a3c3-a1131495c18e"}`.
- `QUEUE#PWRESET`: `estado=ENVIADO`, `intentos=0`, `enviadoEn=1794889555`,
  `expiresAt=1794891353` (TTL 1798 s ≈ 30 min); `tokenSello` eliminado tras el envío.
- `RESET#<sha256>`: solo hash y TTL (código); no se consultó el item concreto porque el token
  solo viaja en el correo.
- SES: `GetEmailIdentity` `SUCCESS`/`VerifiedForSendingStatus=true`; cuenta `SendingEnabled=true`,
  `ProductionAccessEnabled=false` (sandbox). El worker marcó `ENVIADO` tras la aceptación de SES.
- **Correo recibido: PENDIENTE DE CONFIRMACIÓN DEL RESPONSABLE**. No se dispone de acceso al
  buzón y no se declara entrega verificada.
- Smoke E001–E004 por CloudFront: E001 replay **201**; E002 **200** (`expiresIn=900`,
  `session.id=sessionId`); E003 **200** con rotación; E004 **204** sin cuerpo; refresh posterior
  **401 `UNAUTHORIZED`**. PASS.

## 5. Permisos y evidencia

- `opencode.json` conserva el diff local de 70 líneas; se retiraron las excepciones temporales
  `artefactos-pub` y `describe-vpc-endpoint-services`.
- No se imprimieron contraseñas, tokens, claves AES ni credenciales STS.
- `memoria.md` actualizada en el árbol de trabajo; no publicada porque no se autoriza un PR nuevo.
