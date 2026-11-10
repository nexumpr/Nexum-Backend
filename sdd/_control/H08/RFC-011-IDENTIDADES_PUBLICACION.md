# RFC-011 — Identidades de publicación (artefactos, secretos y gestor IaC)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/RFC-011-IDENTIDADES_PUBLICACION.md` |
| Fecha | 2026-11-10 |
| Estado | **IMPLEMENTADO EN LOCAL — NO APLICADO EN AWS** |
| Autorización | `APRUEBO_RFC011_IDENTIDADES_PUBLICACION` y `APRUEBO_GATE_FASE_BASE_PLAN` |
| Alcance | Crear tres identidades IAM declarativas y desbloquear el plan de fase base |
| Mutación AWS | **Ninguna** (`plan` únicamente) |

## 1. Problema

`envs/h08i` exigía tres ARN que no existían (`artefactos_publicador_arn`,
`secretos_publicador_arn`, `secretos_gestor_iac_arn`). Vacíos hacían fallar el plan cerrado y no
hay OIDC disponible (`RFC-010`: `iam:CreateOpenIDConnectProvider` denegado por SCP).

## 2. Solución implementada

Módulo nuevo `infra/terraform/modules/identidades_publicacion` con tres roles:

| Rol | Nombre | Permisos |
|---|---|---|
| `artefactos_publicador` | `nexum-h08i-artefactos-publicador` | `s3:PutObject`/`AbortMultipartUpload` sólo en `releases/*`; `ListBucket`/`GetBucketLocation` con condición `s3:prefix`; `Deny` de borrado y de administración del bucket |
| `secretos_publicador` | `nexum-h08i-secretos-publicador` | `s3:PutObject` sólo en `nexum/secrets/*`; `ListBucket` acotado; `Deny` de lectura, borrado y administración |
| `secretos_gestor_iac` | `nexum-h08i-secretos-gestor-iac` | `ListBucket`/`GetBucketLocation`/`GetBucketVersioning` del bucket y `GetObject`/`PutObject`/`GetObjectVersion` del prefijo; `Deny` de borrado y administración |

Trust: `sts:AssumeRole` limitado a **un** principal operador concreto
(`aws:PrincipalArn` = `var.principal_operador_arn`). Sin `:root`, sin comodines, sin usuarios IAM,
sin claves de acceso y sin proveedor OIDC.

Variables eliminadas: las tres `*_arn` de `envs/h08i` (quedaban permanentemente vacías). En su
lugar la raíz usa `local.arn_*` derivados de la cuenta y del nombre determinista del rol, y un
bloque `check` compara esos valores con `module.identidades.*_arn` reales cuando existen.

## 3. Decisión de diseño: ARN derivados y no referencias directas

Las preconditions de I-060 validan el **formato** del ARN con `can(regex(...))`. Si se les pasa
`module.identidades.x_arn` (valor desconocido hasta el apply), Terraform no puede evaluarlas y
falla el plan. Por eso:

- el módulo de identidades recibe **nombres** de bucket (no ARN) → sin ciclo;
- la raíz deriva el ARN canónico `arn:aws:iam::<cuenta>:role/<nombre>` (determinista, no inventado);
- `check "identidades_publicacion_coherentes"` verifica al aplicar que el ARN real coincide.

## 4. Defecto D-BASE-02 corregido

La raíz pasaba a `iam.artifact_object_arns` el valor
`arn:aws:s3:::…/releases/*`, que **viola** la validación del módulo (prohíbe comodines y exige
objeto exacto). Se sustituye por `var.iam_artifact_object_arns` (vacío en fase base). Efecto
declarado: el rol runtime **no** puede leer artefactos hasta que exista el release y se aporten los
dos ARN exactos (binario y unidad systemd). Es más restrictivo que antes, no menos.

## 5. Defecto D-BOOT-01 — causa raíz identificada

`terraform plan -input=false -out=bootstrap.tfplan` fallaba con
`Too many command line arguments`. Causa: el envoltorio de shell **rompe los argumentos con `=`**
(`-input=false`, `-out=…`, `-var=…`). `terraform plan -input=false` (sin `=`) sí funciona.

Solución aplicada **sin envoltorios ni procesos indirectos**: usar la forma separada de Terraform,
que el binario acepta igual:

```
terraform plan -input=false -var-file "RUTA.tfvars" -out "RUTA.tfplan"
```

Con esa forma el plan se generó correctamente. Sigue siendo necesario corregir el envoltorio, pero
ya no bloquea.

## 6. Permisos (RFC-002) — cambio mínimo y revisable

`opencode.json` cambia exactamente cuatro reglas, **sin tocar** `apply*`, `destroy*`, `import*`,
`state*` ni `init -migrate-state*`:

| Capa | Antes | Ahora |
|---|---|---|
| `permissions` `terraform plan*` | deny | **ask** |
| `policies` `shell:terraform plan*` | deny | **allow** |
| `permissions` `terraform init -backend=true*` | deny | **ask** |
| `policies` `shell:terraform init -backend=true*` | deny | **allow** |
| nuevas | — | `terraform show*` ask + allow |
| nuevas | — | `aws s3api list-objects-v2*` ask |

Cada ejecución exige aprobación humana (ASK). `terraform apply*` sigue en **DENY**.

## 7. Plan de fase base generado (no aplicado)

> **ESTADO 2026-11-10 (posterior): el plan descrito en esta sección está OBSOLETO.**
> Se generó antes del endurecimiento de la *trust policy* (§11) y antes de retirar
> el `default = ""` de `principal_operador_arn`. **No debe aplicarse.** Su SHA-256
> se conserva solo como trazabilidad. Tras el merge de este PR hay que regenerarlo
> desde `main` y solicitar un Gate nuevo.

| Campo | Valor |
|---|---|
| Comando | `terraform plan -input=false -var-file "…" -out "…"` en `envs/h08i` con backend S3 |
| Artefacto | `fase-base.tfplan` (custodia externa al repositorio, ACL restringida) |
| SHA-256 | **`5628072141D368F84B767A391A5A416A2562B37B54757EB7ED0D10AFD03CAF1E`** (**STALE — NO APLICAR**) |
| Tamaño | 87 447 bytes |
| Cambios | **74** entradas · **72 `create`** · 2 `read` (data sources) |
| Destrucciones / reemplazos | **0 / 0** |
| Importaciones | **0** |
| Proveedor | 65 AWS + 9 locales |

Inventario AWS (72 create): VPC, 2 subredes, IGW, 1 route table + 2 asociaciones, NACL + 8 reglas,
4 SGs + 5 reglas egress + 4 ingress, 3 VPC endpoints (2 gateway S3/DynamoDB + **1 interface
`logs`**), DynamoDB (tabla única), ElastiCache (subnet group, parameter group, replication group
`cache.t4g.micro` 1 nodo redis 7.1 con TLS y cifrado, 2 usuarios + user group), 2 buckets S3
(artefactos y secretos con versionado, SSE-S3, PAB, ownership, lifecycle y política TLS),
4 roles IAM + 5 políticas + 1 instance profile.

**Ausencias verificadas:** `aws_instance` 0 · `aws_nat_gateway` 0 · `aws_eip` 0 · `aws_lb` 0 ·
`aws_cloudfront_distribution` 0 · `aws_autoscaling_group` 0 · `aws_launch_template` 0.

## 8. Coste

| Partida | USD/hora | 6 h (prueba) | 45 días |
|---|---|---|---|
| Redis OSS `cache.t4g.micro` | 0.0160 | 0.0960 | 17.2800 |
| Interface endpoint `logs` (1–2 AZ × 0.0100) | 0.0100–0.0200 | 0.0600–0.1200 | 10.8000–21.6000 |
| **Total incremental** | **0.0260–0.0360** | **0.1560–0.2160** | **28.0800–38.8800** |

Gateway endpoints (S3/DynamoDB), DynamoDB `PAY_PER_REQUEST`, S3 sin objetos y roles IAM no añaden
cargo por hora. El total sigue dentro del presupuesto vigente de I-000 (102.7830 USD brutos;
113.0613 con margen sobre 120.0000 verificados). Coste real observado: `NOT_VERIFIED`.

## 9. Secreto de Redis (RFC-004)

`module.redis` exige `password_aplicacion` conocido en el plan (precondition de longitud). Se
generó un valor aleatorio de 32 caracteres con RNG criptográfico **en memoria**, se escribió en un
`tfvars` **fuera del repositorio** (`C:\Nexum-Custodia\…`, ACL sin herencia y sólo el usuario
actual), y **no** se imprime en la salida ni se registra en `memoria.md`. El plan binario también
queda en esa carpeta protegida porque contiene el valor sensible.

Pendiente declarado: el bundle de secretos hacia S3 (RFC-004 fase B) sigue sin publicarse; hasta
entonces el valor solo vive en el state cifrado y en el tfvars protegido.

## 10. BLOQUEOS que permanecen

`RFC-004` fase B (publicación del bundle), `H-01` (TLSv1 con certificado por defecto), `RFC-006`
(WAF temporal), `RFC-007` (SNS/CMK), `D-100-01/02`, `D-110-01`, `AWS_SCP_NOT_VERIFIED`,
`READINESS 503`, `ARTIFACT_DELIVERY` (objetos exactos para el runtime) y `D-PERM-01`
(reglas de cadena exacta inertes en el motor de permisos).

Este documento no autoriza ningún `apply`.

## 11. Trust policy: defecto reportado, verificación y endurecimiento (2026-11-10)

### 11.1 Defecto reportado

Se informó de un enunciado `DenegarPrincipalNoAutorizado` con `resources = ["*"]` dentro de
`data.aws_iam_policy_document.trust_operador`. Una *trust policy* de rol **no admite**
`Resource`/`NotResource`: AWS responde `MalformedPolicyDocument` y el rol no se crea, de modo que
el fallo habría aparecido en el primer `apply` (no en `terraform validate`).

### 11.2 Estado real verificado

El enunciado **no existe** en el árbol de trabajo de la rama: `trust_operador` contiene un único
enunciado y ninguna declaración `resources`/`not_resources`. Evidencia de la política ya
**renderizada** (no del HCL) en el plan custodiado:

```
module.identidades.data.aws_iam_policy_document.trust_operador
{ "Version": "2012-10-17",
  "Statement": [ { "Sid": "SoloPrincipalOperadorAutorizado",
                   "Effect": "Allow",
                   "Action": "sts:AssumeRole",
                   "Principal": { "AWS": "arn:aws:iam::713797602090:role/AccountFullAccessRole" },
                   "Condition": { "StringEquals": { "aws:PrincipalArn":
                                   "arn:aws:iam::713797602090:role/AccountFullAccessRole" } } } ] }
```

Sin `Resource`, sin `Principal: "*"` y sin `:root`. No se presenta esta comprobación como
aprobación del `apply`: solo acredita el contenido del plan, no la respuesta de AWS.

### 11.3 Correcciones aplicadas

| Cambio | Motivo |
|---|---|
| `StringEquals` → `ArnEquals` sobre `aws:PrincipalArn` | AWS recomienda operadores de ARN para claves de tipo ARN. Documentado: para sesiones de rol, `aws:PrincipalArn` devuelve el ARN **del rol**, no el ARN STS de la sesión; por eso la confianza se ancla al rol IAM real (`AccountFullAccessRole`). |
| `default = ""` retirado de `principal_operador_arn` (entorno y módulo) | Un valor vacío por defecto es un fail-open silencioso; ahora la variable es obligatoria y además está validada. |
| Validación explícita contra `arn:aws:sts::` | El ARN de sesión cambia en cada login; anclar la confianza ahí rompería el siguiente despliegue. |
| Precondition «confianza circular» | El operador no puede ser una de las tres identidades creadas por el módulo. |
| 4 preconditions sobre el **JSON renderizado** | `Resource`, `NotResource`, `NotPrincipal`, un `Deny`, un `Principal` distinto del operador o una condición distinta hacen fallar el **plan** (no solo el test). |
| `tests/infra/terraform/test_identidades_publicacion_contrato.py` (45 pruebas) | Contrato del módulo, cableado del entorno y **fixtures negativas** que demuestran que el detector rechaza las variantes defectuosas. |

Las preconditions decodifican `data.aws_iam_policy_document.trust_operador.json` con
`jsondecode` y examinan `keys()` de cada enunciado; no buscan texto, de modo que un atributo
añadido en el bloque equivocado no pasa inadvertido. `terraform validate` no las evalúa: su
verificación efectiva ocurre en el plan/apply real, que sigue `NOT_RUN`.

### 11.4 Consecuencia sobre el plan

El plan de §7 queda **STALE**. Debe regenerarse desde `main` tras el merge, con el mismo
`tfvars` custodiado (contraseña Redis existente, sin generar otra) y `desplegar_aplicacion=false`,
y requiere un Gate nuevo `APRUEBO_APPLY_FASE_BASE_SHA256_<SHA256>`.
