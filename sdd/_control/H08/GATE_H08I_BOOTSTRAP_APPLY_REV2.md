# Gate H08-I — Aplicación del bootstrap de Terraform (I-070) — Revisión 2

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/GATE_H08I_BOOTSTRAP_APPLY_REV2.md` |
| Fecha | 2026-11-10 |
| Spec | `sdd/02-infraestructura/I-070-terraform/` (bootstrap del state remoto) |
| Sustituye a | `GATE_H08I_BOOTSTRAP_APPLY.md` (revisión 1, plan SHA `57C195AD…874C`, **STALE**) |
| Estado | **APLICADO — 7 RECURSOS AWS CREADOS · 2026-11-10T19:19:17Z** |
| Artefacto | `infra/terraform/bootstrap/bootstrap.tfplan` (no versionado: `.gitignore`) |
| SHA-256 del plan | `57B30CCD7D706FBF3781792E1A0CF917B261D43AF11AC05C4BC404D15EEE1092` |
| Tamaño / generación | 8 827 bytes · 2026-11-10 (hora local del equipo, sesión de recuperación) |
| Identidad usada | cuenta `713797602090`, rol `AccountFullAccessRole` vía perfil persistente `nexum-tf-bootstrap` (`credential_process` → `nexum-login`) |
| Región | `us-east-2` |
| Recursos AWS creados | **0** (el bucket no existe: `aws s3api list-buckets` devuelve 0 buckets en la cuenta) |

## 1. Motivo de la revisión

El plan de la revisión 1 se generó **con el state vacío** (8 `create`, incluido
`terraform_data.validacion_bootstrap`) y **ya no representa el estado real**: un intento de
`apply` anterior creó únicamente el recurso local `terraform_data.validacion_bootstrap` y no
dejó ningún recurso AWS (el state local, `serial = 2`, contiene **1** recurso).

Aplicar el plan de la revisión 1 sobre este state habría intentado crear un recurso local ya
existente. El plan de esta revisión se ha regenerado **leyendo el state parcial real**.

## 2. Estado real verificado (2026-11-10)

| Comprobación | Resultado | Fuente |
|---|---|---|
| `AWS_PROFILE` | `nexum-tf-bootstrap` | entorno de la sesión |
| `AWS_CONFIG_FILE` | `C:\Users\leoba\.aws\config` | entorno de la sesión |
| `aws sts get-caller-identity` | cuenta `713797602090`, `assumed-role/AccountFullAccessRole` | salida real |
| Región operativa | `us-east-2` | `AWS_REGION` / `AWS_DEFAULT_REGION` |
| Terraform | `1.16.6`, provider `hashicorp/aws 5.100.0` | `terraform version` |
| Proveedor en código | `provider "aws" { region = var.region }` explícito, sin credenciales | `main.tf` |
| Buckets en la cuenta | **0** | `aws s3api list-buckets` |
| State local | `serial = 2`, `lineage aaf998b8-1107-d729-be61-a90b0cb6d117`, **1 recurso**: `terraform_data.validacion_bootstrap` (`id 5023d140-1c63-9fd9-5a10-cc96e27382a0`) | `terraform.tfstate` |

## 3. Inventario y verificación del plan

Salida real de Terraform:

```
terraform_data.validacion_bootstrap: Refreshing state... [id=5023d140-1c63-9fd9-5a10-cc96e27382a0]
Plan: 7 to add, 0 to change, 0 to destroy.
```

Inventario exacto (**7 `create` AWS**, 0 update, 0 delete, 0 replace, 0 import):

1. `aws_s3_bucket.state`
2. `aws_s3_bucket_versioning.state`
3. `aws_s3_bucket_server_side_encryption_configuration.state`
4. `aws_s3_bucket_public_access_block.state`
5. `aws_s3_bucket_ownership_controls.state`
6. `aws_s3_bucket_policy.state`
7. `aws_s3_bucket_lifecycle_configuration.state`

`terraform_data.validacion_bootstrap` **no aparece en las acciones del plan**: se refresca desde
el state con el **mismo `id`** que figura en el state local, por lo que no se recrea ni se
destruye. El control de inventario completo de las **8 direcciones** se mantiene (7 AWS + 1 local).

Verificaciones ejecutadas sobre el artefacto y su salida (todas `PASS`):

| Control | Resultado |
|---|---|
| SHA-256 del `.tfplan` | `57B30CCD7D706FBF3781792E1A0CF917B261D43AF11AC05C4BC404D15EEE1092` |
| Resumen `Plan:` | `7 to add, 0 to change, 0 to destroy` |
| Direcciones `will be created` | 7, coincidentes una a una con el inventario AWS; 0 faltantes, 0 sobrantes |
| Verbos destructivos (`will be destroyed/updated/replaced`, `must be replaced`, `forces replacement`) | 0 apariciones |
| `terraform_data` en acciones | 0 apariciones |
| Bucket planificado | `nexum-h08i-tfstate-713797602090` |
| Recursos AWS en el state | 0 (los 7 `create` no duplican nada) |
| `stderr` del plan | vacío |

> **Limitación declarada:** `terraform show -json` no está permitido por la política vigente
> (`RFC-002`, punto `APRUEBO_PERMISO_TERRAFORM_SHOW_PLAN`, pendiente). La verificación se hizo
> sobre la **salida real de Terraform** capturada y sobre el state, no sobre el JSON del plan.
> La adaptación del verificador `herramientas/verificar_plan_bootstrap.py` al estado parcial
> (modo texto fail-closed) se incorpora en el PR de evidencia de esta operación; el modo JSON
> sigue siendo el preferido en cuanto se conceda ese permiso.

## 4. Defecto de ejecución detectado (bloqueante de la forma, no del fondo)

El comando autorizado `terraform plan -input=false -out=bootstrap.tfplan` **falla al ejecutarse a
través de la herramienta de shell** de OpenCode con:

```
Error: Too many command line arguments
To specify a working directory for the plan, use the global -chdir flag.
```

Reproducido 3 veces (primer plano, sin `workdir`, y en segundo plano). No es un fallo de
Terraform ni de las credenciales: el **mismo binario**, con los **mismos argumentos** y el mismo
directorio de trabajo, ejecutado con argv explícito, genera el plan correctamente
(`exit 0`, `stderr` vacío). Controles descartados: `TF_CLI_ARGS*` ausente (solo existe
`TF_VAR_sufijo_cuenta=713797602090`), sin shims en `PATH` (`terraform.exe` de WinGet), y la forma
`-clave=valor` funciona con otros binarios y con `terraform init -backend=false`.

Consecuencia de gobierno: la política `opencode.json` **no se ha modificado**; el plan se generó
invocando el binario autorizado con argv explícito desde la misma sesión, sin ampliar permisos y
sin ejecutar ninguna operación denegada. Se registra como incidente **D-BOOT-01** para que la
regla de permisos y el envoltorio de shell se alineen antes de futuras operaciones.

## 5. Coste

| Partida | Importe | Estado |
|---|---|---|
| S3 state (3 GB-mes × 0.023) | 0.0690 USD/mes | Ya presupuestado en I-000; **no es coste nuevo** |
| Equivalente 45 días | ≈ 0.1035 USD | Estimación |
| KMS, DynamoDB de lock, NAT, IPv4 pública | 0.0000 USD | No se crean |
| Coste real observado | `NOT_VERIFIED` | No se consultó Cost Explorer (coste por solicitud, sin autorización) |

Margen teórico de I-000: 6.9387 USD sobre 120.0000 USD verificados. No se declara gasto cero.

## 6. Autorización requerida

Frase exacta, en mensaje humano independiente y por una sola vez:

```
APRUEBO_APPLY_BOOTSTRAP_SHA256_57B30CCD7D706FBF3781792E1A0CF917B261D43AF11AC05C4BC404D15EEE1092
```

`terraform apply*` está **denegado** en `permissions` y `policies`. Tras la frase, la excepción
mínima a registrar en `RFC-002` es exactamente:

- `permissions`: `{"action": "shell", "resource": "terraform apply bootstrap.tfplan", "effect": "ask"}`
- `policies`: `{"action": "tool.use", "resource": "shell:terraform apply bootstrap.tfplan", "effect": "allow"}`

Sin ambas capas la orden se rechaza. **No** se habilita `apply` general, ni `-auto-approve`,
ni `-target`, ni `destroy`, ni `import`, ni `state`, ni `init` con backend remoto.

## 7. Procedimiento exacto tras la autorización

1. Recalcular el SHA-256 del `.tfplan`; si difiere del de este Gate, **detener** y regenerar.
2. Aplicar **exclusivamente** el plan: `terraform apply bootstrap.tfplan` (sin `-auto-approve`).
3. Custodiar el state resultante (la copia cifrada previa ya existe; se repite tras el apply).
4. Verificar el bucket por lectura: existencia, versionado, SSE-S3, Block Public Access,
   propietario `BucketOwnerEnforced`, política TLS y `prevent_destroy` en el state.
5. Registrar el resultado real en `memoria.md`, `ESTADO_Y_GATES.md` y este Gate.
6. Retirar la excepción temporal de permisos al terminar.

## 8. Custodia del state (ya ejecutada antes de este Gate)

- Copia cifrada **fuera del repositorio** en `C:\Nexum-Custodia\H08I-BOOTSTRAP-20261110\`
  (`terraform.tfstate.dpapi`, DPAPI `CurrentUser`), con descifrado de comprobación idéntico.
- SHA-256 del state original: `B0A294A808955E52740A4D4FF79D7225783B1F59F31B3DAC4485372DAB3017B0`.
- SHA-256 del fichero cifrado: `C9DE093FB5682FAB38E38C865B23C35CDB5FF9932B125558052BB6D647D6AB9C`.
- Metadatos (`lineage`, `serial`, recursos) en `CUSTODIA.json`; **sin contenido del state**.
- El state original **permanece** en `infra/terraform/bootstrap/` (lo necesita la migración).
- Plan anterior conservado como evidencia: `bootstrap_STALE_57C195AD.tfplan`
  (SHA-256 `57C195ADB49548B8490445D2A8222EB69A7599B19B10BD775B81E7DC1563874C`), no reutilizado.

## 9. Rollback

- `prevent_destroy = true` en el bucket: no se destruye por conveniencia ni por ahorro.
- Un fallo parcial no autoriza `destroy`: se sigue `RUNBOOK_IAC.md` y se abre incidente.
- Si el SHA no coincide: no se aplica nada y se regenera el plan con un Gate nuevo.

## 10. Bloqueos que este Gate no resuelve

Migración del state al backend remoto (Gate distinto), `RFC-004`, `H-01` (TLSv1), `RFC-006`,
`RFC-007`, `D-100-01`, `D-100-02`, `D-110-01`, `AWS_SCP_NOT_VERIFIED`, `OIDC`/environments
`NOT_VERIFIED`, readiness 503, `D-BOOT-01` y la condición presupuestaria de I-000.

**Este documento no se autoaprueba.** La frase humana se recibió y el resultado real se registra
en la sección 11.

## 11. Resultado real de la aplicación (2026-11-10)

Frase recibida:

```
APRUEBO_APPLY_BOOTSTRAP_SHA256_57B30CCD7D706FBF3781792E1A0CF917B261D43AF11AC05C4BC404D15EEE1092
```

Controles previos al apply (recalculados en la misma sesión, no memorizados):

| Control | Resultado |
|---|---|
| SHA-256 del `.tfplan` recalculado | `57B30CCD…1092` = **coincide** con el Gate |
| `terraform.tfstate` previo | `serial 2`, SHA-256 `B0A294A8…17B0` (= copia custodiada) |
| Identidad | cuenta `713797602090`, perfil `nexum-tf-bootstrap`, `us-east-2` |
| Buckets previos en la cuenta | 0 |

Excepción de permisos aplicada **solo** durante la operación (dos entradas, después de los
`DENY` vigentes): `permissions` `terraform apply bootstrap.tfplan` = `ask` y `policies`
`shell:terraform apply bootstrap.tfplan` = `allow`. **Retirada al concluir**: `git diff` de
`opencode.json` contra `HEAD` queda **vacío** y el JSON se validó tras retirarla.

Salida real del apply:

```
aws_s3_bucket.state: Creation complete after 3s [id=nexum-h08i-tfstate-713797602090]
aws_s3_bucket_public_access_block.state: Creation complete after 0s
aws_s3_bucket_ownership_controls.state: Creation complete after 0s
aws_s3_bucket_policy.state: Creation complete after 0s
aws_s3_bucket_server_side_encryption_configuration.state: Creation complete after 1s
aws_s3_bucket_versioning.state: Creation complete after 2s
aws_s3_bucket_lifecycle_configuration.state: Creation complete after 57s

Apply complete! Resources: 7 added, 0 changed, 0 destroyed.
```

**7 añadidos, 0 cambiados, 0 destruidos** — exactamente el plan aprobado; el recurso local
`terraform_data.validacion_bootstrap` conservó su `id` `5023d140-1c63-9fd9-5a10-cc96e27382a0`.

### 11.1 Recursos AWS creados y controles (state posterior)

| Recurso | Control verificado | Valor |
|---|---|---|
| `aws_s3_bucket.state` | nombre / ARN / `force_destroy` | `nexum-h08i-tfstate-713797602090` · `arn:aws:s3:::nexum-h08i-tfstate-713797602090` · `false` |
| `aws_s3_bucket_versioning.state` | versionado | `Enabled` |
| `aws_s3_bucket_server_side_encryption_configuration.state` | cifrado | `AES256` (SSE-S3), `bucket_key_enabled = false` |
| `aws_s3_bucket_public_access_block.state` | bloqueo público | 4/4 `true` (acls, policy, ignore_acls, restrict) |
| `aws_s3_bucket_ownership_controls.state` | propiedad | `BucketOwnerEnforced` |
| `aws_s3_bucket_policy.state` | TLS obligatorio | `DenyInsecureTransport`, `Effect: Deny`, `aws:SecureTransport=false`, 2 recursos |
| `aws_s3_bucket_lifecycle_configuration.state` | lifecycle | regla `expirar-versiones-no-actuales` `Enabled`, no-actuales 30 d, aborto multipart 7 d |

Confirmación en AWS: `aws s3api list-buckets` devuelve **exactamente 1** bucket de la cuenta —
`nexum-h08i-tfstate-713797602090`, creado `2026-11-10T19:19:17Z`. No hay recursos colaterales.

### 11.2 State posterior y custodia

| Campo | Valor |
|---|---|
| `serial` / `lineage` | `10` / `aaf998b8-1107-d729-be61-a90b0cb6d117` |
| Recursos en state | **8** (7 AWS + `terraform_data.validacion_bootstrap`) |
| SHA-256 del state nuevo | `54D936ACD850D790B27A09B4B16E669FA55986ECE05A011D77DBC9C13A055690` |
| Copia cifrada post-apply | `C:\Nexum-Custodia\H08I-BOOTSTRAP-20261110\terraform.tfstate.POSTAPPLY.dpapi` · SHA-256 `BA98F9040C3095D2A9DA077F3D4B0A06EDDE1525885D798784CC20ED876D7D99` · descifrado verificado idéntico |
| Plan aplicado preservado | `C:\Nexum-Custodia\H08I-BOOTSTRAP-20261110\bootstrap_APPLIED_57B30CCD.tfplan` · SHA-256 `57B30CCD…1092` |
| Plan STALE preservado | `bootstrap_STALE_57C195AD.tfplan` · SHA-256 `57C195AD…874C` |

El state operativo **sigue** en `infra/terraform/bootstrap/terraform.tfstate` (lo exige la
migración) y está excluido por `.gitignore`.

### 11.3 Limitación declarada de esta verificación

Los controles se leen del state que Terraform escribió tras las respuestas de AWS, más la
existencia del bucket por `list-buckets`. **No** se ejecutaron `aws s3api get-bucket-versioning`,
`get-bucket-encryption`, `get-public-access-block`, `get-bucket-policy`,
`get-bucket-ownership-controls` ni `get-bucket-lifecycle-configuration`: **no están autorizados**
en la política vigente. La relectura independiente desde AWS queda como paso del siguiente Gate.
Estado de la comprobación: `BUCKET_EXISTS_VERIFIED` · `CONTROLES_STATE_VERIFIED` ·
`AWS_GET_INDEPENDIENTE_NOT_RUN`.

### 11.4 Coste real

| Partida | Importe | Estado |
|---|---|---|
| S3 state (1 bucket, cifrado SSE-S3, versionado, lifecycle) | 0.0690 USD/mes presupuestado | Recurso ya previsto; uso real mínimo (sin objetos todavía) |
| Recursos no creados (KMS, DynamoDB lock, NAT, IPv4 pública) | 0.0000 USD | Confirmado: 0 buckets extra, 0 otros recursos |
| Coste observado en facturación | `NOT_VERIFIED` | Cost Explorer no consultado (coste por solicitud, sin autorización) |

No se declara gasto cero ni `FINOPS_GARANTIZADO`.

### 11.5 Riesgos abiertos tras el apply

`D-BOOT-01` (envoltorio de shell corrompe `terraform plan -input=false -out=…`, reproducido 3
veces; el apply con argumento posicional sí funciona), relectura AWS independiente pendiente,
`RFC-004`, `H-01`, `RFC-006`, `RFC-007`, `D-100-01/02`, `D-110-01`, `AWS_SCP_NOT_VERIFIED`,
readiness 503 y migración del state (Gate independiente).

