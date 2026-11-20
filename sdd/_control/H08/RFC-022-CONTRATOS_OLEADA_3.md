# RFC-022 — Contratos y semántica de Oleada 3 (revisión 2)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/RFC-022-CONTRATOS_OLEADA_3.md` |
| Versión | **1.1.0** |
| Fecha | 2026-11-19 |
| Estado | **PROPUESTO** — no aprobado, no aplicado |
| Autoridad que lo motiva | Instrucción humana de inicio de Oleada 3 y revisión de la v1.0.0 |
| Instrumento de concurrencia | RFC-019 (E-1, E-2, E-3) |
| Aprobador | Responsable de Nexum-Backend |
| Frases | §14 (dos: RFC v1.1.0 y cambios transversales) |
| Sustituye | v1.0.0 (no aprobada). El registro de cambios está en §15 |

> Este documento **no** autoriza nada por sí mismo. Mientras siga `PROPUESTO`, el contrato
> vigente (OpenAPI + SDD aprobado) es la única autoridad y ningún cambio de este documento
> puede aplicarse. Las frases del §14 lo convierten en `APPROVED` y habilitan exclusivamente
> los cambios descritos.

## 1. Contexto y método

La Oleada 3 implementa 13 operaciones. El Gate `GATE_H08E_OLEADA_3.md` fue concedido el
2026-11-19 y habilita implementación, ramas y PR. **No** habilita los cambios contractuales
ni las semánticas públicas mientras esta RFC siga `PROPUESTO`.

Método: comparación `OpenAPI ↔ spec/plan/tasks/checklist ↔ RFC aplicables ↔ código actual`.
Se distingue explícitamente:

| Categoría | Significado | Autorización |
|---|---|---|
| Compatibilidad estructural | JSON/OpenAPI válido, sin campos obligatorios nuevos ni eliminaciones | Verificable con el comparador |
| Compatibilidad de comportamiento | Cambia lo que el cliente observa aunque el esquema no cambie | Esta RFC |
| Semántica financiera | Cambia un cálculo, un saldo o una agregación | Esta RFC + pruebas |
| Compatibilidad Android | El cliente puede necesitar cambios | Esta RFC + entrega |
| Diseño DynamoDB | Cambia el patrón físico de ítems | Esta RFC |
| Cambio transversal | Toca endpoints ya publicados (E024/E028/E030/E031/E033–E036) | Frase específica §14 |

El comparador `COMPATIBLE` **no** sustituye las decisiones de comportamiento, financieras,
DynamoDB o transversales. Cada una se registra en la matriz del §12.

## 2. E020 — Exportación de datos propios

### 2.1 Familias exportables

| Familia | Clave | DTO |
|---|---|---|
| Perfil | `PROFILE` | `UserProfile` |
| Ingreso habitual | `INCOME` | `Income` |
| Movimientos | `MOVEMENT#<UTC>#<id>` | `Movement` |
| Metas | `GOAL#<id>` | `Goal` |
| Pagos recurrentes | `RECURRING#<id>` | `RecurringPayment` |
| Aportaciones | `CONTRIBUTION#<UTC>#<id>` | `Contribution` |
| Presupuestos | `BUDGETSET#<YYYY-MM>` | `Budget` (cada elemento) |
| Consentimientos | `CONSENT#<versión>` | `Consent` |
| Onboarding | `ONBOARDING` | `Onboarding` |
| Preferencias | `PREFERENCES` | `UserPreferences` |

No se exportan hashes, contraseñas, tokens, `REFRESH#`, `IDEM#`, `EMAIL#`, `QUEUE#`,
`RESET#`, `ONE_USE#`, `DELETION#`, secretos, claves ni datos de otros titulares.

### 2.2 Cambio de contrato (aditivo)

Se añaden a `UserExport` las propiedades **opcionales**:

| Propiedad | Tipo | Nulo | Fuente |
|---|---|---|---|
| `income` | `Income` | sí | `INCOME` |
| `recurringPayments` | array de `RecurringPayment` | no (`[]`) | `RECURRING#` |
| `contributions` | array de `Contribution` | no (`[]`) | `CONTRIBUTION#` |
| `budgets` | array de `Budget` | no (`[]`) | `BUDGETSET#` |
| `onboarding` | `Onboarding` | sí | `ONBOARDING` |
| `preferences` | `UserPreferences` | sí | `PREFERENCES` |

`profile`, `exportedAt`, `movements`, `goals`, `consents` conservan tipo y obligatoriedad.
El `required` de `UserExport` **no** cambia. `additionalProperties: false` se mantiene: el
esquema se amplía explícitamente. `income` es un objeto, no un array: E001 V6.0.2 mantiene un
único ingreso habitual y E024 lo actualiza.

### 2.3 Paginación sin topes artificiales

**No existe un máximo de registros por familia ni total.** La exportación recorre todos los
registros exportables hasta agotarlos. El único límite es el de página del contrato:

- `limit`: 1..100, por defecto 25. Es el máximo de filas **de la página** (no de la
  exportación).
- Orden fijo de colecciones: `movements → contributions → recurringPayments → goals →
  budgets`. Dentro de cada colección, orden ascendente por clave.
- `profile`, `income`, `consents`, `onboarding`, `preferences` y `exportedAt` son conjuntos
  acotados y se incluyen completos en cada página.
- `nextCursor` ausente **solo** cuando todas las colecciones están agotadas. Un error
  recuperable devuelve `503` y **no** emite `nextCursor`: nunca se declara terminada una
  exportación incompleta.
- El cliente concatena los arrays en el orden de las páginas. No debe interpretar una página
  como un conjunto independiente.

### 2.4 Cursor

- Opaco, firmado con HMAC (clave derivada de un secreto existente, con separación de dominio
  `e020-export`), ligado al usuario y a la operación `E020_EXPORT`.
- Codifica: colección actual, `SK`/posición exacta de continuación y el instante
  `exportedAt` de la primera página. No codifica datos personales ni claves de DynamoDB en
  claro más allá de la `SK` opaca.
- Alteración, uso por otro usuario, firma inválida o antigüedad > 2 h → `400`.
- La `SK` de continuación se obtiene de `LastEvaluatedKey`/`ExclusiveStartKey`; no se usan
  offsets ni `Scan`.

### 2.5 Memoria y tamaño de respuesta

- El handler lee como máximo `limit` filas por página (≤100 DTOs) más los singletons
  acotados. No carga una colección completa en memoria.
- Cada colección se consulta con `Query`, `Limit` y `ExclusiveStartKey`; el `BUDGETSET` se
  lee por `GetItem`/`Query` y se pagina internamente sobre su lista de ≤50 elementos.
- La respuesta se serializa con un búfer acotado por `limit`; no se construyen respuestas de
  tamaño ilimitado.

### 2.6 Cambios concurrentes durante la exportación

Una exportación paginada sobre una tabla viva **no es una fotografía transaccional**. Esta
RFC lo declara y acota:

1. `exportedAt` se fija en la primera página y viaja en el cursor. Todas las páginas
   comparten la misma marca.
2. Cada página usa `ConsistentRead` para no mezclar réplicas.
3. Los elementos **creados después** de `exportedAt` se excluyen mediante el filtro
   `createdAt <= exportedAt` cuando el ítem lo persiste (movimientos, metas, aportaciones,
   recurrentes). Los presupuestos usan `updatedAt` del conjunto como cota.
4. Los elementos **borrados o cuyo `SK` cambia** después de `exportedAt` pueden no aparecer:
   DynamoDB no conserva una versión histórica legible sin PITR/S3. No se promete una
   fotografía inmutable; se documenta como limitación. La alternativa (PITR export a S3,
   bloqueo de mutaciones o almacenamiento temporal) crearía infraestructura o afectaría a
   otros endpoints y **no** se autoriza en esta oleada.
5. El cursor es monotónico por `SK`: un mismo ítem no se devuelve dos veces por el avance
   normal. Un cambio de `SK` (por ejemplo, editar `occurredAt`) puede hacer que el ítem se
   lea en su nueva posición si esta queda por delante del cursor, o que se omita si queda por
   detrás. No se duplica.
6. Para reducir omisiones, el export **no** filtra por estado mutable salvo lo exigido por el
   contrato (metas borradas, aportaciones revertidas): los ítems históricos se exportan con
   su estado vigente.
7. La entrega a Android debe indicar que la exportación es un recorrido ordenado y no una
   copia transaccional; si se requiere consistencia fuerte, se necesitará una decisión
   posterior con PITR/S3.

### 2.7 Errores recuperables

- Un fallo de lectura en cualquier página → `503` sin `nextCursor`; el cliente reintenta el
  mismo cursor.
- Un cursor caducado o inválido → `400`; el cliente reinicia la exportación.
- Un fallo de serialización de un ítem → `500` sin `nextCursor`; no se presenta un dato
  inventado.
- Nunca se declara `200` con `nextCursor` ausente si quedan colecciones por recorrer.

### 2.8 Privacidad

`Cache-Control: no-store`; DTO proyectados del contrato; sin logs con correo, importes,
tokens ni claves; sin atributos internos.

## 3. E048/E049 — Presupuestos: cambio de diseño físico y control de versión

### 3.1 Diferencia con el patrón vigente

El SDD aprobado y `internal/domain/dynamo/keys.go` documentan:

```
USER#<userId> / BUDGET#<YYYY-MM>#<categoryId>
```

La propuesta usa un **ítem agregado por periodo**:

```
USER#<userId> / BUDGETSET#<YYYY-MM>
```

`BUDGET#` queda **reservado y sin uso**; no se migra nada porque no existen presupuestos
persistidos. Esta RFC es el instrumento formal del cambio; `spec.md`/`plan.md` no se editan
sin su propio procedimiento. `BUDGETSET#` se añadirá a los helpers de claves y a la
documentación **solo después** de que esta RFC sea aprobada.

### 3.2 Justificación

- `E049` reemplaza el conjunto completo de un periodo. Con ítems por categoría, el peor caso
  (50 categorías retiradas + 50 nuevas) requiere 101 acciones, por encima del límite de 100
  de `TransactWriteItems`.
- El ítem agregado permite un **reemplazo atómico con una sola escritura condicionada**,
  sin borrados multi-ítem, sin estados intermedios y sin riesgo de dejar el periodo a medias.
- El tamaño máximo (50 categorías × ~100 B) está muy por debajo de 400 KB.
- No crea tablas, GSI ni recursos. Vive en la partición del titular y lo cubre la purga E014.

### 3.3 Contrato

- `PageOfBudget` se mantiene: `items[]` de `Budget` + `nextCursor`. Cada `Budget` conserva
  `categoryId`, `period`, `limitAmount`, `currency`, `version`.
- `BudgetReplace` se mantiene: `period`, `currency`, `budgets[]` (0..50, sin categorías
  duplicadas, importes exactos no negativos).
- El `version` de cada elemento es el `version` del conjunto del periodo. No hay versiones
  por categoría.

### 3.4 ETag de colección y versión inicial

- `E048?period=YYYY-MM` devuelve `ETag: "b<YYYY-MM>-<version>"` (validador fuerte).
- Un periodo sin conjunto tiene `version = 0` y `ETag: "bYYYY-MM-0"`.
- `E048` sin `period` devuelve todos los periodos y **no** emite `ETag`: no hay un conjunto
  único que validar. Para editar, el cliente consulta el periodo concreto.
- `E049` exige `If-Match` con la misma forma y el mismo `period` del cuerpo. Ausente → `428`.
  Formato inválido o débil → `400`/`412` según el helper existente. Periodo distinto o
  versión distinta → `412`.
- `E049` con `"b<period>-0"` crea el conjunto (versión 1) solo si no existe. Con
  `"b<period>-N"` reemplaza si la versión vigente es `N`.
- La respuesta `200` de `E049` devuelve el conjunto resultante y el nuevo `ETag`.

### 3.5 Paginación real de E048

- Con `period`: se lee el `BUDGETSET` (≤50 elementos) y se pagina con `limit`/`cursor` sobre
  la lista ordenada por `categoryId`. El cursor codifica `period|categoría` y está firmado.
  Aunque el ítem se lea completo, el tamaño está acotado por contrato (50); la respuesta
  nunca supera `limit` y el cliente recorre todas las páginas.
- Sin `period`: se pagina entre periodos con `Query` + `Limit` + `ExclusiveStartKey`; cada
  periodo aporta sus elementos. El cursor codifica `period|categoría`.
- `limit` 1..100, por defecto 25. Sin `Scan`.

### 3.6 Reemplazo completo y cliente

- El cuerpo de `E049` es el conjunto **completo** del periodo. Las categorías ausentes
  desaparecen (no quedan ítems huérfanos).
- El cliente debe obtener **todas** las páginas de `E048?period=...` antes de un reemplazo
  completo. Cada página del mismo periodo lleva el mismo `ETag`; si el `ETag` cambia entre
  páginas, el cliente reinicia la lectura. No se debe construir el reemplazo con una página
  parcial.
- La respuesta de `E049` es también paginable si el conjunto supera `limit`; el cliente debe
  recorrerla si necesita el conjunto completo.

### 3.7 Concurrencia y CAS

- La escritura es una `UpdateItem`/`PutItem` condicionada por
  `version = :expected` (o `attribute_not_exists` si `expected = 0`).
- Dos nodos concurrentes: solo uno gana; el otro recibe `412` sin modificar nada.
- Pruebas obligatorias: dos escrituras con el mismo ETag (una gana, una 412), creación con
  `"b<period>-0"`, intento de crear sobre un conjunto existente (412), periodo distinto
  (412), `If-Match` ausente (428), validador débil (400/412), reemplazo con 0 categorías,
  reemplazo con 50 categorías, lectura paginada 25/100.

### 3.8 E014 y aislamiento

- `BUDGETSET#` vive en `USER#<userId>`; la purga genérica de E014 lo elimina. Se añadirá una
  prueba de regresión con presupuestos creados y purgados.
- Ninguna consulta cruza `PK` de otro usuario.

## 4. E026 — Cálculo financiero: disponible estimado vs neto realizado

### 4.1 Comparación de fuentes

| Fuente | Qué define |
|---|---|
| **A-070 §3** | Disponible V6 = `incomeEquivalent − recurringFixedEstimate − oneTimeFixed − antExpenses − goalReservations`. El gasto fijo recurrente ya pagado registra movimiento pero **no se descuenta dos veces**; el variable recurrente no se descuenta como obligación hasta el pago. |
| **`CalculateAvailableV6`** | Implementa la fórmula con `MonthlyFinancialSummary`: ingresos equivalentes, estimación fija recurrente, fijos pagados, fijos puntuales, gastos variables y reservas de metas. `pendingFixed = max(0, estimate − paid)`; `totalFixed = paid + pending`; `available = incomeEq − totalFixed − oneTime − ant − reservations`. |
| **RFC-016 §17.7** | Cita `availableEstimated` como efecto observable de E042 (recalculado). No define su fórmula ni lo declara campo de `FinancialSummary`. |
| **OpenAPI `FinancialSummary`** | `period`, `currency`, `income`, `expenses`, `savings`, `net`, `asOf`. `net` se describe como «ingreso efectivamente registrado menos egresos y ahorro transferido, sin contar ingreso base estimado; no doble gasto». |
| **Casos de uso actuales** | No hay ningún endpoint que consuma `CalculateAvailableV6`; E026 no existe. `DashboardCachedService` usa `UserDashboardData` (availableCents, totalSavedCents, activeGoals), una estructura histórica distinta del contrato. |

### 4.2 Distinción de conceptos

| Concepto | Definición | Fuente | ¿Campo de `FinancialSummary`? |
|---|---|---|---|
| `income` | Ingreso **realizado** por movimientos `type=income` del periodo | OpenAPI/E026 | Sí, ya existe |
| `expenses` | Egreso **realizado** por movimientos `type=expense` | OpenAPI/E026 | Sí, ya existe |
| `savings` | Ahorro **reservado** en aportaciones activas del periodo | OpenAPI/E026 | Sí, ya existe |
| `net` | `income − expenses − savings` (realizado) | OpenAPI | Sí, ya existe |
| `availableEstimated` | Disponible **proyectado** A-070: ingresos equivalentes − obligación fija recurrente pendiente − egresos realizados − reservas | A-070/`CalculateAvailableV6` | **No existe** |

`net` **no** sustituye a `availableEstimated`: el primero es realizado y excluye el ingreso
base; el segundo es proyectado e incluye el ingreso equivalente y las obligaciones
recurrentes aún no pagadas. Declararlos equivalentes sería un error financiero.

### 4.3 Resolución mínima propuesta

**Añadir `availableEstimated` como propiedad opcional de `FinancialSummary`** (string
decimal, admite signo, mismo patrón que `net`). Es un cambio de contrato aditivo, justificado
porque:

1. A-070 y `CalculateAvailableV6` ya definen el cálculo y el proyecto lo considera un
   indicador V6.
2. Sin el campo, Android tendría que replicar una fórmula con ingreso equivalente,
   recurrencias y reservas, con riesgo de doble descuento.
3. `net` no lo representa y no se puede inferir de los campos actuales.

Si el responsable prefiere no ampliar el contrato, la alternativa es **no emitir**
`availableEstimated` y documentar que E026 solo publica los campos actuales; el cálculo
A-070 queda interno y sin consumidor. Esta RFC recomienda la ampliación, pero la decisión
final es humana.

### 4.4 Fórmula exacta

Con `int64` centavos, aritmética racional de `CalculateMonthlyEquivalent` (A-070 §2.2):

```
incomeEq          = monthlyEquivalent(INCOME habitual)
                  + Σ monthlyEquivalent(reglas RECURRING activas type=income)
recurringFixed    = Σ monthlyEquivalent(reglas RECURRING activas type=expense)
paidRecurring     = Σ movimientos type=expense del periodo con recurringPaymentId != null
pendingFixed      = max(0, recurringFixed − paidRecurring)
expenses          = Σ movimientos type=expense del periodo
goalReservations  = Σ aportaciones status=active con occurredAt en el periodo
availableEstimated = incomeEq − pendingFixed − expenses − goalReservations
```

Propiedades:

- **Anti-doble descuento:** `paidRecurring` ya está dentro de `expenses`; solo se añade
  `pendingFixed` (la parte no pagada). Pagar una obligación no cambia el disponible: cambia
  `pendingFixed` por `expenses` en la misma cuantía.
- **Importe real distinto de la regla:** si `paidRecurring > recurringFixed`, `pendingFixed=0`
  y el exceso queda en `expenses`. Si es menor, la diferencia queda en `pendingFixed`.
- **Reservas:** las aportaciones son `saving`, no `expense`; se restan una sola vez como
  `goalReservations` y no se cuentan en `expenses`.
- **Ingreso base:** `income` sigue siendo realizado; `incomeEq` solo se usa en
  `availableEstimated`. No se duplica el ingreso habitual en `income`.
- **Periodo:** zona IANA de preferencias; por defecto `America/Mexico_City`. Límites locales
  convertidos a UTC para consultar `MOVEMENT#`/`CONTRIBUTION#`.

### 4.5 Pruebas obligatorias

1. Ingreso habitual 5 000, recurrente fijo 1 000 **no pagado**, otros gastos 200, aportación
   500 → `availableEstimated = 3 300`; `net = −700` (sin ingreso realizado).
2. El mismo caso con el recurrente **pagado** (movimiento 1 000) → `availableEstimated =
   3 300`; `expenses = 1 200`; demuestra que pagar no descuenta dos veces.
3. Pago real 950 (regla 1 000) → `pendingFixed = 50`, `expenses` incluye 950, disponible
   consistente.
4. Recurrente `income` activo → suma a `incomeEq`, no a `income`.
5. Aportación revertida → no cuenta en `goalReservations`.
6. Sin datos → todos los importes `0.00`, `availableEstimated = 0.00`.
7. Periodo con cambio de mes y zona IANA.

## 5. E037 — Pago de una regla recurrente: semántica precisa

### 5.1 Ocurrencia

Una **ocurrencia de pago** es el registro manual de un pago real de una regla recurrente,
identificado por `recurringPaymentId` y `paidAt`. E037 **no** es un planificador: no genera
ocurrencias futuras, no avanza `nextDueDate` y no crea pagos automáticos.

### 5.2 Fecha y zona horaria

- `paidAt` es el instante del pago (RFC 3339). Se normaliza a UTC con milisegundos.
- La **fecha de control de duplicado** es el día civil en la zona IANA del titular
  (`PREFERENCES.timezone`; por defecto `America/Mexico_City`).
- Dos pagos de la misma regla en el mismo día civil local se consideran la misma ocurrencia:
  el segundo devuelve `409` aunque use otra `Idempotency-Key`. Un pago legítimo adicional el
  mismo día (pago dividido) no está soportado por el contrato actual; requeriría una RFC que
  añada un identificador de ocurrencia o permita múltiples pagos por fecha. Se declara la
  limitación.

### 5.3 Relación con la frecuencia

E037 **no** exige que `paidAt` coincida con `nextDueDate` ni con la cadencia. Un pago puede
ser anticipado o tardío. La frecuencia gobierna la programación que muestra Android; E037
solo exige que la regla exista, sea del titular y esté activa. No se modifica la regla.

### 5.4 Reintentos y replay

- Misma `Idempotency-Key` + mismo payload normalizado → replay del `200` original con el
  mismo `Movement` (cabecera `X-Idempotent-Replay: true`).
- Misma clave + payload distinto → `409`.
- Clave nueva en el mismo día local → `409` por duplicado; el cliente debe localizar el
  movimiento con E027/E029.
- Si la respuesta se pierde tras confirmar la transacción, el reintento con la misma clave
  recupera el resultado. Nunca se crea un segundo movimiento.

### 5.5 Transacción y concurrencia

`TransactWriteItems` con:

1. movimiento `MOVEMENT#<paidAt>#<id>`;
2. puntero `MOVEMENTPTR#<id>`;
3. localizador `RECURRINGPAY#<ruleID>#<YYYY-MM-DD>` con condición `attribute_not_exists`;
4. registro de idempotencia `USER#<id>/IDEM#E037_PAY_RECURRING#<hash>`;
5. `ConditionCheck` de `RECURRING#<ruleID>` con `active=true` (o ausencia de `active` en
   ítems heredados) y misma `PK`.

Dos EC2 concurrentes: solo una transacción confirma. La otra recibe replay (misma clave) o
`409` (localizador existente). No hay doble gasto. Si falla la condición de la regla →
`404`/`409`; si falla el movimiento por colisión de ID → reintento acotado.

### 5.6 Importe real

El `amount` del cuerpo es el importe **efectivamente pagado** y puede diferir del importe
declarado en la regla. Debe ser positivo, exacto en centavos y `currency=MXN`. La regla no se
modifica.

### 5.7 Movimiento generado y categoría `recurring`

| Campo | Valor |
|---|---|
| `type` | Tipo de la regla: `income`, `expense` o `saving` |
| `amount` | Importe del cuerpo |
| `currency` | `MXN` |
| `categoryId` | `recurring` (constante interna) |
| `description` | `name` de la regla (≤120, ya validado) |
| `occurredAt` | `paidAt` normalizado |
| `recurringPaymentId` | ID de la regla |
| `contributionId` | `null` |

La regla **no tiene `categoryId`**; no se puede atribuir el gasto a una categoría original
que no existe. `recurring` es una categoría interna estable, visible en E027/E029 y en la
analítica por categoría. Impacto:

- **Presupuestos (E050):** un gasto recurrente aparece bajo `recurring`; si el usuario crea
  un presupuesto para `recurring`, se computa; si no, no aparece en E050 pero sí en E026 y
  E027. No se presenta como gasto de otra categoría.
- **Analítica Android:** puede agrupar por `recurringPaymentId` y mostrar el nombre de la
  regla; la categoría `recurring` es explícita.
- **Futuro:** añadir `categoryId` a las reglas recurrentes requeriría RFC y cambios en
  `RecurringCreate`/`RecurringPayment`/`RecurringPaymentCreate`; no se hace en esta oleada.

No se inventan propiedades públicas.

## 6. E042 — Eliminación de meta: alternativas y cambio de comportamiento

### 6.1 Alternativa A — borrado con reversión automática

- E042 revierte automáticamente las aportaciones activas que referencian la meta y marca la
  meta como borrada.
- **Problema:** una aportación puede repartirse entre varias metas (hasta 12). El contrato
  `Contribution` tiene un único `status` para toda la aportación y una lista de allocations.
  Revertir la aportación completa afectaría a metas no implicadas; revertir solo la
  allocation de la meta borrada no es representable sin añadir un estado por allocation
  (cambio de contrato).
- Si se restringe a aportaciones que solo asignan a esa meta, el comportamiento es seguro,
  pero impredecible para Android y difícil de explicar.
- Ventaja: una sola llamada; cumple la descripción literal de RFC-016 §17.7.
- Riesgo: modificación silenciosa de saldos de otras metas; auditoría compleja; posibles
  hotfixes.

### 6.2 Alternativa B — 409 y reversión explícita (recomendada)

- E042 comprueba `activeContributions` de la meta. Si es > 0 → `409` y no modifica nada.
- El cliente revierte las aportaciones con E047 (operación explícita, con `reason`), que
  revierte la aportación completa y todas sus allocations de forma atómica.
- Cuando `activeContributions = 0`, E042 marca la meta con `deleted=true`, `status=archived`,
  `savedAmount=openingSaved`, `version+1` y responde `204` sin cuerpo.
- Segundo `DELETE` sobre una meta ya borrada → `204` (idempotente). Meta inexistente → `404`.
- E040/E041 tratan `deleted=true` como inexistente; E038/E051 la excluyen.
- **Ventaja:** no modifica aportaciones compartidas sin una operación explícita; el usuario
  ve qué se revierte; integridad financiera y auditoría claras; el contrato ya declara 409.
- **Coste Android:** debe manejar `409`, mostrar las aportaciones activas y guiar la
  reversión; después reintentar E042. Es un cambio de flujo, no de esquema.
- **Cambio frente a RFC-016 §17.7:** la tabla describe `status: reversed` tras el borrado;
  con B, la reversión ocurre **antes** y E042 solo borra cuando no quedan activas. Se
  registra como enmienda explícita de comportamiento, no como detalle interno.

### 6.3 Decisión

Se recomienda **B**. La aprobación definitiva queda sujeta a esta RFC. Si el responsable
elige A, se debe añadir un estado por allocation al contrato de `Contribution` o restringir
A a aportaciones no compartidas, con su propio Gate.

## 7. E043–E047 — Aportaciones y reversión: historial y cambios transversales

### 7.1 Claves y transacciones

```
USER#<userId> / CONTRIBUTION#<occurredAt>#<contributionId>
USER#<userId> / CONTRIBUTIONPTR#<contributionId>
```

- `CONTRIBUTION#` ordena por fecha para E045; el puntero resuelve E046 O(1). Ambos en la
  partición del titular; E014 los purga.
- E043 no escribe. E044 escribe aportación + puntero + **un** movimiento `saving` por el
  total + puntero del movimiento + N metas (`savedAmountCents += allocation`,
  `activeContributions+1`, `version+1`) + idempotencia. Máximo 18 acciones.
- Condiciones por meta: existe, `status=active`, no borrada,
  `savedAmountCents + allocation ≤ targetAmountCents`.
- E047 revierte la aportación completa (`status=reversed`, `reversedAt`, `reverseReason`,
  `version+1`), resta cada allocation de su meta (`savedAmountCents -= allocation`,
  `activeContributions-1`, `version+1`), con condiciones `savedAmountCents >= allocation` y
  `activeContributions >= 1`, y escribe la idempotencia. No borra el movimiento.

### 7.2 `remaining` y `canCommit` (E043)

- `remaining = max(0, amount − Σ allocations) + Σ max(0, allocation − capacidadRestante)`.
- `canCommit = remaining == 0` y todas las metas existen, son del titular y están activas.
- Errores estructurales → `400/422`; meta inexistente/ajena → `404`; capacidad insuficiente
  → `200` con `canCommit=false`. Es el propósito del preview.

### 7.3 Relación `Contribution → Movement → Goals`

| Endpoint | Comportamiento |
|---|---|
| **E027** (`GET /movements`) | El movimiento `saving` de la aportación aparece como un movimiento normal. No se añade un campo de estado (no existe en el contrato). La aportación revertida no borra el movimiento; el cliente puede consultar E045/E046 para el estado. |
| **E029** (`GET /movements/{id}`) | Devuelve el movimiento `saving` con `contributionId`; no expone el estado de la aportación. |
| **E030** (`PATCH /movements/{id}`) | **Cambio transversal T-1:** si `contributionId` no está vacío, rechaza con `409` (`CONTRIBUTION_LINKED`) cualquier modificación de `categoryId`, `description` u `occurredAt`. Sin este guardo, un cambio de importe/fecha/categoría dejaría la aportación y las metas inconsistentes. Los movimientos sin vínculo no cambian. |
| **E031** (`DELETE /movements/{id}`) | Ya rechaza con `409` los movimientos con `contributionId` (implementado en Oleada 2). Se añade prueba de regresión; no cambia el código salvo que la prueba revele un defecto. |
| **E026** | `savings` se calcula desde aportaciones `status=active`; el movimiento `saving` no se suma para no duplicar. Revertida → deja de contar. |
| **E051** | Igual que E026; `latestMovements` puede mostrar el movimiento histórico, pero `summary.savings` usa aportaciones activas. |
| **E014** | Purga genérica de la partición; no requiere cambio. Se añade prueba con aportaciones y movimientos. |

El movimiento histórico permanece visible aunque la aportación esté revertida. Es una
limitación del contrato actual (no hay campo `status` en `Movement`); se documenta en la
entrega Android. No se inventa un campo público.

### 7.4 Comportamiento y pruebas

- E047 con `Idempotency-Key` repetida y mismo payload → `200` idéntico; con otra clave sobre
  una aportación ya revertida → `409`.
- Doble reversión concurrente: solo una transacción gana; la otra `409`, sin restar dos veces.
- Meta compartida: E044 actualiza todas las metas de la aportación; E047 revierte todas; E042
  de una meta concreta devuelve `409` hasta que la aportación se revierta (alternativa B).
- Cantidades exactas: sumas y restas en centavos; sin `float64`.
- E030 sobre movimiento vinculado → `409`; sobre movimiento no vinculado → comportamiento
  actual.

## 8. E050/E051 — Agregados, caché y Redis

### 8.1 E050

- Lee el `BUDGETSET` del periodo (≤50).
- `usedAmount` = suma de movimientos `type=expense` del periodo por `categoryId`.
- Excluye reglas recurrentes no ejecutadas; los pagos de E037 se cuentan una sola vez porque
  son movimientos.
- Las aportaciones (`saving`) no son gasto.
- Periodo por zona IANA del titular; por defecto `America/Mexico_City`.

### 8.2 Redis: ACL efectiva

El ACL del usuario de aplicación (Terraform `modules/redis`) es:

```
on ~nexum:* -@all +@read +@write +@keyspace +@scripting +@transaction +@connection
-keys -scan -randomkey -restore -migrate -sort
```

- `INCR` pertenece a `@write`; `GET`/`SET` a `@read`/`@write`; `EXPIRE`/`TTL` a `@keyspace`.
- La clave `nexum:v1:<env>:<userHash>:...` cumple `~nexum:*`.
- `SCAN` sigue denegado y **no se usa**.
- Antes de E051 se verificará el `access_string` efectivo del state Terraform. **No se
  amplía ningún permiso Redis.** Si `INCR` no estuviera permitido, E051 funcionaría sin caché
  (cálculo directo) y se declararía `CACHE_DISABLED_BY_ACL`; no se modifica el ACL.

### 8.3 Caché de E051

- Clave: `nexum:v1:<env>:<userHash>:dashboard:<period>:v<epoch>`.
- `epoch` = contador por usuario en Redis (`INCR`), TTL del contador 7 días.
- TTL de la entrada: 30 s.
- Lectura: si `GET` devuelve una entrada y el epoch coincide, se sirve; si el epoch no se
  puede leer, se calcula desde DynamoDB y **no** se escribe caché (fail-open de caché).
- Invalidez: `INCR` del epoch tras una mutación financiera. Las claves antiguas expiran por
  TTL; no se usa `SCAN`.
- Redis nunca es autoridad de datos financieros: si falla, se calcula desde DynamoDB; una
  mutación no se pierde ni se altera por un fallo de caché.
- Nunca se sirve una entrada si el epoch no coincide (no se sirve caché obsoleta como actual).

### 8.4 Hooks de epoch (cambio transversal T-2)

Para que la invalidación sea efectiva, se añade una llamada **no bloqueante** después de una
mutación exitosa en:

| Endpoint | Motivo |
|---|---|
| E024 (actualizar ingreso) | `availableEstimated` |
| E028/E030/E031 (movimientos) | `expenses`, `income`, `latestMovements` |
| E033–E036 (reglas recurrentes) | `availableEstimated` (estimaciones) |
| E037 (pago recurrente) | `expenses`, `paidRecurring` |
| E042 (borrar meta) | `goals` |
| E044/E047 (aportación/reversión) | `savings`, `goals` |
| E049 (presupuestos) | `budget usage`/dashboard si aplica |

La llamada no cambia la respuesta del endpoint; si Redis falla, se registra y se continúa. Es
un cambio transversal delimitado, con frase propia (§14). No se toca E027/E029/E032/E038–E041
salvo lo indicado.

## 9. Cambios materiales en OpenAPI

| Archivo | Cambio | Clasificación | Frase |
|---|---|---|---|
| `nexum-api-v1.json` / `.yaml` | `UserExport`: +`income`, `recurringPayments`, `contributions`, `budgets`, `onboarding`, `preferences` (opcionales) | Aditivo estructural | RFC v1.1.0 |
| `nexum-api-v1.json` / `.yaml` | `nexum_e048` 200: cabecera `ETag` | Aditivo estructural | RFC v1.1.0 |
| `nexum-api-v1.json` / `.yaml` | `FinancialSummary`: +`availableEstimated` (opcional, string con signo) | Aditivo estructural + semántica nueva | RFC v1.1.0 |
| Resto | Sin cambios de `required`, enum, códigos, rutas ni parámetros | — | — |

No se modifica ningún `required`, enum existente, código de error ni ruta. La paridad
YAML/JSON y `validar_h06.py` deben seguir en PASS. El comparador debe devolver `COMPATIBLE`;
además se revisan comportamiento, semántica financiera y Android (§11).

## 10. Cambios transversales delimitados

| ID | Archivo(s) | Cambio | Autorización |
|---|---|---|---|
| **T-1** | `internal/application/movements/service.go` (o handler E030) + pruebas | E030 rechaza con `409` cualquier parche sobre un movimiento con `contributionId` no vacío | Frase transversal §14 |
| **T-2** | `cmd/server/e024.go`, `e028.go`, `e030.go`, `e031.go`, `e033.go`–`e036.go`, y hooks de Oleada 3 | `IncrementUserEpoch` no bloqueante tras mutación exitosa | Frase transversal §14 |

No se toca E027/E029/E031/E032/E038–E041 salvo el guardo T-1 y la prueba de regresión de
E031. Cualquier otro cambio requiere su propio Gate.

## 11. Matriz de compatibilidad

| Dimensión | E020 | E026 | E037 | E042 | E043–E047 | E048–E050 | E051 |
|---|---|---|---|---|---|---|---|
| Estructural JSON/OpenAPI | Aditivo | Aditivo (`availableEstimated`) | Sin cambio | Sin cambio | Sin cambio | Aditivo (ETag) | Sin cambio |
| Comportamiento | Nuevas familias y paginación | Nuevo campo calculado | Pago real y duplicado por fecha | 409 si aportaciones activas | Reversión sin borrar movimiento | Conjunto único y ETag | Caché por epoch |
| Semántica financiera | Exporta datos | available vs net | Gasto real, categoría `recurring` | No revierte en silencio | Ahorro activo vs revertido | Límites por periodo | Agregados consistentes |
| Android | Debe concatenar páginas | Puede mostrar disponible | Debe manejar 409 y `recurring` | Debe revertir antes de borrar | Debe manejar 409/replay | Debe obtener ETag y todas las páginas | Sin cambio de esquema |
| DynamoDB | Sin cambio | Sin cambio | Nuevo localizador `RECURRINGPAY#` | `deleted`/`activeContributions` | `CONTRIBUTION#` + puntero | `BUDGETSET#` (sustituye a `BUDGET#`) | Sin cambio |
| Transversal | — | — | — | — | T-1 E030 | T-2 hooks | T-2 hooks |

## 12. Matriz de decisiones

| E-ID | PROBLEMA ORIGINAL | DECISIÓN PROPUESTA | CAMBIO PÚBLICO | CAMBIO INTERNO | RFC/GATE | IMPACTO ANDROID | RIESGO RESIDUAL |
|---|---|---|---|---|---|---|---|
| E020 | `UserExport` no cubre familias reales | +6 propiedades opcionales; paginación sin topes; cursor firmado; `exportedAt` estable; `createdAt <= exportedAt` | Sí, aditivo | Proyección DTO, cursor, sin infra nueva | RFC v1.1.0 | Concatenar páginas; no es snapshot transaccional | Omisiones por borrados/cambios de SK concurrentes; documentado |
| E026 | `availableEstimated` no existe y no equivale a `net` | +`availableEstimated` opcional con fórmula A-070; `net` realizado se conserva | Sí, aditivo | Cálculo con `CalculateMonthlyEquivalent`, consultas por periodo | RFC v1.1.0 | Nuevo campo; `net` no cambia | Fórmula nueva; pruebas de no doble descuento |
| E037 | Pago recurrente sin semántica de ocurrencia | Pago real, duplicado por fecha local, replay, transacción, `categoryId=recurring` | No (esquema igual) | Nuevo localizador, movimiento derivado | RFC v1.1.0 + Gate | Categoría `recurring`; 409 por duplicado | Pagos divididos no soportados; documentado |
| E042 | RFC-016 sugiere reversión automática | Alternativa B: 409 si aportaciones activas; E047 previo; tombstone | No (códigos ya existen) | `deleted`, `activeContributions`, tombstone | RFC v1.1.0 + Gate | Debe revertir antes de borrar | Cambio de flujo; documentado |
| E043 | `remaining` ambiguo | `remaining` = no comprometible; `canCommit` | No | Cálculo puro | RFC v1.1.0 | Interpretar `remaining`/`canCommit` | Ambigüedad resuelta por RFC |
| E044 | Movimiento vs aportación | Un movimiento `saving` por total, N metas atómicas | No | `CONTRIBUTION#` + puntero + movimiento | RFC v1.1.0 + Gate | Sin cambio de esquema | Límite 12 metas; 409/422 |
| E045/E046 | Sin claves definidas | `CONTRIBUTION#<fecha>#<id>` + puntero; filtros antes de paginar | No | Nuevas claves | RFC v1.1.0 | Sin cambio | Paginación con filtros; probar |
| E047 | Reversión y historial | `status=reversed`, metas decrementadas, movimiento conservado | No | CAS + idempotencia | RFC v1.1.0 + Gate | Debe manejar 409/replay | Movimiento histórico sin estado visible |
| E048/E049 | `BUDGET#` vs `BUDGETSET#`; ETag ausente | `BUDGETSET#`, ETag `"b<period>-<v>"`, CAS, paginación real | Sí (ETag aditivo) | Cambio de diseño físico | RFC v1.1.0 + Gate | Obtener ETag y todas las páginas | Cliente que no pagine pierde categorías al reemplazar |
| E050 | Uso presupuestal | Desde movimientos `expense`, sin futuros | No | Lee `BUDGETSET#` | RFC v1.1.0 + Gate | Sin cambio | Depende de E037/E028 |
| E051 | Dashboard y caché | Summary + metas activas + 10 movimientos; epoch/TTL | No | Caché Redis por epoch | RFC v1.1.0 + T-2 | Sin cambio de esquema | Redis caído → cálculo directo |
| T-1 | E030 puede romper aportación | Rechazo 409 si `contributionId` | No | Guardo en E030 | Frase transversal | Debe manejar 409 | Cambio de comportamiento en E030 |
| T-2 | Caché obsoleta | `INCR` epoch tras mutaciones | No | Hooks no bloqueantes | Frase transversal | Sin cambio | Si Redis falla, se calcula directo |

## 13. Riesgos

| ID | Riesgo | Mitigación |
|---|---|---|
| R-22-01 | `UserExport` ampliado rompe un cliente con validación estricta | Aditivo; el contrato es la autoridad; documentar en Android |
| R-22-02 | Exportación no es snapshot | Documentado; `exportedAt` + `createdAt`; sin infra nueva |
| R-22-03 | `availableEstimated` nuevo campo | Opcional, fórmula y pruebas; decisión humana |
| R-22-04 | E042 409 cambia el flujo | Alternativa B justificada; Android debe revertir antes |
| R-22-05 | E030 guardo transversal | Frase específica; prueba de regresión |
| R-22-06 | `BUDGETSET#` cambia el diseño físico | RFC + helpers tras aprobación; E014 e aislamiento probados |
| R-22-07 | Caché obsoleta | Epoch + TTL + cálculo directo; `SCAN` prohibido |
| R-22-08 | Pagos divididos no soportados | Limitación declarada; futura RFC |

## 14. Frases de aprobación

**Frase A — RFC-022 v1.1.0** (contratos y semánticas):

```text
APRUEBO_RFC_022_CONTRATOS_OLEADA_3_V1_1_0
```

**Frase B — cambios transversales delimitados** (T-1 E030 y T-2 hooks de epoch):

```text
APRUEBO_CAMBIOS_TRANSVERSALES_OLEADA_3
```

La frase A habilita los cambios del §9 y las semánticas §2–§8. La frase B habilita solo el
§10. Ninguna autoriza merges, publicación S3, `terraform apply`, `instance refresh` ni
despliegue; cada uno conserva su Gate propio. La v1.0.0 no fue aprobada y no se aplica.

## 15. Registro

| Fecha | Versión | Cambio | Estado |
|---|---|---|---|
| 2026-11-19 | 1.0.0 | Reconciliación inicial | **NO APROBADA** |
| 2026-11-19 | 1.1.0 | E020 sin topes y con concurrencia; E048/E049 con diseño físico y paginación; E026 con comparación A-070/RFC-016/`CalculateAvailableV6` y `availableEstimated`; E037 con ocurrencia/zona/categoría; E042 con alternativas A/B; E043–E047 con historial y T-1; E050/E051 con ACL y epoch; matriz de decisiones | **PROPUESTO** |
