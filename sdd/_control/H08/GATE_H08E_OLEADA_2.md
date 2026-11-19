# Gate H08-E — Oleada 2 (15 endpoints)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/GATE_H08E_OLEADA_2.md` |
| Versión | 1.1.0 |
| Fecha | 2026-11-18 |
| Estado | **APPROVED** — frases concedidas por el responsable el 2026-11-18 (registro en §12.1) |
| Autoridad que lo motiva | Instrucción humana del 2026-11-18: «YA EMPEZAMOS OLEADA 2» |
| Instrumento de concurrencia | **RFC-019** (cláusula **E-3**, reutilización por oleada) |
| Instrumento contractual | **RFC-021** (frecuencias de recurrentes) — `PROPUESTO`, frase separada |
| Diseño destructivo | `DISENO_E014_PURGA.md` |
| Texto legal | `PROPUESTA_AVISO_E015_OLEADA_2.md` — `DRAFT`, no aplicado |
| Aprobador | Responsable de Nexum-Backend |
| Frases | §12 (dos, con orden) |

> Este documento **no** autoriza nada por sí mismo. Solo las frases exactas del §12 activan
> el alcance. Preparar el Gate no es aprobarlo.

## 1. Baseline verificado (2026-11-18)

| Dato | Valor |
|---|---|
| `HEAD` = `origin/main` | `c9f7ec1688666718768fdfa3d7a19cbeea1fa1a2` |
| Cambio local preservado | `opencode.json` (permisos locales). No se commitea ni se revierte |
| PR abiertos | 0 |
| CI de `main` | Go Backend CI `success` · SDD documental `success` |
| ASG | `nexum-h08i-asg-backend` 2/2/2 · subredes `us-east-2a`,`us-east-2b` |
| Instancias | `i-0d2d27a4e52a4ce40`, `i-0e6a60fcf2749e710` · `InService`/`Healthy` · **ambas en `us-east-2a`** |
| Launch Template | `lt-05445f32a7ee8a2f7` `$Latest` v12 = release `v0.16.1+615ac1c734f2` |
| Target group | 2 targets `healthy` |
| DynamoDB | `NEXUM_CORE_H08I` `ACTIVE` · GSI `GSI1` · 430 ítems |
| Redis | `nexum-h08i-redis` `available` · `cache.t4g.micro` |
| PITR / TTL por API | `NOT_VERIFIED` (permiso de herramienta denegado). No se declara verificado |
| Endpoints publicados | 26 → objetivo de esta oleada 41 |

## 2. Alcance (15 E-ID)

E014 · E027 · E028 · E029 · E030 · E031 · E032 · E033 · E034 · E035 · E036 · E038 · E039 ·
E040 · E041.

Métodos, rutas, códigos de éxito, `Idempotency-Key` e `If-Match` según
`sdd/03-endpoints/openapi/nexum-api-v1.{yaml,json}` (autoridad de transporte, 75 operaciones).

**Fuera de alcance:** E001–E013, E015–E019, E020, E021, E022–E025, E026, E037, E042–E075.

## 3. Qué autoriza RFC-019 y qué exige autorización nueva

| Materia | Amparo | Estado |
|---|---|---|
| Varias specs en desarrollo simultáneo | RFC-019 **E-1** | **Ya autorizado**; no se re-aprueba |
| Una sola release agrupada | RFC-019 **E-2** | **Ya autorizado**; no se re-aprueba |
| Reutilización para esta oleada | RFC-019 **E-3** | Se invoca aquí; requiere el Gate del §12 |
| Los 15 endpoints y su código | Gate §12 frase 1 | **Autorización nueva** |
| Corrección aditiva de enum de recurrentes | **RFC-021** | **Autorización nueva** (frase 2) |
| Cualquier merge | Gate por PR + HEAD SHA | **Autorización nueva, por PR** |
| `terraform apply` | Gate por SHA-256 del plan | **Autorización nueva** |
| Publicación S3 | Gate por release | **Autorización nueva** |
| Cambio del texto de E015 | Gate con hash del texto nuevo | **Autorización nueva** |
| Cualquier recurso AWS nuevo o acción IAM nueva | Gate de plan específico | **Autorización nueva** |

La aprobación histórica de E007–E011 y la de Oleada 1 **no** se extrapolan a estos endpoints.

## 4. Concurrencia y propiedad (RFC-019 E-1)

| Regla | Límite |
|---|---|
| Worktrees escritores | máx. **4** |
| Endpoints activos simultáneos | máx. **8** |
| PR por endpoint | **obligatorio, 15 en total** |
| Release | **una sola**, agrupada |
| Tablas/índices DynamoDB nuevos | **prohibidos** |

Orden por línea: A `E028→E027→E029→E030→E031` · B `E033→E032→E034→E035→E036` ·
C `E039→E038→E040→E041` · D `E014` (integra al final).

Propietario único (orquestador): `cmd/server/main.go`, `cmd/server/registro_oleada2.go`,
`internal/transport/http/router.go`, `internal/domain/dynamo/keys.go`, OpenAPI,
`infra/terraform/**`, `memoria.md`, `BASELINE_SHA256.json`, `.gitleaks.toml`, workflows.
Las líneas solo escriben sus paquetes de dominio/aplicación/adaptador/transporte y sus pruebas.

## 5. Correcciones obligatorias

### 5.1 F-01 — Frecuencias de recurrentes → RFC-021 (**bloquea línea B**)

`RecurringCreate.frequency` y `RecurringPayment.frequency` publican 4 valores
(`weekly,biweekly,monthly,yearly`); E023 emite 7 canónicos y E001 registra esos 7.
RFC-021 propone el conjunto aditivo de **9 valores** (ninguno retirado), emisión de **7
canónicos**, normalización `biweekly→fortnightly` y `yearly→annual`, sin tocar `RecurringPatch`
(no admite `frequency`). Sin la frase 2, E033/E032/E034/E035/E036 quedan `BLOCKED_CONTRACT` y
**no se mergean**; el resto de líneas continúa.

### 5.2 F-02 — Idempotencia financiera aislada por usuario (E028/E033/E039)

`IdempotencyKeys(operation,key)` no incluye usuario. Se añade, sin tocar la heredada de E001:

```go
// PK = USER#<userID> ; SK = IDEM#<operación>#<sha256(operación:clave)>
func UserIdempotencyKeys(userID, operation, idempotencyKey string) (pk, sk string)
```

- `USER#*` ya está en `LeadingKeys` → **sin cambio IAM**.
- Registro: `payloadHash` (SHA-256 del DTO validado y normalizado), `status`, `httpStatus`,
  `responseBody` inmutable, `resourceId`, `createdAt`, `expiresAt` (vencimiento lógico 24 h).
- Flujo: lectura consistente → si `COMPLETED` y mismo `payloadHash` → replay con
  `X-Idempotent-Replay: true` y código original; si `payloadHash` distinto → **409**.
- Escritura: recurso + registro de idempotencia en la **misma** `TransactWriteItems` con
  condición `attribute_not_exists(PK)` sobre el registro → atomicidad y ausencia de duplicados.
- Cancelación por condición: relectura; mismo payload → replay; distinto → 409; sin registro
  (carrera) → un reintento y, si persiste, 409/503 sin duplicar.
- Dos usuarios con la misma clave **nunca** comparten resultado (PK distinta); prueba explícita.

### 5.3 F-03 — E014 con purga real

Diseño completo en `sdd/_control/H08/DISENO_E014_PURGA.md`. Resumen vinculante:

1. `confirmation = DELETE_MY_ACCOUNT` + verificación Argon2id; contraseña incorrecta → **422**
   (consistente con E007), nunca 401 por contraseña.
2. `TransactWriteItems` de 3 ítems: `PROFILE.status = DELETION_PENDING` + `deletedAt` +
   `sessionEpoch + 1` + `version + 1`; `USER#<id>/DELETION#<requestId>` (`PENDING`);
   y `QUEUE#DELETION/<requestId>` (`SK` estable con `dueAt` como atributo).
3. Revocación inmediata efectiva: `AuthorizeSession` exige `PROFILE.status = ACTIVE` con lectura
   consistente y época alineada → todo access y refresh dejan de valer en ambos nodos.
4. Worker real en el binario con lease en Redis (patrón de E005), reclamación atómica **que
   incluye trabajos `RUNNING` con `leaseUntil` vencido**, `fence` incremental y todas las
   escrituras de progreso/cierre condicionadas a `owner` + `fence`: el propietario anterior no
   puede completar ni sobrescribir. El descubrimiento consulta `PENDING`/`RETRY`/`SLOW_RETRY` por
   `dueAt` **y `RUNNING` por `leaseUntil` vencido**, y pagina con `LastEvaluatedKey` aunque una
   página no aporte coincidencias. Reintentos con backoff y `SLOW_RETRY` cada 24 h (nunca
   abandonados). No hay servicio nuevo.
5. Purga por lotes en fases `COLLECT` → `EXTERNAL` → `PARTITION` → `VERIFY` → `TERMINAL`, con
   cursor `lastSK` persistido. El recorrido **excluye** `DELETION#<requestId>`: el control de
   progreso nunca se borra. `PURGED` solo tras verificar el catálogo. Se purgan `USER#<id>`,
   `EMAIL#<sha256>`, `REFRESH#<digest>`, entradas de `QUEUE#PWRESET` del titular y caché
   (`UserCachePrefix`). Sin `BatchWriteItem` (no autorizado): `Query`/`DeleteItem` ya permitidos.
6. Residuos no localizables (`RESET#`, `ONE_USE#`, indexados por hash de token, y el `IDEM#`
   heredado de E001) se declaran con **vencimiento lógico**, quedan inutilizables y su
   eliminación física es asíncrona por TTL, **sin plazo prometido**; no se declaran borrados.
7. `202 scheduled` solo es válido porque el worker existe, es durable y su progreso es observable.

### 5.4 F-04 — E015

El aviso aprobado (`2026-11-v2`) declara movimientos, metas y eliminación automática como no
disponibles. Propuesta `2026-11-v3` en `PROPUESTA_AVISO_E015_OLEADA_2.md` (**DRAFT**): inventario
de afirmaciones afectadas, redacción propuesta, impacto en hash y plan de versionado. **No se
aplica** sin Gate propio con el hash nuevo. No se toca `nexumpr/nexum-privacidad`.

### 5.5 F-05 — SES y límites

La solicitud de producción de SES se gestiona **en otra sesión**. Aquí no se tramita, no se
modifican permisos y no se depende de ella. E006/E011 se revalidan solo si hay condiciones
autorizadas; si no, quedan `PARTIAL` declarados como hoy.

## 6. Definition of Done por endpoint

Handler real con DTO estricto y campos desconocidos rechazados; servicio de aplicación con
reglas de dominio; importes `int64` centavos (`internal/domain/money`), nunca `float64`;
ownership desde `sub`; recurso ajeno → 404; `Query`/`GetItem` sin `Scan`; Redis solo caché o
cuota; `If-Match`/ETag en E030/E035/E041 (`428`/`412`/`200`); idempotencia según §5.2;
reglas financieras (E031/E036 no borran movimientos con aportación confirmada; E041 no baja
`targetAmount` de `savedAmount`; E038 `totals` sobre la colección completa); pruebas de
aplicación, adaptador y transporte; `gofmt`/`build`/`vet` limpios; sin hallazgos nuevos de
`staticcheck`/`gosec`/`gitleaks`/Trivy; `tasks.md`/`checklist.md` con evidencia real.

Si el host bloquea la ejecución de pruebas, la autoridad es el CI Linux y se declara
`NOT_RUN_LOCAL`, nunca PASS.

## 7. Integración y merges

Ensayo acumulativo en `chore/h08e-oleada2-integracion` (A→C→B→D) antes del primer merge; no se
mergea. Gate por PR: `APRUEBO_MERGE_PR_<N>_SHA_<SHA40>`; si el HEAD cambia, el Gate queda
`STALE` y se renueva solo ese PR. Sin `--admin`, sin force push, sin bypass. `strict` se
mantiene.

## 8. Release agrupada (RFC-019 E-2)

Artefacto ARM64 del CI de `main` final con SHA-256 verificado · SBOM · unidad · publicación S3
inmutable (`If-None-Match: *`, SSE `AES256`, checksum) · plan Terraform acotado a Launch
Template, `terraform_data` de invariantes y ARN S3 nuevos en `runtime` · apply con Gate de
SHA-256 · **un** `instance refresh` `MinHealthyPercentage=100` · capacidad restaurada a 2/2/2 ·
rollback conservado.

## 9. Riesgos declarados

| ID | Riesgo | Tratamiento |
|---|---|---|
| R-01 | Ambas EC2 en `us-east-2a` | Limitación heredada; no se declara multi-AZ |
| R-02 | PITR/TTL no legibles por API | `NOT_VERIFIED`, sin inventar |
| R-03 | `IDEM#` de E001 sin usuario | Vencimiento lógico 24 h; eliminación física asíncrona del proveedor, sin plazo prometido. Corrección futura con RFC |
| R-04 | `ONE_USE#`/`RESET#` no enumerables | Vencimiento lógico 30 min; inutilizables al exigir `PROFILE` activo; eliminación física asíncrona. Documentado, no “borrado” |
| R-05 | Purga sin `BatchWriteItem` más lenta | Lotes paginados, worker con backoff |
| R-06 | RFC-021 pendiente | Frase 2; línea B bloqueada hasta entonces |
| R-07 | Aviso E015 desactualizado | Propuesta + Gate con hash antes de anunciar E014 |
| R-08 | SES en sandbox | Fuera de esta sesión |

## 10. Prohibiciones

`Scan`; mocks en producción; recursos o índices nuevos; `terraform destroy/import/state`;
editar `spec.md`/`plan.md` sin RFC; commitear `opencode.json`, secretos, tfstate/tfvars, tokens o
PII; tocar E001–E013, E015–E019, E022–E025, E068–E071; declarar `AWS_VERIFIED` sin HTTP real.

## 11. Evidencia de cierre

Por endpoint: PR, HEAD SHA, run CI, pruebas ejecutadas con resultado, códigos HTTP reales,
`DEPLOYED` y `AWS_VERIFIED` separados. Global: artefacto y plan con SHA-256, refresh id,
instancias, targets, regresión de 26 heredados y E006/E011 declarados `PARTIAL` o `PASS` según
evidencia disponible.

## 12. Frases de autorización

**Orden obligatorio:** (1) Gate de la oleada → permite implementar y abrir PR.
(2) RFC-021 → necesaria **antes de mergear** la línea B.

Frase 1 — Gate Oleada 2:

```text
APRUEBO_GATE_OLEADA_2_RFC019_E014_E027_E028_E029_E030_E031_E032_E033_E034_E035_E036_E038_E039_E040_E041
```

Frase 2 — enmienda contractual de frecuencias:

```text
APRUEBO_RFC_021_FRECUENCIAS_RECURRENTES
```

Ninguna de las dos autoriza merges, `terraform apply`, publicación S3 ni el cambio del texto de
E015.

### 12.1 Registro de aprobación

| Frase | Estado | Fecha | Efecto |
|---|---|---|---|
| `APRUEBO_GATE_OLEADA_2_RFC019_E014_E027_E028_E029_E030_E031_E032_E033_E034_E035_E036_E038_E039_E040_E041` | **CONCEDIDA** | 2026-11-18 | Habilita implementación, ramas y PR de los 15 endpoints |
| `APRUEBO_RFC_021_FRECUENCIAS_RECURRENTES` | **CONCEDIDA** | 2026-11-18 | Habilita la corrección aditiva del enum de recurrentes |

Siguen requiriendo autorización propia y separada: merge de cada PR, `terraform apply`, publicación
S3, `instance refresh` y el cambio de texto de E015.
