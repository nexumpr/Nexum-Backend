# DynamoDB — modelo lógico de accesos para los 75 contratos

**Diseño H06; implementación/medición NOT_RUN.** La autoridad para crear tablas/índices es H05 I-040 y la infraestructura Terraform H05 I-070. Estos son patrones obligatorios para OpenCode: no crear otros índices o tablas discrecionalmente ni cambiar la semántica por comodidad. Validar cardinalidad, tamaño y costes antes de `apply`.

## Modelo principal

Tabla propuesta `nexum-core-<entorno>`, clave compuesta `PK` String / `SK` String. `USER#<sub>` agrupa elementos por sujeto. Cada elemento lleva `entityType`, `createdAt`, `updatedAt`, `version` (número), `schemaVersion` y, donde corresponda, `expiresAt` TTL Unix seconds; **TTL no constituye eliminación instantánea**. Salvo catálogos, nunca leer otra partición con userId recibido del cliente: el `sub` procede de JWT válido y sesión no revocada. Proyecciones de listado y datos sensitivos se separan por tipo.

| Elementos (SK) | Acceso contractual | Fuente, patrón |
|---|---|---|
| `PROFILE`, `INCOME`, `PREFERENCES`, `ONBOARDING`, `NOTIFICATION_PREF` | GetItem + CAS `attribute_not_exists`/`version` | E012-013, E018-019, E024-025, E057-058, E068-069 |
| `CONSENT#<timestamp>#<id>` | Query PK + `begins_with(SK, ...)`, AppendOnly | E016-017 |
| `SESSION#<id>` y `SECURITY#<epoch>` | GetItem + CAS, revocación; token de refresh **solo hash** | E002-009, E014 |
| `MOVEMENT#<UTC ISO8601>#<id>` | Query PK `begins_with(SK, MOVEMENT#)`; orden SK por fecha | E027-031, E037 |
| `MOVEMENT_ID#<id>` (locator propio) | GetItem PK usuario + SK locator → GetItem SK ordenada | E029/030/031; guardar/revertir locator atómicamente con movimiento |
| `RECURRING#<id>` / `PAYMENT#<recurrence>#<period>` | GetItem, Query por prefijo; token de período único | E032-037 |
| `GOAL#<id>` / `CONTRIBUTION#<id>` | Query/GetItem; transacciones con versión/metas | E038-047 |
| `BUDGET#<YYYY-MM>#<category>` | Query por período + CAS/replace, inversión estable | E048-050 |
| `NOTIFICATION#<UTC>#<id>` / `NOTIFICATION_ID#<id>` | Query usuario, locator Get; read controlado | E061-063 |
| `DEVICE#<id>` / `REMINDER#<id>` | GetItem/Query; cifrado de tokens dispositivo | E059-060, E064-067 |
| `IDEM#<scope>#<keyHash>` | Put condicional / estado final guardado y TTL; hash de body | E001, E028, E033, E037, E039, E044, E047, E059, E065 |

## Identidad, correo y revocación

- Partición reservada `EMAIL#<sha256(normalized_email)>` / SK `OWNER` garantiza unicidad mediante `TransactWriteItems` con reserva y PROFILE. Nunca exponer hashes como identificadores públicos. Cambios de email requieren flujo verificado por separado.
- `REFRESH#<sha256(token)>` / SK `SESSION` permite localizar y rotar refresh opaco sin revelar token; escritura CAS de un solo uso; persistir familia `SESSION#` y `securityEpoch`. No depender solo de Redis para la revocación: DynamoDB autoritativo + Redis invalidación rápida en **ambos** nodos.
- Tokens de recuperación/verificación en particiones `ONE_USE#<digest>` con expiración lógica y CAS; los TTL físicos pueden demorarse. Sin secretos en traza ni ejemplos reales.

## Índices / patrón de consulta

- `GSI1` **solo si** es imprescindible para `MOVEMENT` por tipo/fecha: `GSI1PK=USER#<sub>#TYPE#<kind>`, `GSI1SK=<UTC>#<id>`. Write amplification y particiones evaluadas en H08 contra tráfico/FinOps. Alternativa permitida sin índice: Query PK + rango por SK y filtro en memoria del **resultado paginado**, nunca Scan global, y dejar claro que el filtro podría producir páginas vacías; preferir GSI si se exige eficiencia por tipo.
- `GSI2` para workers: `GSI2PK=DUE#<YYYY-MM-DD>#<shard>`, `GSI2SK=<dueAt>#<id>`; solo recordatorios activos con consentimiento; el worker no recorre tabla completa y aplica claim condicional para evitar doble envío.
- Catálogo: partición `PUBLIC#CATALOG` con SK `CATEGORY#<id>` / `FREQUENCY#<id>`, consultas acotadas; FAQs/consejos en `PUBLIC#CONTENT` y versionados.
- Si el conjunto excede los límites de tamaño o carga, se produce RFC con mediciones y Gate, no `Scan` ni índice improvisado.

## Operación atómica obligatoria

- Movimiento nuevo: `IDEM#` (si aplica) + `MOVEMENT` + `MOVEMENT_ID` + proyección versionada **en una transacción**, evitando dos escrituras sobre el mismo ítem en la misma transacción. Lecturas en consistencia fuerte cuando se verifica saldo.
- Recurrente pagado: `PAYMENT#` condicional único + `MOVEMENT` + locator + proyección; la programación no es gasto ejecutado. Reintento con la misma clave devuelve el mismo pago.
- Aportación dividida E044: idempotencia, CONTRIBUTION, movimientos propios, locators y versiones de metas (hasta 12 asignaciones distintas). **La suma de asignaciones = monto total en centavos**. Si la cantidad de acciones excede 100 o el tamaño transaccional >4 MB, responder error previo sin escrituras; no dividir silenciosamente la atomicidad.
- Reversión E047: CAS de estado `active→reversed`, movimientos de compensación, locators, y decrementar metas una sola vez; no borrar historial. Ingreso base no genera ingreso real; saldos y presupuestos solo gastos efectivos.
- `If-Match` / ETag: versión consistente por recurso; omisión HTTP 428, valor stale 412; PATCH sin cuerpo o campos desconocidos 400. Dynamo `ConditionalCheckFailed` traduce conflicto correctamente.

## Seguridad / operación / respaldo

KMS o cifrado gestionado según costo aprobado; mínimo IAM por PK/patrón de rol, cifrado de tokens push, red privada para Redis, backend nunca expone credenciales AWS. Dynamo PITR/backup sujeto al presupuesto y políticas H05; no ejecutar respaldos costosos por defecto. Streams, si se activan, deben estar declaradas en Terraform y sus cargos calculados. Comprobar restauración con ejercicios autorizados H08, no marcarla `PASS` en H06.
