# Gate H08-E / E004 — Despliegue en AWS y verificación HTTPS

**ESTADO:** `COMPLETED — AWS_VERIFIED — USER_ACCEPTED` (2026-11-16).

Registro del despliegue del release `v0.11.0+1e03d5cd12a7` en la infraestructura existente.
No se creó ningún recurso nuevo, no se modificó DynamoDB, Redis, ALB, CloudFront, VPC ni
`BASE_URL`, y no se ejecutó `terraform destroy`.

## 1. Gates humanos usados (todos previos a su operación)

| Operación | Frase |
|---|---|
| Publicación S3 | `APRUEBO_EXCEPCION_RFC002_PUBLICACION_S3_E004` · `APRUEBO_PUBLICACION_S3_E004_V0.11.0` |
| `terraform apply` | `APRUEBO_APPLY_LT_IAM_E004_SHA256_5FC9634FB5376B33F94D102BF43D11938A8DABDAFF55E1AEA7AD7BEB295FD735` |
| Instance refresh | `APRUEBO_ASG_E004_V0.11.0` |

## 2. Terraform apply (IAM runtime + Launch Template)

| Campo | Valor |
|---|---|
| Plan | `e004-lt-iam.tfplan` (custodia local, 164 483 B) |
| **SHA-256 del plan** | **`5FC9634FB5376B33F94D102BF43D11938A8DABDAFF55E1AEA7AD7BEB295FD735`** |
| Acciones | **0 add · 4 change · 0 destroy** |
| Recursos | `computo[0].aws_launch_template.backend`, `computo[0].terraform_data.invariantes_computo`, `iam.aws_iam_role_policy.runtime`, `iam.terraform_data.iam_preconditions` |
| Resultado | Aplicado el mismo archivo, sin regenerar tras el Gate |

**Defecto evitado en el preflight.** Un primer plan generado sólo con el tfvars de fase base
habría **degradado** el `user_data` a `releases/e001-d9f0696` y **reducido** la política IAM de
8 a 2 objetos, perdiendo `v0.8.0`, `v0.9.0` y `v0.10.0`. Causa: `-var-file` tiene precedencia
sobre `TF_VAR_*`. Se descartó ese plan (renombrado `…-STALE-NO-USAR`), se generó uno nuevo con
un segundo `-var-file` de override y se verificó explícitamente el `user_data` decodificado, el
digest, la unidad y los 10 ARN antes de aplicar.

**Estado posterior verificado** (`terraform show -json`, 114 recursos):

- Launch Template `nexum-h08i-lt-20261111034054695000000001` → **`latest_version = 5`**,
  creada `2026-11-16T03:04:03Z`.
- `user_data`: `BINARIO_URI=…/releases/v0.11.0+1e03d5cd12a7/nexum-backend-linux-arm64`,
  `BINARIO_SHA=fd67708a…aa12d`, `UNIDAD_URI=…/nexum-backend.service`, `UNIDAD_SHA=6fa4d5fb…05cc`.
- `aws_iam_role_policy.runtime`: **10 objetos exactos** (E001 primitivo, `v0.8.0`, `v0.9.0`,
  `v0.10.0` y `v0.11.0`, dos cada uno) → rollback viable a los cuatro releases previos.
- `default_version` sigue en 1 porque Terraform no lo fija; el ASG usa `$Latest`.
- Excepción temporal de `terraform apply` **retirada** al concluir: `permissions` 161,
  `policies` 43, sin reglas con `e004-lt-iam`; `deny` general de `apply`/`destroy`/`import`/`state`
  intacto.

## 3. Instance refresh

| Campo | Valor |
|---|---|
| ID | `d6708f22-e073-4e82-8118-cc8be002f93c` |
| Estrategia | Rolling, `MinHealthyPercentage=100`, `MaxHealthyPercentage=150`, `InstanceWarmup=120`, `ScaleInProtectedInstances=Refresh`, `SkipMatching=false`, sin AutoRollback |
| Capacidad | Max 2 → **3** transitorio; restaurado a **Min 2 / Desired 2 / Max 2** |
| Resultado | **`Successful` 100 %** |

**Incidente real de capacidad (declarado, no oculto).** Entre `03:17` y `03:23 UTC` fallaron
**cuatro** lanzamientos con «no hay capacidad suficiente de `t4g.micro` en us-east-2b»; AWS
recomendaba us-east-2a o us-east-2c. El ASG siguió reintentando y terminó provisionando. **No
hubo caída de servicio**: en todo momento hubo dos instancias sanas, aunque durante unos minutos
una sirvió E003 (v4) y otra E004 (v5). Consecuencia operativa registrada: un rollback futuro
también necesitará capacidad en us-east-2b y no es instantáneo.

## 4. Estado final de la flota

| Instancia | AZ | Estado | LT | Protegida |
|---|---|---|---|---|
| `i-09e9b8e81c8a6d054` | us-east-2b | `InService` / `Healthy` | **v5** | sí |
| `i-0dd03e076fc0058e2` | us-east-2a | `InService` / `Healthy` | **v5** | sí |

ASG `nexum-h08i-asg-backend`: `Min 2 / Desired 2 / Max 2`, `HealthCheckType=EC2`, gracia 120 s,
LT `lt-05445f32a7ee8a2f7` con `$Latest`. Target group `nexum-h08i-tg-backend`: **dos targets
`healthy`** en 8080. Instancias anteriores (`i-02666abdd2bdfae8d`, `i-0acdf148c0ffe16f2`)
sustituidas y terminadas.

## 5. Verificación HTTPS (BASE_URL vigente)

`https://d2lpmqy5w6x9es.cloudfront.net/api/v1` — cuenta sintética, sin registrar tokens,
contraseñas ni correos.

| Prueba | Esperado | Obtenido |
|---|---|---|
| E001 `POST /auth/register` | 201 | **201** |
| E002 `POST /auth/login` | 200 · `Bearer` · 900 · `session.id=sessionId` | **200** |
| E004 `POST /auth/logout` (Bearer, sin cuerpo) | 204 sin contenido | **204**, cuerpo de longitud **0** |
| E003 refresh de la sesión cerrada | 401 | **401** |
| E004 segundo logout con el JWT revocado | 401 | **401** |
| Segunda sesión: login + refresh | 200/200 con rotación | **200/200**, rotado, misma sesión |
| Tercera sesión independiente | 200/200 | **200/200** |
| Replay de refresh consumido | 409 | **409** |
| `GET /health/live` | 200 | **200** |

**Aceptación humana (`E004_USER_ACCEPTED=VERIFIED`, 2026-11-16).** El responsable ejecutó en
Postman, con cuenta sintética: login **200**, logout con `Bearer` y sin body **204 sin
contenido**, y refresh con el token vigente **no consumido** → **401** con
`code=UNAUTHORIZED`. No se registran tokens, correos ni credenciales. La prueba antigua que
devolvió **409** correspondía a reutilización de un refresh ya consumido y **no** es la prueba
de aceptación; la definitiva es la de **401**.

## 6. Limitaciones de verificación (explícitas)

- **DynamoDB:** la lectura directa del ítem para observar `status=REVOKED` quedó `NOT_RUN`
  porque `aws dynamodb get-item` está bajo el `deny` general y no se solicitó excepción. La
  revocación se acredita **funcionalmente** por HTTPS (refresh 401 y logout 401 posteriores).
- **Atribución por nodo:** CloudFront y ALB no exponen qué instancia atendió cada petición, de
  modo que no se afirma una prueba cruzada instrumentada. Lo demostrado es que **ambas**
  instancias están en v5, sanas y sirviendo el target group; el bootstrap falla cerrado si el
  digest del artefacto no coincide, así que dos nodos sanos implican dos nodos ejecutando E004.

## 7. Residuos documentales

`D-E004-04` — **RESUELTO** (2026-11-16). `plan.md` §2 paso 3 (línea 9) citaba
`SESSIONHASH#digest`, fuera de las dos frases que autorizó RFC-015. Con
`APRUEBO_ENMIENDA_DOCUMENTAL_E004_PLAN_PASO3` se corrigió esa única línea y E004 queda con **0
ocurrencias**. La misma frase de plantilla persiste en E003 y E005–E011: son specs `APPROVED`,
exigen RFC propio y no se tocan desde un cierre documental de E004.
