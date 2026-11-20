# RFC-022 — Contratos y semántica de Oleada 3 (revisión 3)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/RFC-022-CONTRATOS_OLEADA_3.md` |
| Versión | **1.2.1** |
| Fecha | 2026-11-19 |
| Estado | **APPROVED** — aprobado por el responsable el 2026-11-19 |
| Autoridad que lo motiva | Instrucción humana de inicio de Oleada 3 y auditoría correctiva de la v1.1.0 |
| Instrumento de concurrencia | RFC-019 (E-1, E-2, E-3) |
| Aprobador | Responsable de Nexum-Backend |
| Frases | §14 (dos); frase de aprobación concedida: `APRUEBO_RFC_022_CONTRATOS_OLEADA_3_V1_2_1` |
| Sustituye | v1.1.0 y v1.0.0, no aprobadas. Registro en §15 |

> Esta versión está **APPROVED** desde el 2026-11-19. Habilita exclusivamente los cambios
> descritos en los §2–§10; no autoriza merges, S3, `terraform apply`, `instance refresh` ni
> despliegue, que conservan sus Gates propios.

## 1. Contexto y cambios de esta revisión

Correcciones obligatorias de la auditoría de la v1.1.0:

1. **E049** no pagina la respuesta: devuelve el conjunto completo del periodo (≤50), sin
   `nextCursor` ni parámetros `limit`/`cursor`. E048 conserva la paginación.
2. **E026** no incorpora `availableEstimated`: no existe una fuente determinista que
   clasifique cada recurrente como fijo o variable. Se adopta la alternativa B.
3. **E051** calcula directamente desde DynamoDB, sin caché de dashboard. Se elimina T-2.
4. **E020** no promete ausencia de duplicados ni snapshot transaccional: define garantías
   reales y limitaciones.
5. **E037** no usa la categoría inexistente `recurring`: mapea por tipo a categorías del
   catálogo real.
6. **E042** mantiene la alternativa B (409 + E047), con enmienda explícita de RFC-016 §17.7.
7. **T-1** se limita a E030: 409 cuando el movimiento está vinculado a una aportación.
   E031/E027/E029 no cambian.
8. **E026 — ahorros realizados:** `savings` suma los movimientos `type=saving` **independientes**
   (`contributionId` nulo) del periodo y las aportaciones `status=active`; el movimiento de una
   aportación no se suma por separado. Una aportación revertida no cuenta como ahorro activo,
   aunque su movimiento histórico se conserve.
9. **E043 — suma exacta:** `Σ allocations == amount` en centavos, con las mismas reglas
   estructurales que E044. E043 no admite un reparto incompleto; la capacidad insuficiente se
   expresa con `canCommit=false` y `remaining` calculado, sin escribir.

El comparador `COMPATIBLE` no sustituye las decisiones de comportamiento, financieras,
DynamoDB o transversales. Cada una se registra en la matriz del §12.

## 2. E020 — Exportación de datos propios

### 2.1 Familias y contrato

`UserExport` añade propiedades **opcionales**:

| Propiedad | Tipo | Nulo | Fuente |
|---|---|---|---|
| `income` | `Income` | sí | `INCOME` |
| `recurringPayments` | array de `RecurringPayment` | no (`[]`) | `RECURRING#` |
| `contributions` | array de `Contribution` | no (`[]`) | `CONTRIBUTION#` |
| `budgets` | array de `Budget` | no (`[]`) | `BUDGETSET#` |
| `onboarding` | `Onboarding` | sí | `ONBOARDING` |
| `preferences` | `UserPreferences` | sí | `PREFERENCES` |

`profile`, `exportedAt`, `movements`, `goals` y `consents` conservan tipo y obligatoriedad.
El `required` no cambia. No se exportan hashes, contraseñas, tokens, `REFRESH#`, `IDEM#`,
`EMAIL#`, `QUEUE#`, `RESET#`, `ONE_USE#`, `DELETION#`, secretos ni datos de otros titulares.

### 2.2 Paginación

- Sin topes artificiales por familia ni totales. La exportación recorre todas las colecciones
  hasta agotarlas.
- `limit` 1..100 (por defecto 25) es el máximo de filas **de la página**, no de la
  exportación.
- Orden fijo: `movements → contributions → recurringPayments → goals → budgets`; dentro de
  cada colección, ascendente por SK. `profile`, `income`, `consents`, `onboarding`,
  `preferences` y `exportedAt` se incluyen completos en cada página.
- `nextCursor` ausente **solo** cuando todas las colecciones están agotadas. Un error
  recuperable devuelve `503` **sin** `nextCursor`: nunca se declara terminada una exportación
  incompleta.
- Cursor opaco firmado con HMAC, ligado al usuario y a `E020_EXPORT`; codifica colección, SK
  de continuación y `exportedAt`. Alteración, uso por otro usuario o antigüedad > 2 h → 400.

### 2.3 Garantías reales bajo concurrencia

Una exportación paginada sobre una tabla viva **no es un snapshot transaccional**. Esta RFC
declara las garantías y limitaciones exactas:

1. `exportedAt` se fija en la primera página y viaja en el cursor. Todas las páginas comparten
   la misma marca, pero no una fotografía de los datos.
2. `ConsistentRead` evita leer réplicas retrasadas **dentro de cada página**; no convierte
   varias páginas en una transacción ni en una lectura consistente entre páginas.
3. `createdAt <= exportedAt` solo excluye elementos **nuevos** creados después de iniciar la
   exportación cuando el ítem conserva ese atributo. **No** reconstruye la versión anterior de
   un elemento modificado ni recupera un elemento borrado.
4. El cursor avanza por SK. Un elemento cuyo SK no cambia y no se borra se devuelve como
   máximo una vez en el recorrido normal. Un cambio de SK (por ejemplo, editar `occurredAt`)
   puede provocar una omisión o una lectura en la nueva posición; **no se promete ausencia
   absoluta de duplicados ni de omisiones bajo mutaciones concurrentes**.
5. Los elementos borrados antes de ser leídos no aparecen. Los elementos modificados
   aparecen con el valor vigente en el momento de su lectura.
6. No se implementan PITR, exportación a S3, bloqueos de cuenta ni almacenamiento temporal.
   Esas alternativas crearían infraestructura o afectarían a otros endpoints y no se autorizan
   en esta oleada.
7. Android debe tratar la exportación como un **recorrido ordenado y best-effort**, no como
   una copia transaccional. Si necesita consistencia fuerte, se requerirá una decisión
   posterior con PITR/S3.

### 2.4 Memoria, errores y privacidad

- Cada página lee como máximo `limit` filas (≤100 DTOs) más los singletons acotados; no se
  carga una colección completa.
- Fallo de lectura → 503 sin `nextCursor`; cursor caducado/ inválido → 400; fallo de
  serialización → 500 sin `nextCursor`.
- `Cache-Control: no-store`; DTO del contrato; sin logs con correo, importes, tokens ni claves.

## 3. E048/E049 — Presupuestos

### 3.1 Diseño físico

El SDD y `BudgetKeys` documentan `USER#<id> / BUDGET#<YYYY-MM>#<categoryId>`. La propuesta usa
un ítem agregado por periodo:

```
USER#<id> / BUDGETSET#<YYYY-MM>
  items: [{categoryId, limitAmountCents}]  (0..50)
  period, currency, version, updatedAt
```

`BUDGET#` queda reservado y sin uso. No hay datos que migrar. El ítem agregado permite un
reemplazo atómico con **una** escritura condicionada; con ítems por categoría, 50 altas + 50
bajas superarían las 100 acciones de `TransactWriteItems`. El tamaño máximo (50 × ~100 B) está
muy por debajo de 400 KB. No se crean tablas, GSI ni recursos. Se actualizarán helpers y SDD
**solo después** de aprobar esta RFC.

### 3.2 E048 — lectura paginada

- Admite `limit`, `cursor` y `period`.
- Con `period`: lee el `BUDGETSET` (≤50) y pagina la lista ordenada por `categoryId` con
  `limit`/`cursor`. Aunque el ítem se lea completo, el tamaño está acotado por contrato; la
  respuesta nunca supera `limit` y el cliente recorre todas las páginas.
- Sin `period`: pagina entre periodos con `Query` + `Limit` + `ExclusiveStartKey`; cada
  periodo aporta sus elementos.
- Con `period`, la respuesta incluye `ETag: "b<YYYY-MM>-<version>"` (validador fuerte). Un
  periodo sin conjunto tiene versión 0 y `ETag: "bYYYY-MM-0"`. Sin `period` no se emite ETag:
  no hay un conjunto único que validar.
- `limit` 1..100, por defecto 25; sin `Scan`.

### 3.3 E049 — reemplazo completo

- **No admite `limit` ni `cursor`.** El cuerpo `BudgetReplace` es el conjunto completo del
  periodo (0..50 categorías, sin duplicados, importes no negativos).
- Exige `If-Match` con la misma forma y el mismo `period` del cuerpo. Ausente → 428; formato
  inválido o débil → 400/412; periodo o versión distintos → 412.
- `"b<period>-0"` crea el conjunto (versión 1) solo si no existe. `"b<period>-N"` reemplaza
  si la versión vigente es N.
- La respuesta `200` es `PageOfBudget` con **todos** los elementos del conjunto y **sin**
  `nextCursor` (está completo por construcción, ≤50). Se declara la cabecera `ETag` de la
  respuesta con el nuevo validador `"b<period>-<version+1>"`; es un cambio aditivo de
  OpenAPI.
- Reemplazo atómico con `UpdateItem`/`PutItem` condicionado por `version` o
  `attribute_not_exists` si expected=0. Dos nodos concurrentes: uno gana; el otro 412.
- Las categorías ausentes desaparecen del conjunto (no quedan ítems huérfanos).
- El cliente debe leer **todas** las páginas de `E048?period=...` antes de un reemplazo
  completo. Cada página del mismo periodo lleva el mismo ETag; si cambia entre páginas, el
  cliente reinicia. No se debe construir el reemplazo con una página parcial.

### 3.4 Pruebas obligatorias

0, 1, 25 y 50 categorías; lectura paginada 25/100; respuesta completa de E049 sin
`nextCursor`; reemplazo concurrente; ETag caducado; ETag de otro periodo; 428 sin `If-Match`;
412 con validador débil; E014 purga `BUDGETSET#`; aislamiento por usuario.

## 4. E026 — `availableEstimated`: alternativa B

### 4.1 Comparación de fuentes

| Fuente | Qué define |
|---|---|
| **A-070 §3** | Disponible = `incomeEquivalent − recurringFixedEstimate − oneTimeFixed − antExpenses − goalReservations`. El fijo recurrente pagado no se descuenta dos veces; el variable recurrente no se descuenta hasta el pago. |
| **`CalculateAvailableV6`** | Implementa la fórmula con `MonthlyFinancialSummary`: ingreso equivalente, estimación fija recurrente, fijos pagados, fijos puntuales, gastos variables y reservas. `pendingFixed = max(0, estimate − paid)`. |
| **`MonthlyFinancialSummary`** | Distingue `RecurringFixedEstimateCents`, `PaidRecurringFixedCents`, `OneTimeFixedCents`, `AntExpensesCents` y `GoalReservationsCents`. |
| **`RecurringPayment` / `RecurringCreate`** | Publican `type` (`income|expense|saving`), frecuencia, importe y fechas. **No** existe un campo que clasifique un recurrente como fijo o variable. |
| **Movimientos** | Publican `type`, `amount`, `categoryId`, `description`, `occurredAt`, vínculos. **No** existe un campo fijo/variable ni una categoría que separe ambos conceptos. |
| **Prototipo V6.0.2** | No se localizó en el repositorio una fuente verificable que asigne fijo/variable a cada recurrente. No se inventa. |
| **OpenAPI `FinancialSummary`** | `period`, `currency`, `income`, `expenses`, `savings`, `net`, `asOf`. `net` es realizado, no proyección. |

### 4.2 Por qué no se puede emitir `availableEstimated` con exactitud

Sin una clasificación fija/variable determinista, agrupar todos los `type=expense` como
obligación fija produciría un indicador que el usuario podría interpretar como dinero
realmente disponible, cuando en realidad mezcla estimaciones y gastos variables. La
alternativa A exigiría una fuente aprobada y demostrable para **todos** los componentes; no
existe en el repositorio. Por tanto, **se adopta B**.

### 4.3 Decisión B

- `FinancialSummary` **no cambia**. E026 devuelve `period`, `currency`, `income`, `expenses`,
  `savings`, `net`, `asOf`.
- `income` = suma de movimientos `type=income` del periodo. El ingreso habitual (`INCOME`,
  E024) no se suma para no mezclar ingreso base con ingreso efectivamente registrado; sigue
  consultable en E024.
- `expenses` = suma de movimientos `type=expense` del periodo.
- `savings` = **suma de movimientos `type=saving` independientes** (`contributionId` nulo) del
  periodo **+ suma de aportaciones `status=active`** con `occurredAt` en el periodo.
  - Un movimiento `saving` con `contributionId` no nulo **no se suma**: su aportación es la
    autoridad contable.
  - Una aportación revertida no cuenta como ahorro activo; su movimiento histórico se conserva
    pero no aumenta `savings`.
  - Un movimiento `saving` independiente creado por E028 o por E037 (regla recurrente `saving`)
    cuenta una sola vez.
  - `income` y `expense` se mantienen en sus respectivos agregados.
- `net` = `income − expenses − savings`, exacto en centavos; puede ser negativo.
- `asOf` = instante UTC del cálculo.
- Periodo por defecto: mes actual en la zona IANA de las preferencias; si no hay preferencia
  válida, `America/Mexico_City`. Límites locales convertidos a UTC.
- La proyección A-070 queda documentada como **pendiente de fuente suficiente**. No se
  publica un indicador aproximado.

### 4.4 Pruebas de E026

Sin datos → ceros; solo ingreso; solo gasto; **ahorro directo E028**; **ahorro recurrente
E037**; **aportación E044**; **aportación revertida E047**; combinación de los cuatro
escenarios; ausencia de doble conteo; modificación/eliminación de movimientos independientes;
periodo con cambio de mes/zona y dos usuarios; `net` puede ser negativo; el ingreso habitual
no se duplica.

## 5. E037 — Pago de regla recurrente

### 5.1 Ocurrencia, fecha y frecuencia

- Una ocurrencia es el registro manual de un pago real de una regla, identificado por
  `recurringPaymentId` y `paidAt`. No es un planificador; no avanza `nextDueDate`.
- `paidAt` se normaliza a UTC con milisegundos. La fecha de duplicado es el día civil en la
  zona IANA del titular (`PREFERENCES.timezone`, por defecto `America/Mexico_City`).
- Dos pagos de la misma regla el mismo día local → el segundo 409 aunque use otra
  `Idempotency-Key`. Pagos divididos el mismo día no están soportados por el contrato actual;
  requerirían RFC.
- E037 no exige que `paidAt` coincida con `nextDueDate` ni con la cadencia. Un pago puede ser
  anticipado o tardío. Solo exige regla existente, del titular y activa.

### 5.2 Replay, transacción y concurrencia

- Misma `Idempotency-Key` + mismo payload → replay 200 idéntico; payload distinto → 409;
  clave nueva el mismo día local → 409.
- `TransactWriteItems`: movimiento + `MOVEMENTPTR#` + `RECURRINGPAY#<rule>#<fecha>` +
  `IDEM#E037_PAY_RECURRING#` + `ConditionCheck` de regla activa. Dos EC2: solo una confirma;
  la otra replay o 409. Sin doble gasto.
- El importe del cuerpo es el realmente pagado y puede diferir del declarado en la regla;
  debe ser positivo, exacto en centavos y MXN.

### 5.3 Movimiento y categoría

El catálogo real (E022) es:

```
vivienda, servicios, transporte, comida, otros, ingreso, ahorro
```

**No existe `recurring`.** No se inventa una categoría pública. El movimiento se clasifica
por el tipo de la regla en una categoría existente:

| Tipo de regla | `categoryId` | Justificación |
|---|---|---|
| `expense` | `otros` | Categoría genérica de gasto; la regla no tiene categoría propia. No se atribuye a vivienda/comida/etc. |
| `income` | `ingreso` | Categoría de ingreso del catálogo. |
| `saving` | `ahorro` | Categoría de ahorro del catálogo. |

`description` = nombre de la regla; `recurringPaymentId` conserva el vínculo. E050 cuenta los
pagos `expense` bajo `otros`; no se presentan como gastos de una categoría específica. Android
puede agrupar por `recurringPaymentId` y mostrar el nombre de la regla. Añadir `categoryId` a
las reglas requeriría RFC propia.

## 6. E042 — Eliminación de meta: alternativa B

### 6.1 Alternativas

- **A (reversión automática):** una aportación puede repartirse entre varias metas; el
  contrato `Contribution` tiene un único `status` y una lista de allocations. Revertir la
  aportación completa afectaría a metas no implicadas; revertir solo la allocation no es
  representable sin un estado por allocation (cambio de contrato). Se descarta.
- **B (409 + E047):** E042 comprueba `activeContributions`; si > 0 → 409 sin modificar nada.
  El cliente revierte con E047, que revierte la aportación completa y todas sus allocations de
  forma atómica. Cuando no quedan activas, E042 marca `deleted=true`, `status=archived`,
  `savedAmount=openingSaved`, `version+1` y responde 204 sin cuerpo.

### 6.2 Enmienda de RFC-016 §17.7

RFC-016 describe `status: reversed` tras el borrado. Con B, la reversión ocurre **antes** y
E042 solo borra cuando no quedan activas. Es una **enmienda explícita de comportamiento**, no
un detalle interno. El contrato ya declara 409. Segundo DELETE sobre meta borrada → 204
(idempotente); meta inexistente → 404. E040/E041 tratan `deleted=true` como inexistente;
E038/E051 la excluyen.

## 7. E043–E047 — Aportaciones y reversión

### 7.1 Claves y transacciones

```
USER#<id> / CONTRIBUTION#<occurredAt>#<id>
USER#<id> / CONTRIBUTIONPTR#<id>
```

- E043 no escribe. E044 escribe aportación + puntero + **un** movimiento `saving` por el total
  + puntero del movimiento + N metas (`savedAmountCents += allocation`,
  `activeContributions+1`, `version+1`) + idempotencia. Máximo 18 acciones.
- Condiciones por meta: existe, `status=active`, no borrada,
  `savedAmountCents <= targetAmountCents − allocation` y `version` vigente (leída de forma
  consistente por el servicio). No hay topes constantes artificiales.
- E047 revierte la aportación (`status=reversed`, `version+1`) y resta cada allocation
  (`savedAmountCents -= allocation`, `activeContributions-1`, `version+1`) con condiciones
  `savedAmountCents >= allocation` y `activeContributions >= 1`; escribe idempotencia. No
  borra el movimiento histórico.
- E047 con `Idempotency-Key` repetida y mismo payload → 200 idéntico; otra clave sobre una
  aportación revertida → 409; doble reversión concurrente → solo una gana.

### 7.2 E043 `remaining`/`canCommit`

- `Σ allocations == amount` en centavos, sin metas duplicadas, 1..12 metas e importes
  positivos. Un reparto incompleto o con suma distinta es un error estructural (`422`), igual
  que en E044. E043 no persiste nada.
- `remaining = Σ max(0, allocation − capacidadRestante)` (con la suma ya exacta, el remanente
  no asignado es cero). `canCommit = remaining == 0` y todas las metas existen, son del titular
  y están activas.
- Errores estructurales → 400/422; meta inexistente/ajena → 404; capacidad insuficiente →
  200 con `canCommit=false` y `remaining` calculado. Es el propósito del preview.
- Prueba de paridad: los mismos datos deben ser aceptados o rechazados por E043 y E044 con la
  misma regla; el preview no puede ser más permisivo que el alta.

### 7.3 Historial en E027/E029/E030/E031/E026/E051

| Endpoint | Comportamiento |
|---|---|
| E027 | El movimiento `saving` aparece como movimiento normal; no se añade estado (no existe en el contrato). |
| E029 | Devuelve el movimiento con `contributionId`; no expone el estado de la aportación. |
| E030 | **T-1:** 409 si `contributionId` no está vacío y se intenta modificar `categoryId`, `description` u `occurredAt`. Sin el guardo, el libro diverge de la aportación. Movimientos sin vínculo no cambian. |
| E031 | Conserva el 409 ya implementado para movimientos con `contributionId`. Prueba de regresión; sin cambio de código. |
| E026 | `savings` = movimientos `saving` independientes + aportaciones `active`; el movimiento vinculado no se suma; revertida no cuenta. |
| E051 | Igual; `latestMovements` puede mostrar el movimiento histórico, pero `summary.savings` usa la fórmula de E026. |
| E014 | Purga genérica de la partición; se añade prueba con aportaciones y movimientos. |

El movimiento histórico permanece visible aunque la aportación esté revertida; es una
limitación del contrato actual (no hay `status` en `Movement`). Se documenta en Android; no se
inventa un campo público.

## 8. E050/E051 — Agregados y Redis

### 8.1 E050

- Lee el `BUDGETSET` del periodo (≤50).
- `usedAmount` = suma de movimientos `type=expense` del periodo por `categoryId`.
- Excluye reglas futuras; los pagos de E037 se cuentan una vez (son movimientos).
- Las aportaciones (`saving`) no son gasto.
- Periodo por zona IANA del titular; por defecto `America/Mexico_City`.

### 8.2 E051 — cálculo directo

- **Sin caché de dashboard en esta oleada.** Calcula desde DynamoDB en cada petición:
  `summary` (E026), metas activas no borradas (hasta 100, por `targetDate`/`id`) y 10
  movimientos más recientes.
- No se usa Redis para E051, no se usa `SCAN`, no se amplía el ACL y no se añaden hooks de
  epoch a E024/E028/E030/E031/E033–E036. Se elimina T-2.
- La caché queda registrada como **optimización posterior**; requerirá un mecanismo que
  garantice consistencia con las mutaciones (por ejemplo, invalidación transaccional o
  versión de datos) y su propio Gate.
- Coste/latencia: las consultas son `Query` por partición y `GetItem`; con los volúmenes
  previstos (≤100 metas, 10 movimientos, un periodo) es razonable. Se medirá en AWS; no se
  declara una cifra no verificada.
- Redis sigue siendo obligatorio para sesiones, cuotas y otros endpoints ya publicados; E051
  no depende de él.

## 9. Cambios materiales en OpenAPI

| Archivo | Cambio | Clasificación | Frase |
|---|---|---|---|
| `nexum-api-v1.json` / `.yaml` | `UserExport`: +6 propiedades opcionales | Aditivo | RFC v1.2.1 |
| `nexum-api-v1.json` / `.yaml` | `nexum_e048` 200: cabecera `ETag` | Aditivo | RFC v1.2.1 |
| `nexum-api-v1.json` / `.yaml` | `nexum_e049` 200: cabecera `ETag` | Aditivo | RFC v1.2.1 |
| `FinancialSummary` | **Sin cambio** (B) | — | — |
| Resto | Sin cambios de `required`, enum, códigos, rutas o parámetros | — | — |

Paridad YAML/JSON y `validar_h06.py` deben seguir en PASS. El comparador debe devolver
`COMPATIBLE`.

## 10. Cambio transversal T-1

| ID | Archivo(s) | Cambio | Autorización |
|---|---|---|---|
| **T-1** | `internal/application/movements/service.go` + pruebas | E030 responde 409 si el movimiento tiene `contributionId` no vacío y el parche intenta cambiar `categoryId`, `description` u `occurredAt`. Movimientos sin vínculo no cambian. | Frase §14 |

No se toca E027/E029/E031/E014 ni se añaden hooks de caché. Cualquier otro cambio requiere su
propio Gate.

## 11. Matriz de compatibilidad

| Dimensión | E020 | E026 | E037 | E042 | E043–E047 | E048–E050 | E051 |
|---|---|---|---|---|---|---|---|
| Estructural | Aditivo | Sin cambio | Sin cambio | Sin cambio | Sin cambio | Aditivo (ETag E048/E049) | Sin cambio |
| Comportamiento | Nuevas familias | Sin campo nuevo | Pago real; categoría por tipo | 409 con activas | Reversión sin borrar movimiento | Conjunto único; E049 completo | Cálculo directo |
| Semántica financiera | Exporta datos | `net` realizado | Gasto real bajo `otros` | No revierte en silencio | Ahorro activo vs revertido | Límites por periodo | Agregados consistentes |
| Android | Concatenar páginas; no snapshot | Sin cambio | Manejar 409 y `otros`/`ingreso`/`ahorro` | Revertir antes de borrar | Manejar 409/replay | Obtener ETag y todas las páginas de E048 | Sin cambio |
| DynamoDB | Sin cambio | Sin cambio | `RECURRINGPAY#` | `deleted`/`activeContributions` | `CONTRIBUTION#` + puntero | `BUDGETSET#` | Sin cambio |

## 12. Matriz de decisiones

| E-ID | PROBLEMA | DECISIÓN | CAMBIO PÚBLICO | CAMBIO INTERNO | RFC/GATE | IMPACTO ANDROID | RIESGO |
|---|---|---|---|---|---|---|---|
| E020 | Exportación incompleta y promesas de consistencia | +6 propiedades; cursor; garantías best-effort documentadas | Sí, aditivo | Proyección DTO | RFC v1.2.1 | Concatenar; no snapshot | Omisiones/duplicados bajo mutaciones; documentado |
| E026 | `availableEstimated` sin fuente fija/variable | **B:** no se añade; solo campos actuales | No | Cálculo realizado | RFC v1.2.1 | Sin cambio | Proyección A-070 pendiente |
| E037 | Categoría `recurring` inexistente | Mapeo por tipo: `expense→otros`, `income→ingreso`, `saving→ahorro` | No | Movimiento derivado | RFC v1.2.1 | Categoría genérica; vínculo por `recurringPaymentId` | Gasto recurrente bajo `otros` |
| E042 | Reversión automática riesgosa | **B:** 409 + E047; tombstone | No | `activeContributions` | RFC v1.2.1 | Revertir antes de borrar | Cambio de flujo |
| E043 | `remaining` ambiguo | No comprometible; `canCommit` | No | Cálculo puro | RFC v1.2.1 | Interpretar ambos | Resuelto |
| E044 | Movimiento vs aportación | Un `saving` por total; N metas atómicas | No | `CONTRIBUTION#` + puntero | RFC v1.2.1 | Sin cambio | Límite 12 |
| E045/E046 | Claves | `CONTRIBUTION#<fecha>#<id>` + puntero | No | Nuevas claves | RFC v1.2.1 | Sin cambio | Paginación filtrada |
| E047 | Reversión e historial | `status=reversed`, metas decrementadas, movimiento conservado | No | CAS + idempotencia | RFC v1.2.1 | Manejar 409/replay | Movimiento sin estado visible |
| E048/E049 | `BUDGET#` vs `BUDGETSET#`; ETag | `BUDGETSET#`; E048 pagina; E049 completo ≤50; ETag en ambos | Sí (ETag aditivo) | Diseño físico | RFC v1.2.1 | E048 todas las páginas; E049 completo | Cliente que no pagine E048 |
| E050 | Uso presupuestal | Desde `expense`; sin futuros; sin duplicar E037 | No | Lee `BUDGETSET#` | RFC v1.2.1 | Sin cambio | Depende de E037/E028 |
| E051 | Caché obsoleta | Cálculo directo; sin caché | No | Sin Redis | RFC v1.2.1 | Sin cambio | Latencia; se medirá |
| T-1 | E030 rompe aportación | 409 si vínculo y cambio de campos | No | Guardo E030 | Frase T-1 | Manejar 409 | Cambio de comportamiento E030 |

## 13. Riesgos

| ID | Riesgo | Mitigación |
|---|---|---|
| R-22-01 | `UserExport` ampliado rompe cliente estricto | Aditivo; documentar en Android |
| R-22-02 | Exportación no es snapshot | Garantías §2.3; sin infra nueva |
| R-22-03 | Categoría `otros` para recurrentes | Es una categoría real y genérica; no se atribuye una específica |
| R-22-04 | E042 409 cambia flujo | Alternativa B justificada; Android revierte antes |
| R-22-05 | E030 guardo transversal | Frase T-1; prueba de regresión |
| R-22-06 | `BUDGETSET#` cambia diseño físico | RFC + helpers tras aprobación; E014 e aislamiento |
| R-22-07 | E051 sin caché puede ser más lento | `Query` acotadas; medición AWS; caché futura con Gate |
| R-22-08 | Pagos divididos no soportados | Limitación declarada; futura RFC |

## 14. Frases de aprobación

**Frase A — RFC-022 v1.2.0:**

```text
APRUEBO_RFC_022_CONTRATOS_OLEADA_3_V1_2_1
```

**Frase B — cambio transversal T-1 (solo E030):**

```text
APRUEBO_CAMBIO_TRANSVERSAL_E030_MOVIMIENTO_APORTACION
```

La frase A habilita §2–§9. La frase B habilita solo §10. Ninguna autoriza merges, S3,
`terraform apply`, `instance refresh` ni despliegue. La v1.1.0 y la v1.0.0 no fueron
aprobadas y no se aplican.

## 15. Registro

| Fecha | Versión | Cambio | Estado |
|---|---|---|---|
| 2026-11-19 | 1.0.0 | Reconciliación inicial | **NO APROBADA** |
| 2026-11-19 | 1.1.0 | E020, E048/E049, E026, E037, E042, E043–E047, E050/E051 | **NO APROBADA** |
| 2026-11-19 | 1.2.0 | E049 sin paginación de respuesta; E026 alternativa B; E051 sin caché; E020 garantías reales; E037 categorías del catálogo; E042 B; T-1 únicamente | **NO APROBADA** |
| 2026-11-19 | 1.2.1 | E026 suma ahorros independientes + aportaciones activas sin doble conteo; E043 exige suma exacta y paridad con E044 | **APPROVED** — `APRUEBO_RFC_022_CONTRATOS_OLEADA_3_V1_2_1` |
