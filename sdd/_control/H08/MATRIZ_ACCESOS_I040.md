# I-040 — Matriz de accesos, seguridad y FinOps de la tabla `NEXUM_CORE`

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/MATRIZ_ACCESOS_I040.md` |
| Fecha | 2026-11-07 |
| Spec | `sdd/02-infraestructura/I-040-dynamodb/` |
| Autoridad física | `MATRIZ_FISICA_DYNAMODB_E001_E075.md` (H05) + A-060 |
| Autoridad de patrones | `sdd/03-endpoints/DYNAMODB_ACCESOS_H06.md` |
| Implementación | `infra/terraform/modules/dynamodb` (local, `AWS_NOT_RUN`) |
| Estado | `IMPLEMENTADO_LOCAL` · verificación AWS `NOT_RUN` |

## 0. Precedencia y discrepancia registrada

La matriz física H05 y A-060 fijan la tabla como **`NEXUM_CORE_<env>`**; `DYNAMODB_ACCESOS_H06.md` la
menciona como «tabla propuesta `nexum-core-<entorno>`». Se implementa **`NEXUM_CORE_<ENV>`** porque la autoridad
del diseño físico es H05 y el nombre de A-060 ya está fijado en el código Go (`internal/domain/dynamo`). La
diferencia es **sólo de capitalización del nombre físico**, no de claves, índices ni semántica; queda registrada
para que un futuro cambio de nombre exija RFC y migración, nunca un renombrado silencioso.

## 1. Esquema físico implementado

| Elemento | Valor |
|---|---|
| Tabla | `NEXUM_CORE_<ENV>` (una sola) |
| PK / SK | `PK` (S) / `SK` (S) |
| GSI1 | `GSI1PK` (S) / `GSI1SK` (S), proyección `ALL` |
| GSI2 | `GSI2PK` (S) / `GSI2SK` (S), proyección `INCLUDE {status, dueAt, entityType}`, **opt-in** |
| TTL | `expiresAt` (N, epoch seconds), habilitado |
| Cifrado | SSE con clave administrada por AWS |
| Capacidad | `PAY_PER_REQUEST` |
| PITR | Habilitado por variable (presupuestado) |
| Streams | Deshabilitados |

## 2. Matriz de patrones de acceso

Columnas: **Op.** operación DynamoDB · **Clave/índice** · **Consulta** · **Consistencia** · **Condición de
escritura** · **Transacción** · **Aislamiento** · **Costo**.

| Entidad (SK) | Op. | Clave / índice | Consulta | Consistencia | Condición | Transacción | Aislamiento | Costo |
|---|---|---|---|---|---|---|---|---|
| `PROFILE`, `INCOME`, `PREFERENCES`, `ONBOARDING`, `NOTIFICATION_PREF` | Get / Put / Update | `PK=USER#<sub>`, SK exacta | `GetItem` | Fuerte cuando se valida estado; eventual por defecto | `attribute_not_exists(PK)` al crear; `version = :v` al actualizar | Con `EMAIL#` al registrar | `sub` de JWT; nunca `userId` del cliente | 1 RRU/WRU por ítem ≤4 KB |
| `CONSENT#<ts>#<id>` | Query / Put | `PK=USER#<sub>` | `Query` + `begins_with(SK,"CONSENT#")` | Eventual | Append-only, sin update | No | Por partición de usuario | 1 RRU por página de 4 KB |
| `SESSION#<id>`, `REFRESH#<sha256>`, `SECURITY#<epoch>` | Get / Put / Delete | `PK=SESSION#<hash>` o `USER#<sub>` | `GetItem` / `Query` acotada | **Fuerte** al revocar | CAS de un solo uso (`attribute_not_exists` / versión de familia) | Revocación con `securityEpoch` | Token sólo como hash | Bajo: ítems pequeños |
| `MOVEMENT#<UTC>#<id>` | Query / Get / Put / Update / Delete | `PK=USER#<sub>` (rango SK) y `GSI1` | `Query` por rango de SK; `Query` GSI1 por tipo | Fuerte cuando se verifica saldo | `version`, `attribute_not_exists` por idempotencia | Sí (movimiento + locator + proyección) | `sub` de JWT | 1 WRU base **+ 1 WRU GSI1** por movimiento ≤1 KB |
| `MOVEMENT_ID#<id>` (locator) | Get / Put | `PK=USER#<sub>`, SK locator | `GetItem` + `GetItem` de la SK ordenada | Fuerte | Escrito en la misma transacción del movimiento | Sí | Mismo usuario | 2 lecturas fuertes por acceso por id |
| `RECURRING#<id>`, `PAYMENT#<rec>#<period>`, `RECURRING_TOKEN#<period>` | Get / Query / Put | `PK=USER#<sub>` | `GetItem` o `Query` por prefijo | Fuerte al pagar | Token de periodo único (`attribute_not_exists`) | Sí (pago + movimiento + locator + proyección) | `sub` de JWT | Similar a movimientos; sin GSI |
| `GOAL#<id>`, `CONTRIBUTION#<id>` | Query / Get / Put | `PK=USER#<sub>` | `Query` por prefijo | Fuerte | `version` optimista; suma de asignaciones = total | Sí (hasta 12 asignaciones) | `sub` de JWT | Transacción con hasta ~14 ítems |
| `BUDGET#<YYYY-MM>#<CAT>` | Query / Put / Update | `PK=USER#<sub>` | `Query` por prefijo de periodo | Eventual / fuerte al recalcular | Replace con `version`; inversión estable | Con movimientos que lo afecten | `sub` de JWT | 1 ítem por presupuesto |
| `NOTIFICATION#<UTC>#<id>`, `NOTIFICATION_ID#<id>` | Query / Get / Update | `PK=USER#<sub>` | `Query` por rango | Eventual | CAS de estado leído | No | `sub` de JWT | Bajo |
| `DEVICE#<id>`, `REMINDER#<id>` | Get / Query / Put / Delete | `PK=USER#<sub>`; `GSI2` sólo si el worker existe | `GetItem` / `Query`; `Query` GSI2 por `DUE#<fecha>#<shard>` | Eventual en el worker | Claim condicional por recordatorio | Con consentimiento | Worker sólo recorre `DUE#`, no la tabla | GSI2 replicaría escrituras de recordatorios |
| `IDEM#<scope>#<keyHash>` | Put | `PK=USER#<sub>` o `EMAIL#` según alcance | `GetItem` / `Put` | Fuerte | `attribute_not_exists` + hash de cuerpo | Sí, en la operación de negocio | Mismo usuario | 1 WRU + TTL de limpieza |
| `EMAIL#<sha256(email)>` / `OWNER` | Get / Put | `PK=EMAIL#<hash>` | `GetItem` | **Fuerte** | `attribute_not_exists` (unicidad) | Sí, con `PROFILE` | Hash, nunca email claro | 1 WRU por alta |
| Catálogos `PUBLIC#CATALOG`, `PUBLIC#CONTENT` | Query / Get | `PK=PUBLIC#…` | `Query` acotada por prefijo | Eventual | Read-only versionado | No | Público, sin datos personales | Bajo |

## 3. ¿Alguna operación exige `Scan`?

**No.** Recorrido de las 75 operaciones (E001–E075) contra el esquema implementado:

- **Identidad y sesiones (E001–E014):** `EMAIL#` (unicidad), `USER#<sub>` (perfil/sesiones) y `SESSION#<hash>`.
  Todas por clave exacta.
- **Privacidad y consentimientos (E015–E020):** `USER#<sub>` con `Query` por prefijo.
- **Movimientos, recurrentes, metas y presupuestos (E021–E050):** `USER#<sub>` con rango de SK; el filtrado por
  tipo usa `GSI1` (no `Scan`). La alternativa «Query + filtro en memoria» existe y está documentada en H06, pero
  produce páginas vacías; se implementó `GSI1` por eficiencia y por soporte explícito de E027–E031.
- **Estadísticas y dashboard (E051–E056):** agregados sobre `MOVEMENT#` por rango de SK del propio usuario.
- **Notificaciones y recordatorios (E057–E067):** `USER#<sub>` para el usuario; el barrido programado usaría
  `GSI2` (`DUE#`), que **no está creado** porque no existe worker confirmado. Sin worker, ninguna operación
  autorizada requiere recorrer la tabla completa.
- **Preferencias, ayuda, salud y voz (E068–E075):** `USER#<sub>` y `PUBLIC#`.

**Conclusión:** todas las operaciones previstas se resuelven con `GetItem`, `Query` por partición de usuario o
`Query` de `GSI1`. `Scan` sigue prohibido por A-060 (§2) y no se ha habilitado ninguna ruta que lo necesite.

## 4. Seguridad y datos financieros

- **Aislamiento por usuario:** la partición es `USER#<sub>` con `sub` extraído del JWT (A-050); el cliente nunca
  aporta `userId` como autoridad (R-031). El diseño no permite consultas cruzadas sin conocer la partición.
- **Escrituras condicionales y concurrencia:** `attribute_not_exists` para reservas (email, idempotencia,
  token de periodo) y `version` para actualizaciones optimistas; los conflictos se resuelven en la aplicación
  (A-070).
- **Idempotencia y relaciones:** `IDEM#` con TTL y las transacciones descritas en §2 evitan duplicados y estados
  parciales; el límite de 100 acciones / 4 MB se valida en la aplicación antes de escribir.
- **Índices:** `GSI1` es sparse (sólo ítems con `GSI1PK`), por lo que no replica toda la tabla. `GSI2` incluye
  sólo tres atributos de control, nunca importes.
- **Cifrado:** SSE en reposo con clave administrada por AWS. **Sin KMS propio** (coste fijo no presupuestado);
  si se exigiera clave propia, sería RFC con coste.
- **IAM:** la política de mínimo privilegio pertenece a **I-060**; este módulo sólo expone `table_arn` e
  `indices` para que esa spec acote `dynamodb:GetItem/Query/PutItem/UpdateItem/DeleteItem/TransactWriteItems`
  y `ConditionCheckItem` sobre la tabla y sus índices. No se crea ningún rol aquí (fuera de Gate).
- **Secretos:** ninguno en Terraform, variables ni user-data. `terraform validate` no revela credenciales.

## 5. FinOps — impacto de I-040 sobre el presupuesto I-000

Base: catálogo verificado de I-000 (`infra/finops/precios_us-east-2.json`); no se consultan precios nuevos ni se
usa Cost Explorer. **Ninguna cifra de esta sección es una medición de AWS.**

| Partida | Supuesto I-000 | Efecto de I-040 | Riesgo |
|---|---|---|---|
| `DDB_STORAGE` 3 GB-mes | datos + réplicas de índice | `GSI1` con proyección `ALL` **duplica** el almacenamiento de los ítems de movimiento. Con 3 GB ya presupuestados y datos de demostración (≪1 GB), sigue dentro de la línea | Bajo. Si el volumen real supera 3 GB, primer punto de desvío |
| `DDB_LECTURAS` 2 M RRU | lecturas eventuales/fuertes | `Query` por partición y `GSI1` consumen 0.5–1 RRU por 4 KB; `GetItem` fuerte 1 RRU. El diseño **reduce** lecturas frente a `Scan` (que cobraría la tabla completa) | Ninguno añadido; el diseño es más barato que la alternativa |
| `DDB_ESCRITURAS` 0.6 M WRU | escrituras de aplicación | `GSI1` añade **1 WRU por escritura de movimiento/actualización**. Con 0.6 M WRU presupuestados, duplicar el tramo de movimientos añadiría ≈0.375 USD/45 d (0.6 M × 0.625 USD/M), dentro del margen de **6.9387 USD** | **Bajo pero real**; registrado, no ocultado |
| `DDB_PITR` 3 GB-mes (0.6000 USD) | PITR sobre datos | Habilitado por variable (`true` en h08i). PITR se factura por GB monitorizado | Presupuestado; no medido |
| `DDB_BACKUP` 3 GB-mes (0.3000 USD) | copias programadas | **No se crean** en I-040 (I-130). La línea sigue reservada en el presupuesto | Sin consumo hasta I-130 |
| Streams / KMS / capacidad provisionada | no presupuestados | **No se crean** (streams off, SSE AWS-managed, `PAY_PER_REQUEST`) | Nulo por diseño |
| Transferencia | gateway endpoint DynamoDB (I-010) | Sin NAT ni tráfico de Internet: el endpoint de gateway no tiene cargo | Nulo |

**Consecuencia:** I-040 no añade partidas nuevas; sólo materializa las ya presupuestadas y añade la amplificación
de `GSI1`. El presupuesto A **113.0613** / B **118.5613** USD/45 d frente a 120.0000 verificados **no se invalida
aritméticamente**, pero el consumo real sigue `NOT_VERIFIED` porque no hay AWS desplegada ni Cost Explorer
autorizado. No se declara `FINOPS_GARANTIZADO`.

## 6. Respaldo, recuperación y protección

| Obligación | Estado en I-040 | Responsable si falta |
|---|---|---|
| PITR | Habilitado (variable, `true` en h08i) | — |
| Copias programadas y retención | **No implementadas** | I-130 |
| Protección contra borrado | Variable; `false` en h08i para permitir el teardown del plan Free | Entornos persistentes deben activarla |
| Borrado real de cuenta | **No implementado** (orquestación auditable) | H08-E / I-130 |
| Restauración probada | `NOT_RUN` (requiere AWS) | Gate de despliegue |

## 7. CI aplicable

| Control | Estado |
|---|---|
| Terraform local (`fmt`, `init -backend=false`, `validate`, 131 pruebas) | **PASS real** |
| CI Go (`go build/vet/test`, gosec, govulncheck, SBOM) | **PASS real** en el PR |
| CI SDD (validador integral + SHA-256) | **PASS real** en el PR |
| **CI Terraform** | **NOT_IMPLEMENTED** — el workflow pertenece a **I-080**; no se crea aquí ni se declara verde |

I-080 es la spec dueña de esa obligación; I-040 no la suprime, la registra como dependencia y aporta la evidencia
local equivalente.

## 8. Conclusión

La tabla única, `GSI1` y el TTL aprobados **soportan** los patrones de las 75 operaciones sin `Scan`. `GSI2`
queda disponible pero **desactivado** hasta que exista el worker confirmado. Los riesgos abiertos son de
**verificación** (AWS, restauración, amplificación real de `GSI1`) y de **specs posteriores** (IAM, backups,
borrado de cuenta), no defectos de la implementación local.
