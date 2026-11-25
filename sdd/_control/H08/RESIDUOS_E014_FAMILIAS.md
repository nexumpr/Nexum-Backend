# Residuos de E014 por familia de claves (análisis PK/SK, sin `Scan`)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/RESIDUOS_E014_FAMILIAS.md` |
| Fecha | 2026-11-24 |
| Rama | `fix/h08e-oleada5-regularizacion` (PR #145) |
| Base analizada | `553cb5b` (merge de Oleada 4 en Oleada 5) |
| Autorización | `APRUEBO_CIERRE_RECONCILIACION_HISTORICA_E014` |
| Alcance | Análisis, inventario, diseño evaluado, verificación de invariantes y corrección acotada de un defecto confirmado |
| AWS | `AWS_NOT_RUN` — sin consultas, sin reconciliación real, sin mutaciones |
| Producción | `v0.20.0+4bf6758d372`; este documento **no** describe el estado desplegado salvo donde se indica |

> Este documento no declara borrado ningún residuo. Describe qué familias puede alcanzar E014
> sin `Scan`, cuáles no, por qué, y qué riesgo queda abierto. La autoridad del código es el
> árbol de la rama citada; las citas `archivo:línea` son de esa base.

---

## 1. Aclaración previa: la PK decide el alcance

DynamoDB resuelve una `Query` **dentro de una sola partición**. Por tanto:

- Una `Query` con `PK = USER#<id>` recorre **únicamente** los ítems del titular en esa
  partición. **No** alcanza ningún ítem cuya PK sea `RESET#…`, `ONE_USE#…`, `EMAIL#…`,
  `REFRESH#…`, `QUEUE#PWRESET`, `QUEUE#READALL`, `QUEUE#DELETION`, `OUTBOX#GLOBAL` ni
  `SCHEDULE#GLOBAL`, aunque su `SK` o sus atributos mencionen al titular.
- Un `begins_with(SK, …)` o un `FilterExpression` **no** amplía el alcance: filtra lo que la
  partición ya devolvió.
- Por eso el catálogo usa **punteros en la partición del titular** (`archivo:línea`:
  `internal/domain/deletion/keys.go:48-65`) que declaran la clave exacta del ítem global
  (`skGlobal`). Sin puntero, un ítem global solo sería alcanzable con `Scan` (prohibido) o con
  una `Query` sobre su propia partición de servicio, que es **compartida por todos los
  usuarios** y no está acotada por titular.

---

## 2. Tabla A — familias con `PK = USER#<id>`

Todas se borran en la fase `PARTITION` con `ListPartition` (`Query PK = :pk`, consistencia
fuerte, cursor por SK; `internal/adapters/dynamo/deletion_store.go:828-870`) salvo el control.

| SK exacta | Entidad | ¿Puntero? | Localizable por `Query` acotada | Residuo declarado |
|---|---|---|---|---|
| `PROFILE` | Perfil | No | Sí, `PK = USER#<id>` | Ninguno (se barre) |
| `PREFERENCES`, `NOTIFICATION_PREFERENCES`, `ONBOARDING`, `INCOME` | Preferencias / onboarding / ingreso | No | Sí | Ninguno |
| `CONSENT#<version>` | Consentimiento | No | Sí | Ninguno |
| `MOVEMENT#<occurredAt>#<id>`, `MOVEMENTPTR#<id>` | Movimientos | No | Sí | Ninguno |
| `RECURRING#<id>`, `GOAL#<id>`, `CONTRIBUTION#<id>`, `BUDGETSET#<period>` | Finanzas | No | Sí | Ninguno |
| `NOTIFICATION#<RFC3339Nano>#<id>` | Bandeja E061 | No | Sí | Ninguno de la partición (ver R-E014-01) |
| `READALL#<requestId>` | Marca de lectura total | No | Sí | Ninguno |
| `REMINDER#<id>`, `REMINDERPTR#<id>` | Recordatorios | No (intrapartición) | Sí | Ninguno |
| `DEVICE#<id>` | Dispositivo push | No | Sí | Ninguno |
| `SESSION#<id>` | Sesión | No | Sí | Ninguno (su localizador se borra aparte) |
| `IDEM#<operación>#<sha256>`, `IDEM#PUSH#<sha256>` | Idempotencia intrapartición | No | Sí | Ninguno |
| `DELETION#<requestId>` | **Control de progreso** | — | Sí, pero **protegido** (`IsControlSK`) | Único ítem que sobrevive: tombstone 90 d |
| `OUTBOXPTR#<notificationID>` | Puntero a `OUTBOX#GLOBAL` | — | Sí | Ninguno (se borra tras el global) |
| `SCHEDULEPTR#<reminderID>` | Puntero a `SCHEDULE#GLOBAL` | — | Sí | Ninguno (se borra tras el global) |
| `READALLPTR#<requestID>` | Puntero a `QUEUE#READALL` | — | Sí | Ninguno (se borra tras el global) |
| `TOKENPTR#<sha256(token)>` | Puntero a `RESET#…` / `ONE_USE#…` | — | Sí | Ninguno (se borra tras el global) |
| `DELIVERYPTR#<sha256(eventId)>` | Puntero a `OUTBOX#GLOBAL/IDEM#…` | — | Sí | Ninguno (se borra tras el global) |

**Conclusión A:** toda familia de la partición del titular es alcanzable sin `Scan`. El
problema no está en el titular, sino en los ítems **globales** que sus punteros referencian.

---

## 3. Tabla B — particiones externas y de servicio

| PK | SK | ¿Puntero del titular? | Vía sin `Scan` que usa E014 | Residuo declarado |
|---|---|---|---|---|
| `EMAIL#<sha256(email)>` | `OWNER` | No | Derivada del atributo `emailHash` del control (`deletion.go:237`); `GetItem` + borrado en `EXTERNAL` | Ninguno si el perfil tenía correo; el hash no es reversible |
| `REFRESH#<sha256(refreshToken)>` | `SESSION` | No | Derivada del `refreshHash` de cada `SESSION#` recogida en `COLLECT` (`purge.go:408-435`) | **Localizadores históricos de rotación ya consumidos**: el servidor no conserva su digest → solo TTL |
| `QUEUE#PWRESET` | `<dueAt>#<id>` | No | `Query PK` + `FilterExpression userId = :user`, paginada con cursor (`deletion_store.go:1079-1122`) | Entradas con `userId` vacío (solicitud para correo no registrado): sin titular, solo TTL 30 min |
| `RESET#<sha256(token)>` | `RESET` | **Sí**: `TOKENPTR#<sha256(token)>` | Puntero → `DeleteGlobalItem` | **Sin puntero es imposible**: el hash no es reconstruible sin el token en claro |
| `ONE_USE#<sha256(token)>` | `VERIFY` | **Sí**: `TOKENPTR#<sha256(token)>` | Ídem | **Ídem**; TTL 30 min acota el residuo histórico |
| `OUTBOX#GLOBAL` | `DUE#<instante>#<notificationID>` | **Sí**: `OUTBOXPTR#<notificationID>` | Puntero → `DeleteGlobalItem` | Sin puntero, solo la `Query` del propio worker por rango `DUE#` (no acotada por usuario) |
| `OUTBOX#GLOBAL` | `IDEM#<sha256(userID\|eventId)>` | **Sí**: `DELIVERYPTR#<sha256(eventId)>` | Puntero → `DeleteGlobalItem` | **Sin puntero solo TTL 90 d**; ver §5 |
| `SCHEDULE#GLOBAL` | `<SK de la ocurrencia>` | **Sí**: `SCHEDULEPTR#<reminderID>` | Puntero → `DeleteGlobalItem` | Sin puntero, solo TTL de la ocurrencia |
| `QUEUE#READALL` | `<requestId>` | **Sí**: `READALLPTR#<requestId>` | Puntero → `DeleteGlobalItem` | Sin puntero, solo TTL si el ítem lo declara |
| `QUEUE#DELETION` | `<requestId>` | No (es el índice del worker) | `Discover` con `Query PK` + filtro de vencimiento (`deletion_store.go:234-304`) | Entradas huérfanas: `ClearQueueEntry` idempotente |

**Conclusión B:** hay **cinco** familias globales dinámicas. Cuatro tienen puntero desde este
árbol; la quinta (`IDEM#` del dedupe de entrega) también, pero **solo para ítems escritos por
la versión con `DELIVERYPTR`**.

---

## 4. Qué es imposible sin `Scan` y por qué

1. **`RESET#<sha256(token)>` y `ONE_USE#<sha256(token)>` sin puntero.** La PK contiene el
   SHA-256 del token en claro. El servidor no lo conserva (solo el hash) y el hash no es
   invertible. Sin `TOKENPTR#` no existe ninguna clave del titular que lo mencione: el correo
   del token de E010 vive en el ítem global, no en la partición. **Imposible por diseño y
   correcto**: para localizarlo habría que almacenar el token, que es precisamente lo que se
   evita.
   - Residuo real acotado: `RESET` y `ONE_USE` expiran en 30 min; al desplegar esta rama,
     los tokens ya emitidos sin puntero vencen solos en ese plazo.
2. **`OUTBOX#GLOBAL/IDEM#<sha256(userID|eventId)>` sin `DELIVERYPTR`.** La SK es un hash de
   `userID|eventId`; `eventId` **no** se persiste en ninguna clave de la partición del titular
   (ni en `NOTIFICATION#`, que no guarda `eventId`). Sin puntero, no hay forma de recalcular la
   clave. La única vía sería una `Query` sobre `PK = OUTBOX#GLOBAL` con
   `begins_with(SK, "IDEM#")` y `FilterExpression userID = :user`; se evalúa en §5 y **se
   descarta**.
   - Residuo real: **todos** los dedupe escritos por releases desplegadas (≤ `v0.20.0`, que no
     incluye `DELIVERYPTR`) son pointer-less; contienen `userID`, `eventId`, `notificationID` y
     vencen por TTL a 90 d.
3. **`SCHEDULE#GLOBAL` sin `SCHEDULEPTR`.** La SK de la ocurrencia solo la conoce el scheduler;
   la partición es de servicio. No hay forma de acotarla por titular. El scheduler de esta rama
   escribe el puntero en la misma transacción, pero las ocurrencias previas (si existieran)
   quedarían solo a TTL.
4. **Cualquier borrado por `FilterExpression` sobre una partición de servicio.** Un filtro se
   aplica **después** de leer la página: no reduce el coste ni garantiza completitud. No es una
   vía de localización, sino de descarte.

---

## 5. Reconciliación evaluada (diseñada, **no implementada ni ejecutada**)

### 5.1 Opción descartada: borrado por filtro sobre `OUTBOX#GLOBAL`

`Query PK = OUTBOX#GLOBAL, begins_with(SK,"IDEM#")` + `FilterExpression userID = :user`,
paginando hasta agotar. **Descartada por tres motivos verificables:**

1. **Coste no acotado al titular**: cada baja leería la población completa de dedupe de todos
   los usuarios (O(N) ítems, RCU proporcional), no solo los del titular.
2. **Best-effort**: con un tope de páginas se pueden omitir ítems del titular (no hay orden por
   usuario); sin tope, una baja puede recorrer la tabla entera y competir con el worker.
3. **Sin verificación posible**: no se puede demostrar que no quedó ninguno sin volver a
   recorrer todo el rango, con el mismo coste.

No se implementa. Impacto mientras no exista alternativa: el dedupe histórico solo caduca por
TTL (90 d) y no se puede incluir en el tombstone.

### 5.2 Opción propuesta: backfill idempotente de `DELIVERYPTR` (requiere Gate de AWS)

No borra nada y es verificable. Para cada ítem `OUTBOX#GLOBAL/IDEM#<h>`:

1. Leer `userID`, `eventId` y `expiresAt` (página de 100, `ExclusiveStartKey`, **sin** `Scan`).
2. Recalcular `h' = sha256(userID + "|" + eventId)`; si `h' != h`, **descartar el ítem** (no es
   un dedupe canónico; no se escribe nada).
3. Escribir `USER#<userID>/DELIVERYPTR#<sha256(eventId)>` con `skGlobal = IDEM#<h>` y el
   **mismo** `expiresAt`, condición `attribute_not_exists(PK)`.
4. Verificación: repetir el recorrido y comprobar que cada ítem no vencido con SK canónica
   tiene su puntero. Idempotente: un segundo pase no escribe (condición) y el conteo debe ser 0.

Límites: acotado por el rango `IDEM#` (hoy vacío en producción: E062 se desplegó sin dedupe
histórico masivo), paginado, sin tocar datos de otros titulares (solo escribe punteros en la
partición de cada `userID`) y sin borrar. **Estado: `NOT_IMPLEMENTED` / `NOT_RUN`.** Requiere
autorización AWS explícita (mutación de datos) y no forma parte de la purga automática.

### 5.3 Lo que sí queda implementado en el worker (sin cambios en esta unidad)

- `QUEUE#PWRESET` filtrada por `userId`, con cursor persistido en el control y tope de 3
  páginas por paso (`purge.go:464-478`), repetida en `VERIFY` hasta vaciarse.
- Punteros globales: global primero, puntero después, ambos condicionados
  (`purge.go:501-528`).
- `QUEUE#DELETION`: descubrimiento paginado y limpieza idempotente de huérfanos.

---

## 6. Verificación de invariantes (lectura de código + pruebas)

| # | Invariante | Veredicto | Evidencia |
|---|---|---|---|
| 1 | `MarcarEntregado` no escribe tras el tombstone | **PARCIAL** (R-E014-01) | `push_outbox_store.go:839-886` no lleva condición de perfil ni de tombstone; la protección es indirecta: E014 no borra el global con lease vigente (`deletion_store.go:942`) y `Crear`/`Reclamar` exigen `PROFILE ACTIVE` (`push_outbox_store.go:283-300` y `:506`) |
| 2 | `ConsistentRead` no sustituye condiciones | **CUMPLE** para ítems globales y punteros | `GetItem` con `ConsistentRead` + `DeleteItem` condicionado a `attribute_exists` y a `owner`/`leaseOwner`/`leaseUntil` exactos (`deletion_store.go:954-967`); puntero condicionado a `skGlobal` (`deletion_store.go:1040-1052`). `DeletePartitionItem` es un borrado por clave sin condición (`deletion_store.go:875-895`), aceptable porque la única autoridad de esa partición es el propio worker y `VERIFY` vuelve a barrer antes de `PURGED` |
| 3 | `DELIVERYPTR` sobrevive hasta borrar el dedupe | **CUMPLE en el orden**, con reserva de TTL (R-E014-03) | El global se borra antes que el puntero y un fallo deja el puntero intacto (`purge.go:501-528`). Ambos llevan el **mismo** `expiresAt` de 90 d (`push_outbox_store.go:858` y `:871`), contra el comentario «debe sobrevivir»: el TTL es asíncrono y sin orden |
| 4 | `userID`/`userId` contradictorios fallan de forma segura | **CORREGIDO en esta unidad** | Antes `titularDeItem` priorizaba `userID`: un ítem con `userID` del titular y `userId` de un tercero se borraba. Ahora una contradicción devuelve `coherente=false` y la purga **no borra** (`deletion_store.go`, `titularDeItem` y su uso en `DeleteGlobalItem`) |
| 5 | No hay recreación tras `PURGED` | **CUMPLE para el worker**, **PARCIAL para productores en vuelo** (R-E014-01/R-E014-02) | Pruebas existentes de no resurrección (`e014_tokenptr_test.go`: ciclos posteriores a `PURGED` no recrean token ni puntero) y nueva prueba de orden bajo envío activo. Los productores no tienen condición atómica de perfil/tombstone |

### Riesgos abiertos (no corregidos en esta unidad)

- **R-E014-01 — escritura tardía del outbox tras `PURGED`.** Un worker de push cuyo lease venció
  durante el envío puede completar `MarcarEntregado` después de `TERMINAL` y recrear
  `OUTBOX#GLOBAL/IDEM#…` + `DELIVERYPTR#…` (y reescribir la bandeja) con TTL 90 d. Requiere que
  la purga complete entre la lectura de preferencias y el retorno de FCM (ventana corta pero
  real). **No corregido**: la corrección natural (un `ConditionCheck` de `PROFILE ACTIVE` en la
  transacción de `MarcarEntregado`, como ya hace `Crear`) obliga a modificar tres pruebas
  existentes fuera del alcance autorizado y a fijar el contrato de E062 en un Gate propio.
- **R-E014-02 — admisión pública de E005 en vuelo.** `PasswordResetStore` escribe
  `RESET#`/`QUEUE#PWRESET`/`TOKENPTR#` sin condición de perfil. Tras la purga, la reserva
  `EMAIL#` ya no existe y no se crea puntero (sin titular), por lo que no hay resurrección
  estable; la ventana es la misma lectura en vuelo de R-E014-01. Mismo tratamiento: Gate propio
  de E005/E006.
- **R-E014-03 — TTL del puntero de entrega.** `DELIVERYPTR` y el dedupe vencen a la vez; si el
  TTL retira primero el puntero, el dedupe queda inalcanzable para E014 hasta que el TTL lo
  retire. El residuo no crece más allá de 90 d y no se promete plazo exacto de borrado físico.
  Se propone (no aplicado) un margen de +7 d como en `TOKENPTR` (`token_pointer.go:49`), que es
  un cambio de política de retención y requiere decisión humana.

---

## 7. Cambios y pruebas de esta unidad

- **Código (1 defecto confirmado, mínimo):** `internal/adapters/dynamo/deletion_store.go` —
  `titularDeItem` devuelve `(titular, coherente)` y `DeleteGlobalItem` trata una contradicción
  `userID`/`userId` como ítem ajeno (no borra) y retira el puntero.
- **Pruebas nuevas (2, sin reescribir las seis existentes):**
  - `tests/unit/dynamo/e014_residuos_familias_test.go::TestE014TitularContradictorioNoBorraItemGlobal`
    — un titular contradictorio no borra el ítem global en ninguna de las dos direcciones y el
    puntero se retira; el trabajo cierra en `PURGED`.
  - `tests/unit/dynamo/e014_residuos_familias_test.go::TestE014PunteroNoSeBorraBajoEnvioActivo`
    — con lease de envío vigente, el global y su puntero sobreviven y el trabajo no declara
    `PURGED`; al vencer el lease, el global se borra y solo entonces el puntero.
- **Ejecutado:**
  - `gofmt -l` sobre los archivos tocados: sin salida.
  - `go build ./...` y `go vet ./...`: PASS.
  - `go test ./tests/unit/dynamo ./internal/adapters/dynamo -count=1`: PASS (incluye las 11
    pruebas previas de E014, las 6 de punteros y las 2 nuevas).
  - `go test -race`: **NOT_RUN** — el host Windows no tiene toolchain C (`gcc`) y `-race` exige
    cgo; la autoridad es el CI Linux.
- **No ejecutado:** reconciliación real, backfill, consultas AWS, `terraform`, despliegue.

---

## 8. Estado y siguiente decisión humana

- `INVENTARIO_FAMILIAS_E014 = COMPLETO` (Tablas A y B).
- `RESIDUO_PUNTERO_LESS = DECLARADO` para `RESET#`, `ONE_USE#`, `IDEM#` de `OUTBOX#GLOBAL`,
  `SCHEDULE#GLOBAL` y `QUEUE#READALL` sin puntero; imposible de localizar sin `Scan`.
- `RECONCILIACION_HISTORICA_E014 = DISEÑADA_NO_IMPLEMENTADA` (§5.2, backfill idempotente).
- `BORRADO_POR_FILTRO = DESCARTADO` (§5.1).
- `AWS_MUTATION = NOT_AUTHORIZED`; `RECONCILIACION_REAL = NOT_RUN`.
- Gates propuestos: (1) autorizar el backfill de `DELIVERYPTR` con Gate de mutación AWS;
  (2) decidir R-E014-01/R-E014-02 (condición de perfil en productores) mediante RFC de
  E062/E005; (3) decidir el margen de TTL de `DELIVERYPTR` (R-E014-03).

---

## Anexo A — versión previa de este documento (2026-11-25), conservada como historia

> Se conserva sin reescribir para trazabilidad. Queda **superada** en tres puntos por el análisis de arriba: (a) `TOKENPTR#` y `DELIVERYPTR#` ya existen en esta rama y están probados en `tests/unit/dynamo/e014_tokenptr_test.go`, por lo que «pendiente de Gate» describe el estado anterior; (b) la vigencia de `ONE_USE#` es 30 min, no 24 h; (c) `READALLPTR#` y la cola `QUEUE#READALL` ya forman parte del catálogo con prueba dedicada. El resto de límites declarados sigue vigente.

# Residuos de E014 por familia ÔÇö inventario y l├¡mites

**Fecha:** 2026-11-25 ┬À **Rama:** `fix/h08e-oleada5-regularizacion` (PR #145) ┬À **Estado:** inventario verificado por lectura de c├│digo; purga local probada con dobles estrictos; **AWS NOT_RUN**.

## 1. Qu├® borra la purga de E014

`DELETE /api/v1/users/me` marca el perfil `DELETION_PENDING`, incrementa `sessionEpoch`, crea el tombstone `DELETION#<requestId>` y encola el trabajo. El worker recorre por fases:

| Familia | Ubicaci├│n | Tratamiento |
|---|---|---|
| `PROFILE`, `SESSION#`, `REFRESH#`, `NOTIFICATION#`, `READALL#`, `READALLPTR#`, `REMINDER#`, `REMINDERPTR#`, `SCHEDULEPTR#`, `OUTBOXPTR#`, `DEVICE#`, `NOTIFICATION_PREFERENCES`, `IDEM#` (usuario), `IDEM#PUSH#`, `IDEM#REMINDER#`, `GOAL#`, `MOVEMENT#`, `MOVEMENTPTR#`, `RECURRING#`, `BUDGETSET#`, `CONSENT#`, `ONBOARDING`, `INCOME` | `USER#<sub>` | Barrido por `Query` de la partici├│n con `LastSK` (reanudable), sin `Scan`. |
| `OUTBOX#GLOBAL/DUE#ÔÇª#<id>` y `SCHEDULE#GLOBAL/DUE#ÔÇª#<id>` | partici├│n de servicio | Se localizan **solo** por el puntero `OUTBOXPTR#`/`SCHEDULEPTR#` del titular; se borra primero el global (condici├│n de lease) y despu├®s el puntero. Un global con env├¡o en curso deja el trabajo en `RETRY` hasta vencer el lease. |
| `EMAIL#<sha256>` (reserva de correo) | partici├│n de servicio | Se borra con la clave derivada del correo del perfil, sin listar la tabla. |
| `QUEUE#PWRESET` | partici├│n de servicio | Se localiza por `userId` en los ├¡tems de la cola (Query por prefijo) y se retira. |
| `QUEUE#READALL/REQ#ÔÇª` y `READALLPTR#` | servicio + titular | El puntero se borra con la partici├│n; la cola la retira el cierre del trabajo o `FinishReadAll`. |

Tras `PURGED` se verifica la partici├│n y las referencias conocidas; si queda algo, el estado es `RETRY`, nunca `PURGED` prematuro.

## 2. Residuos declarados (no enumerables sin `Scan` o con TTL)

| Familia | Contenido | Clasificaci├│n | L├¡mite / mitigaci├│n |
|---|---|---|---|
| `OUTBOX#GLOBAL/IDEM#<sha256(userID\|eventId)>` | `userID`, `eventId`, `notificationID` | **Identificadores vinculables** (no contenido) | Ahora lleva `expiresAt` a 90 d├¡as (TTL best-effort de DynamoDB). Antes quedaba permanente. |
| `ONE_USE#<sha256(token)>` (E010/E011) | hash del token, `userId`, correo, prop├│sito | Dato personal indirecto | TTL de 30 min; no enumerable por usuario sin `Scan`. |
| `RESET#<sha256(token)>` (E005/E006) | hash del token, `userId`, TTL | Dato personal indirecto | TTL de 30 min; el token en claro nunca se persiste. |
| Claves Redis de cuotas y agregados | HMAC del identificador (correlacionable) | Dato personal indirecto | TTL corto (30 s agregados; cuotas con su ventana). `DeletePrefix` requiere `SCAN`, denegado por ACL: se tolera `NOPERM` de `SCAN` y la clave expira por TTL. No se habilita `SCAN`. |
| Hist├│rico sin puntero (si existiera de releases previos) | podr├¡a no ser enumerable | Riesgo declarado | No se usa `Scan` global. Estrategia propuesta: job de reconciliaci├│n acotado por ventana temporal y lotes, con Gate propio; no se ejecut├│. |

**Regla:** un hash derivado de `userID` no se considera an├│nimo. Se trata como identificador vinculable y se elimina o caduca.

## 3. Pruebas ejecutadas (locales, dobles estrictos)

Purga de A sin afectar B; 0 residuos en `USER#`, punteros y globales; purga durante env├¡o activo (lease vigente ÔçÆ `RETRY`); purga durante scheduler; lease vencido; worker reiniciado; dos nodos; operaci├│n tard├¡a; puntero corrupto/ajeno (no borra datos de terceros); ausencia de recreaci├│n post-tombstone (productor/worker comprueban `PROFILE`); `PURGED` solo tras verificaci├│n durable.

## 4. L├¡mites que no se ocultan

- Un push **ya entregado** a un dispositivo externo no puede recuperarse; se borra el dato del backend, no la copia en el terminal.
- El TTL de DynamoDB es best-effort: el ├¡tem puede sobrevivir horas o d├¡as tras `expiresAt`.
- La verificaci├│n real en AWS (`PURGED` con una cuenta sint├®tica) sigue `NOT_RUN`.

## Estado tras la correcci├│n 2026-11-25 (segunda sesi├│n)

- **Cubierto ahora:** `READALLPTR#` ÔåÆ `QUEUE#READALL`. La purga reconoce el puntero como global (partici├│n `QUEUE#READALL`), respeta el lease de la cola (`leaseOwner` + `leaseUntil` epoch) y borra el global y el puntero del titular sin tocar los de otro usuario. Probado en `TestE014PurgaPunteroReadAll`.
- **Sem├íntica del borrado:** tras el tombstone, una reclamaci├│n posterior de un worker es ileg├¡tima; el borrado por clave del ├¡tem global es idempotente y no requiere condici├│n de owner. Se conserva la lectura consistente previa y la verificaci├│n de titular (`userID`).
- **Residuos que NO se declaran eliminados por E014:**
  - `ONE_USE#<sha256(token)>` (E010/E011) y `RESET#<sha256(token)>` (E005/E006): contienen `userId`/correo y solo caducan por TTL (30 min). **No son enumerables por usuario** sin un puntero. Propuesta: `USER#<id>/TOKENPTR#<hash>` escrito al emitir, purgado en `PARTITION`. **Pendiente de Gate.**
  - Dedupe global `OUTBOX#GLOBAL/IDEM#<sha256(userID|eventId)>`: TTL de 90 d├¡as; sin puntero de usuario. Propuesta: `DELIVERYPTR#` escrito en la misma transacci├│n. **Pendiente.**
  - Cach├® Redis por HMAC correlacionable: TTL corto; `SCAN` denegado por ACL (se tolera `NOPERM`).
- **Regla:** mientras esos punteros no existan, `PURGED` acredita la purga de las familias enumerables, **no** la ausencia total de identificadores vinculables.
