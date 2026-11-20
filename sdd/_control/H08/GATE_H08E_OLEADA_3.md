# Gate H08-E — Oleada 3 (13 endpoints)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/GATE_H08E_OLEADA_3.md` |
| Versión | 1.2.0 |
| Fecha | 2026-11-19 |
| Estado | **GATE CONCEDIDO — RFC-022 v1.2.0 PENDIENTE** |
| Autoridad que lo motiva | Instrucción humana de inicio de Oleada 3 (2026-11-19) |
| Instrumento de concurrencia | **RFC-019** (cláusulas **E-1**, **E-2**, **E-3**) |
| Instrumento contractual | **RFC-022** (`PROPUESTO`, frase separada) |
| Aprobador | Responsable de Nexum-Backend |
| Frases | §12 (dos, en orden) |

> Este documento **no** autoriza nada por sí mismo. Solo las frases exactas del §12 activan
> el alcance. Preparar el Gate no es aprobarlo.

## 1. Baseline verificado (2026-11-19)

| Dato | Valor comprobado |
|---|---|
| `origin/main` | `0169f006935b825747d0508b81c6c32661027baf` (PR #121) |
| `HEAD` local | `0169f006935b825747d0508b81c6c32661027baf` (fast-forward desde `c9f7ec1`) |
| Cambio local preservado | `opencode.json` (84 inserciones / 9 eliminaciones). No se commitea ni se revierte |
| PR abiertos | 0 |
| CI de `main` | Go Backend CI `44641630991` SUCCESS · SDD documental `44641631014` SUCCESS |
| ASG | `nexum-h08i-asg-backend` Min 2 / Desired 2 / Max 2 · subredes `us-east-2a`, `us-east-2b` |
| Instancias | `i-037aa73e08855de59`, `i-0c8119d2bee0a85ec` · InService/Healthy · Launch Template **v14** · **ambas en `us-east-2a`** |
| Target group | `nexum-h08i-tg-backend` · 2/2 targets `healthy` |
| Launch Template | `lt-05445f32a7ee8a2f7` `$Latest` v14 · release `v0.18.0+9722224c15c5` · AMI `ami-047e988c757245767` |
| DynamoDB | `NEXUM_CORE_H08I` `ACTIVE` · GSI `GSI1` · 519 ítems |
| Redis | `nexum-h08i-redis-001` `available` · `cache.t4g.micro` · Redis OSS |
| HTTPS | `GET /api/v1/health/live` → **200** `{"status":"UP"}` en `https://d2lpmqy5w6x9es.cloudfront.net/api/v1` |
| Plan / créditos | `FREE` / `ACTIVE` · **142.94 USD** restantes · expiración **2027-05-04** |
| Rollback | LT v13 + release `v0.17.0+87aac167fe28` conservados |
| Endpoints acumulados | 41/75 (26 Oleada 1 + 15 Oleada 2) |

**Limitaciones heredadas que este Gate no resuelve:** ambas EC2 en `us-east-2a`; SES en
sandbox (E006/E011 `PARTIAL`); E015 `2026-11-v3` sin Gate legal; trabajos antiguos de E014
`NOT_VERIFIED`; PITR/TTL no legibles por API; CID del estado real de AWS `NOT_VERIFIED`.

## 2. Alcance (13 E-ID)

`E020` · `E026` · `E037` · `E042` · `E043` · `E044` · `E045` · `E046` · `E047` · `E048` ·
`E049` · `E050` · `E051`.

Métodos, rutas, códigos de éxito, `Idempotency-Key`, `If-Match` y esquemas según
`sdd/03-endpoints/openapi/nexum-api-v1.{json,yaml}`, con las enmiendas **solo** si RFC-022
es aprobada.

**Fuera de alcance:** E001–E019, E021–E025, E052–E075, H08-I, H08-E de oleadas anteriores,
SES, E015, multi-AZ definitivo. No se inicia Oleada 4 ni Oleada 5.

## 3. Qué autoriza RFC-019 y qué exige autorización nueva

| Materia | Amparo | Estado |
|---|---|---|
| Varias specs en desarrollo simultáneo | RFC-019 **E-1** | Ya autorizado; no se re-aprueba |
| Una sola release agrupada | RFC-019 **E-2** | Ya autorizado; no se re-aprueba |
| Reutilización para Oleada 3 | RFC-019 **E-3** | Se invoca aquí; requiere frase 1 |
| Los 13 endpoints y su código | Gate §12 frase 1 | **Autorización nueva** |
| Cambios de contrato E020/E048 y semánticas §2–§8 | **RFC-022** | **Autorización nueva** (frase 2) |
| Cualquier merge | Gate por PR + HEAD SHA40 | **Autorización nueva, por PR** |
| Publicación S3 | Gate por release | **Autorización nueva** |
| `terraform apply` | Gate por SHA-256 del plan | **Autorización nueva** |
| `instance refresh` / contingencia | Gate por operación | **Autorización nueva** |
| Cualquier recurso AWS nuevo o acción IAM nueva | Gate de plan específico | **Autorización nueva** |

Las aprobaciones históricas (Oleadas 1 y 2, RFC-019, RFC-021) **no** se extrapolan a estos
endpoints.

## 4. Concurrencia y propiedad (RFC-019 E-1)

| Regla | Límite |
|---|---|
| Worktrees escritores | máx. **4** |
| E-ID activos simultáneos | máx. **8** |
| PR por endpoint | **obligatorio, 13 en total** |
| Release | **una sola**, agrupada |
| Tablas/índices DynamoDB nuevos | **prohibidos** |
| GSI2 | **prohibido** |
| `Scan` | **prohibido** |

Líneas previstas:

| Línea | Worktree | E-ID | Orden |
|---|---|---|---|
| A | `w3-recurrente` | E037 | único |
| B1 | `w3-aportaciones` | E043 → E044 → E045 → E046 → E047 | secuencial |
| B2 | `w3-metas-baja` | E042 (depende de B1 para probar 409/204) | al final de B1 |
| C | `w3-presupuestos` | E048 → E049 → E050 | secuencial |
| D | `w3-resumen` | E026 → E051 | secuencial |
| E | `w3-export` | E020 (requiere frase 2) | al final |

Como máximo 4 worktrees activos: A + B1 + C + D en la primera ventana; E entra al liberarse
una línea. B2 se integra al final de B1 en el mismo worktree. El orquestador escribe
`cmd/server/main.go`, los registros `cmd/server/registro_oleada3.go`/`e0NN.go`, el router,
`internal/domain/dynamo/keys.go`, OpenAPI, Terraform, `memoria.md`, `BASELINE_SHA256.json`,
`.gitleaks.toml` y workflows. Ninguna línea toca esos archivos.

## 5. Reconciliaciones y decisiones vinculantes

### 5.1 RFC-022 (frase 2) — cambios de contrato y semánticas

1. **E020:** `UserExport` se amplía con `income`, `recurringPayments`, `contributions`,
   `budgets`, `onboarding`, `preferences` (opcionales). Paginación por tramos de colecciones
   en orden fijo; singletons completos en cada página; cursor firmado. Sin esta aprobación,
   E020 queda `BLOCKED_CONTRACT` y **no se mergea**.
2. **E048/E049:** `E048?period=YYYY-MM` emite `ETag: "b<period>-<version>"`; `E049` lo exige
   en `If-Match` (428/412) y devuelve el conjunto completo del periodo (≤50) **sin**
   `nextCursor` ni parámetros de paginación, con `ETag` de respuesta. Creación con
   `"b<period>-0"`; representación `BUDGETSET#<period>` como ítem único con CAS. Sin esta
   aprobación, E048/E049/E050 quedan `BLOCKED_CONTRACT`.
3. **E026:** `FinancialSummary` no cambia. Se adopta la **alternativa B**: no se publica
   `availableEstimated` porque no existe una fuente determinista que clasifique cada
   recurrente como fijo o variable. E026 devuelve solo `income`, `expenses`, `savings`, `net`
   y `asOf`, con la semántica del §4 de RFC-022.
4. **E037/E042-E047/E050/E051:** semánticas internas y de borde documentadas en RFC-022
   §5-§8 (movimiento derivado y duplicado por fecha; categorías del catálogo real
   `otros`/`ingreso`/`ahorro`; tombstone de meta y 409 con aportaciones activas; claves
   `CONTRIBUTION#<fecha>#<id>` + puntero; `remaining`/`canCommit`; reversión sin borrar el
   movimiento; periodo por zona IANA; **E051 calcula directo desde DynamoDB, sin caché**).

### 5.2 Decisiones de implementación sin cambio de contrato

| ID | Decisión | Motivo |
|---|---|---|
| D-W3-01 | E037 mapea el tipo de la regla a una categoría del catálogo real: `expense→otros`, `income→ingreso`, `saving→ahorro`; `description = name` | El catálogo E022 no contiene `recurring`; no se inventa una categoría pública ni se atribuye una específica |
| D-W3-02 | Duplicado de E037 por fecha local del titular | Evita dobles pagos por cambio de día UTC; la zona se obtiene de preferencias |
| D-W3-03 | E042 no revierte aportaciones; exige E047 previo (409) | Una aportación puede repartirse entre varias metas; la reversión automática afectaría a terceras metas |
| D-W3-04 | E044 crea **un** movimiento de ahorro por el total, no uno por meta | Evita duplicar `savings` y mantiene el vínculo `contributionId` |
| D-W3-05 | E047 no borra el movimiento; el estado `reversed` de la aportación manda en agregados | Conserva historia financiera y evita doble conteo |
| D-W3-06 | E051 devuelve hasta 100 metas activas y 10 movimientos recientes | El contrato no fija cardinalidad; se acota de forma determinista y documentada |
| D-W3-07 | E051 calcula directamente desde DynamoDB, sin caché de dashboard | No existe un mecanismo probado que garantice consistencia tras una mutación; la caché queda como optimización posterior con su propio Gate |
| D-W3-08 | Periodo por defecto en la zona IANA del titular, `America/Mexico_City` si falta | Consistencia entre nodos y con la app |

## 6. Definition of Done por endpoint

Handler real con DTO estricto y campos desconocidos rechazados; servicio de aplicación con
reglas de dominio; importes `int64` centavos (`internal/domain/money`), nunca `float64`;
ownership desde `sub`; recurso ajeno → 404; `Query`/`GetItem` sin `Scan`; Redis solo
sesiones, cuotas u otros usos ya publicados; E051 **sin** caché de dashboard; `Idempotency-Key`
según §5.2 del Gate de Oleada 2; `If-Match`/ETag en E048/E049 (428/412/200); transacciones
condicionadas y sin doble escritura; pruebas de aplicación, adaptador y transporte;
`gofmt`/`build`/`vet` limpios; sin hallazgos nuevos de `staticcheck`/`gosec`/`gitleaks`/
Trivy; `tasks.md`/`checklist.md` con evidencia real.

Si el host bloquea la ejecución de pruebas, la autoridad es el CI Linux y se declara
`NOT_RUN_LOCAL`, nunca PASS.

## 7. Integración y merges

Ensayo acumulativo en `chore/h08e-oleada3-integracion` antes del primer merge; no se mergea.
Gate por PR: `APRUEBO_MERGE_PR_<N>_SHA_<SHA40>`; si el HEAD cambia, el Gate queda `STALE` y
se renueva solo ese PR. Sin `--admin`, sin force push, sin bypass. `strict=true` se mantiene.
Cada PR corrige sus fallos de CI dentro de sí mismo.

## 8. Release agrupada (RFC-019 E-2)

Artefacto ARM64 del CI de `main` final con SHA-256 verificado · SBOM · unidad · publicación S3
inmutable (`If-None-Match: *`, SSE `AES256`, checksum) · plan Terraform acotado a Launch
Template, `terraform_data` de invariantes y ARN S3 nuevos en `runtime` · apply con Gate de
SHA-256 · **un** `instance refresh` `MinHealthyPercentage=100` · capacidad restaurada a 2/2/2 ·
rollback conservado (LT v14 + `v0.18.0+9722224c15c5`).

No se cambia DynamoDB, Redis, VPC, CloudFront, ALB, SES, secretos ni topología por rutina.
Si `us-east-2b` no tiene capacidad `t4g.micro`, se aplica el procedimiento de contingencia
documentado y se solicita un Gate **nuevo** para cambiar subredes o capacidad.

## 9. Certificación funcional AWS

Mínimo dos usuarios sintéticos independientes, datos controlados y cuentas desechables para
E014. Por endpoint se exige HTTP positivo real, persistencia y aislamiento; para E014 se
repite la matriz completa con las nuevas familias. No se declara `FUNCTIONAL_PASS` por un
401/404 ni por CI verde. E006/E011 siguen `PARTIAL`; los trabajos antiguos de E014 siguen
`NOT_VERIFIED`.

## 10. Riesgos declarados

| ID | Riesgo | Tratamiento |
|---|---|---|
| R-01 | Ambas EC2 en `us-east-2a` | Limitación heredada; no se declara multi-AZ |
| R-02 | Capacidad `t4g.micro` en `us-east-2b` | Contingencia documentada + Gate nuevo si hace falta |
| R-03 | RFC-022 pendiente | Frase 2; E020/E048/E049/E050 bloqueados hasta entonces |
| R-04 | Aportaciones con muchas metas | Límite 12 por request; transacción ≤18 acciones |
| R-05 | Meta con aportaciones activas | E042 responde 409; el cliente revierte con E047 |
| R-06 | Caché de dashboard | E051 no usa caché en esta oleada; se elimina T-2 y no se amplía Redis |
| R-07 | SES/E015/multi-AZ | Fuera de esta oleada; se reportan sin ocultarlos |
| R-08 | Windows Application Control | `NOT_RUN_LOCAL`; CI Linux es la autoridad |

## 11. Prohibiciones

`Scan`; mocks en producción; recursos o índices nuevos; `terraform destroy/import/state`;
editar `spec.md`/`plan.md` sin RFC; commitear `opencode.json`, secretos, tfstate/tfvars,
tokens o PII; tocar E001–E019, E021–E025, E052–E075; tocar SES o E015; declarar
`AWS_VERIFIED` sin HTTP real; declarar 54/75 sin evidencia de los 13 endpoints.

## 12. Frases de autorización

**Orden obligatorio:** (1) Gate de la oleada → permite implementar y abrir PR de las líneas
que no dependen de RFC-022. (2) RFC-022 → necesaria **antes de mergear** E020/E048/E049 y
antes de aplicar sus semánticas públicas.

Frase 1 — Gate Oleada 3:

```text
APRUEBO_GATE_OLEADA_3_RFC019_E020_E026_E037_E042_E043_E044_E045_E046_E047_E048_E049_E050_E051
```

Frase 2 - contratos de Oleada 3 (v1.2.0):

```text
APRUEBO_RFC_022_CONTRATOS_OLEADA_3_V1_2_0
```

Frase 3 - cambio transversal T-1 (solo E030):

```text
APRUEBO_CAMBIO_TRANSVERSAL_E030_MOVIMIENTO_APORTACION
```

Ninguna autoriza merges, `terraform apply`, publicación S3, `instance refresh` ni cambios de
SES/E015. Cada PR, el release y el despliegue exigen sus Gates propios.

### 12.1 Registro de aprobación

| Frase | Estado | Fecha | Efecto |
|---|---|---|---|
| `APRUEBO_GATE_OLEADA_3_RFC019_E020_E026_E037_E042_E043_E044_E045_E046_E047_E048_E049_E050_E051` | **CONCEDIDA** | 2026-11-19 | Habilita implementación, ramas y PR de los 13 endpoints. RFC-022 sigue `PROPUESTO`; E020/E048/E049/E050 y las semánticas públicas quedan `CONTRACT_BLOCKED` hasta su frase propia |
| `APRUEBO_RFC_022_CONTRATOS_OLEADA_3` | **RETIRADA** | 2026-11-19 | v1.0.0 no aprobada |
| `APRUEBO_RFC_022_CONTRATOS_OLEADA_3_V1_1_0` | **RETIRADA** | 2026-11-19 | v1.1.0 no aprobada; sustituida por la v1.2.0 |
| `APRUEBO_CAMBIOS_TRANSVERSALES_OLEADA_3` | **RETIRADA** | 2026-11-19 | Incluía T-2 (hooks de caché), eliminado en v1.2.0 |
| `APRUEBO_RFC_022_CONTRATOS_OLEADA_3_V1_2_0` | **PENDIENTE** | — | Habilita los cambios de contrato y semánticas de RFC-022 v1.2.0 |
| `APRUEBO_CAMBIO_TRANSVERSAL_E030_MOVIMIENTO_APORTACION` | **PENDIENTE** | — | Habilita solo el guardo T-1 de E030 |

Siguen requiriendo autorización propia y separada: merge de cada PR, publicación S3,
`terraform apply` por SHA-256, `instance refresh` y cualquier contingencia de capacidad.

### 12.2 Registro de la revisión de RFC-022

La v1.0.0 y la v1.1.0 fueron revisadas por el responsable y **no aprobadas**. Observaciones
vinculantes de la auditoría que dio origen a la v1.2.0:

1. **E049** no pagina la respuesta: devuelve el conjunto completo (≤50), sin `nextCursor` ni
   `limit`/`cursor`; E048 conserva la paginación; ETag en E048 y E049.
2. **E026** no publica `availableEstimated`: no hay fuente determinista fija/variable. Se
   adopta la alternativa B.
3. **E051** calcula directamente desde DynamoDB, sin caché; se elimina T-2 y no se amplía
   Redis.
4. **E020** no promete snapshot transaccional ni ausencia absoluta de duplicados; documenta
   garantías y limitaciones reales.
5. **E037** no usa `recurring` (no existe en el catálogo): mapea por tipo a
   `otros`/`ingreso`/`ahorro`.
6. **E042** mantiene la alternativa B y enmienda expresamente RFC-016 §17.7.
7. **T-1** se limita a E030; E031/E027/E029 no cambian.
8. La matriz de decisiones y las frases de la v1.2.0 se presentan en el informe de
   continuación.
