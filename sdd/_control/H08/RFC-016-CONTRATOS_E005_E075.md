# RFC-016 — Cierre contractual E005–E075 (versión normativa v3)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/RFC-016-CONTRATOS_E005_E075.md` |
| Versión | **4.0.0 — normativa única** (sustituye a v1, v2 y v3) |
| Estado | **PROPUESTO — NO APROBADO — NO APLICADO** |
| Fecha | 2026-11-16 |
| Baseline | `main` `8bc2548` · rama `chore/pre-e005-contratos-permisos` · PR #65 |
| Autoridad de transporte | OpenAPI 3.1 aprobado (H06) |
| Aprobación | `APRUEBO_RFC_016_CONTRATOS_E005_E075` |

## 0. Registro de cambios y precedencia

| Versión | Cambio | Estado |
|---|---|---|
| 1.0.0 | Clasificación inicial ESENCIAL/COMPATIBILIDAD/OPCIONAL/OBSOLETA | sustituida |
| 2.0.0 | §17 con correcciones A–G (revisión material, públicas, outbox, época, 3 estados, E058 exacto) | sustituida |
| **3.0.0** | **Norma única.** Incorpora: worker de outbox con líder y leasing (E005), transacción válida de dos acciones (E006), `E040.openingSaved` como ESENCIAL, modelo de tres estados de los opcionales, consolidación sin duplicidad | **vigente** |

**Esta versión es la única norma.** v1 y v2 quedan como historia en el registro Git; no deben
citarse como vigentes. Cualquier contradicción con ellas se resuelve aquí.

## 1. Regla de decisión

1. El **OpenAPI aprobado** manda sobre la forma del transporte.
2. La **auditoría V6.0.2** aporta necesidades funcionales, no contrato.
3. La app Android **no está publicada**: puede adaptarse al contrato; lo que no puede es quedarse
   sin un dato que **solo el servidor** puede producir.
4. Toda diferencia se clasifica. **Clasificar ≠ resolver.** `OPCIONAL` significa pospuesto y
   registrado, nunca «no hace falta».

## 2. Resultado de la reconciliación (verificado)

```
71 operaciones · 812 diferencias · 0 sin clasificar
ESENCIAL=2 · COMPATIBILIDAD=22 · OPCIONAL=781 · OBSOLETA=7
Opcionales: 462 POSPUESTA_CONFIRMADA (derivación declarada) · 319 PENDIENTE_DE_DECISION
```

Registro por endpoint: `sdd/_control/H08/CLASIFICACION_E005_E075.json`
(generado por `herramientas/clasificar_contratos_e005.py`, determinista).

| Clase | N | Endpoints | Bloquea congelar |
|---|---|---|---|
| ESENCIAL | 2 | E040 (`openingSaved`), E055 (`hasEnoughData`) | **Sí** |
| COMPATIBILIDAD | 22 | E006, E017, E031, E037, E042, E052×2, E055, E058×12, E063, E075 | **Sí** |
| OBSOLETA | 7 | E005–E011 (`SESSIONHASH`, 28 refs) | **Sí** (enmienda editorial) |
| OPCIONAL confirmada | 462 | varios | No (derivación declarada) |
| OPCIONAL pendiente | 319 | 68 endpoints | **No**, pero debe cerrarse en el Gate de cada endpoint |

### 2.1 Nuevo hallazgo ESENCIAL: E040 `openingSaved`

Verificado en OpenAPI: `GET /goals/{goalId}` devuelve
`{id, name, targetAmount, savedAmount, currency, targetDate, status, version}` y nada más.
`POST /goals` **acepta** `initialSavedAmount`, pero **ni E039 ni E040 lo devuelven**.

Consecuencia: Android no puede mostrar el saldo inicial de la meta ni separar «aportado» de
«saldo inicial»; `contributedNet = savedAmount − openingSaved` es **incalculable** por el cliente.
**No es presentación: es un dato que solo el servidor tiene.** → **ESENCIAL**, enmienda **A7**.

### 2.2 Modelo de tres estados de los opcionales

Cada candidato opcional lleva `decision` y `motivo`:

- `POSPUESTA_CONFIRMADA`: existe **derivación declarada** (composición desde un endpoint atómico,
  cálculo con importes del propio contrato, o dato de presentación que no altera la lógica).
- `PENDIENTE_DE_DECISION`: **no se demuestra** que Android funcione sin él. No se declara opcional
  confirmado; queda como decisión abierta del Gate de ese endpoint.

Los 319 pendientes **no bloquean la congelación** (no son esenciales ni incompatibilidad), pero
**sí** deben cerrarse endpoint por endpoint antes de implementarlo. Lista por operación en el JSON.

## 3. Códigos de éxito — decisión: conservar

| ID | Operación | Vigente | Decisión | Motivo |
|---|---|---|---|---|
| E017 | `POST /users/me/consents` | `201` | Conservar | crea recurso |
| E031 | `DELETE /movements/{id}` | `204` | Conservar | borrado idempotente |
| E037 | `POST /recurring-payments/{id}/payments` | `200` | Conservar | ya devuelve el movimiento con `id` |
| E042 | `DELETE /goals/{id}` | `204` | Conservar | borrado idempotente |
| E063 | `POST /notifications/read-all` | `202` | Conservar | no se demostró sincronía |

**Sin cambio de contrato** (A4 solo registra la decisión).

## 4. E005 — `POST /auth/password/forgot`

### 4.1 Contrato

```
Request  EmailRequest { email }                        (público, sin Bearer)
Response 202 Accepted { status: "accepted", requestId }
Errores  400 409 422 429 500 503
```

### 4.2 Qué garantiza el 202 — semántica exacta

> **`202` = «solicitud aceptada y trabajo persistido de forma durable».**
> Garantiza que el envío **se intentará**, no que el correo se haya entregado ni que SES lo haya
> aceptado. El mensaje al usuario nunca dice «enviado».

Si no se puede persistir el trabajo → **no hay `202`**: se devuelve `503` genérico.

### 4.3 Transacción de admisión (antes de responder)

`TransactWriteItems` de **3 acciones** sobre ítems distintos:

| # | Acción | Ítem | Condición |
|---|---|---|---|
| 1 | Put | `USER#<id>` / `PWRESET#<hmacEmail>` `{tokenHash, expiresAt, used:false, version:0}` | `attribute_not_exists(PK)` o versión anterior |
| 2 | Put | `USER#<id>` / `OUTBOX#<ulid>` `{tipo, estado:"PENDIENTE", intentos:0, proximoIntento, tokenCifrado, expiresAt}` | — |
| 3 | ConditionCheck | `USER#<id>` / `PROFILE` | existe y `status = ACTIVE` |

Cuenta inexistente: se ejecutan las **mismas tres acciones** con `destinatario = null` (sin
`tokenCifrado`), de modo que coste, forma y número de acciones son idénticos.

### 4.4 Worker: procesamiento independiente de nuevas peticiones

**Componente:** goroutine dentro del **binario Go existente** (`cmd/server`), arrancada con el
servidor y detenida con el contexto de apagado. **Sin recursos AWS nuevos.**

| Aspecto | Diseño |
|---|---|
| **Coordinación entre 2 nodos** | Líder único por *lease* en Redis: `SET nexum:v1:<env>:worker:pwreset:leader <inst> NX PX 15000`; renovación cada 5 s. Solo el líder reclama y envía |
| **Reclamación atómica** | `UpdateItem` sobre el ítem de outbox con `ConditionExpression: estado = :pendiente AND proximoIntento <= :now` → `SET estado=:enviando, owner=:inst, leaseUntil=:now+30s` |
| **Recuperación tras caída** | Barrido: ítems `estado = :enviando AND leaseUntil < :now` → vuelven a `:pendiente` (o `:requiere_revision` tras 3 ciclos). Funciona al reiniciar EC2: el estado vive en DynamoDB, no en memoria |
| **Backoff** | 30 s, 2 min, 10 min, 1 h, 6 h; **máximo 6 intentos** → `estado=:fallido` |
| **Expiración** | al superar `expiresAt` (TTL del token, 30 min) → `estado=:expirado` y se elimina el cifrado |
| **Apagado controlado** | `Shutdown` cancela el contexto, espera el envío en curso ≤ 5 s y devuelve el ítem a `:pendiente` si no terminó |
| **Redis caído** | no hay líder ⇒ **no se envía** (fail-safe); los ítems esperan. La admisión sigue respondiendo `202` (el trabajo está persistido). Se documenta la degradación |

### 4.5 Token: cómo se envía si solo se persiste el hash

Excepción explícita y acotada al principio «solo hashes»:

| Aspecto | Decisión |
|---|---|
| Cifrado | **AES-256-GCM** con clave de 32 bytes `PASSWORD_RESET_SEAL_KEY_B64` en el bundle RFC-004 existente (sin KMS ni secreto nuevo) |
| AAD | `<userId>|<outboxId>`: impide trasplantar el cifrado a otro ítem |
| Campo | `tokenCifrado` en el ítem de outbox, **nunca** en el ítem `PWRESET#` (que solo guarda `tokenHash`) |
| Ciclo de vida | se crea en la admisión y se **elimina** (`REMOVE tokenCifrado`) al pasar a `ENVIADO`, `EXPIRADO` o `FALLIDO` |
| TTL | el ítem de outbox expira con el token (30 min); DynamoDB lo purga |
| Acceso | solo el worker lo descifra, en memoria, y lo entrega al SDK de SES |
| Logs | prohibido registrar el token, el cifrado o la clave; el redactor de A-090 cubre `token`, `tokenCifrado`, `password` |
| Memoria | el buffer se sobrescribe tras el envío (`zeroize` best-effort) |

**Alternativa descartada:** rotar el token en cada reintento. Rompería el enlace ya entregado si el
proceso cayó después de que SES aceptó el mensaje.

### 4.6 Semántica de entrega y duplicados

**No es exactly-once.** Si el proceso muere después de que SES aceptó y antes de marcar `ENVIADO`:

- el ítem vuelve a `:pendiente` por expiración de lease;
- el reintento reenvía **el mismo enlace** (mismo token, por eso se cifra);
- el usuario puede recibir **dos correos idénticos**; el enlace es de un solo uso y sigue válido;
- el segundo uso devuelve el error genérico sin revelar estado.

Se documenta como **at-least-once**; no se promete unicidad de entrega.

### 4.7 No enumeración

| Escenario | Respuesta | Nota |
|---|---|---|
| Correo existente | `202` | trabajo persistido y enviado |
| Correo inexistente | `202` | mismas 3 acciones, sin envío |
| Fallo de SES | `202` | el trabajo queda `PENDIENTE`; **no** cambia la respuesta |
| Fallo de DynamoDB | `503` | no hay trabajo persistido; no se puede aceptar |
| Rate limit | `429` | mismo límite por IP y por correo, exista o no |
| Cuenta eliminada | `202` | mismas acciones, sin envío |

**Mitigaciones temporales (no promesas):** suelo de latencia con espera acotada (≤ 250 ms) + jitter,
y prueba estadística de comparación de distribuciones (α = 0,01; p50/p95 sobre 200 muestras por
rama). No se afirma que los tiempos sean indistinguibles.

## 5. E006 — `POST /auth/password/reset`

### 5.1 Contrato

```
Request  PasswordResetRequest { token, newPassword }   (público, sin Bearer)
Response 200 Accepted { status, requestId }
```

### 5.2 Transacción válida (corregida)

DynamoDB **no permite dos acciones sobre el mismo ítem** en `TransactWriteItems`. La versión
anterior de este documento proponía varias acciones sobre `PROFILE`: era inválida. Diseño correcto,
**2 acciones sobre 2 ítems distintos**:

| # | Acción | Ítem | Expresión |
|---|---|---|---|
| 1 | `Update` | `USER#<id>` / `PROFILE` | `SET passwordHash=:h, passwordChangedAt=:t, sessionEpoch = if_not_exists(sessionEpoch, :zero) + :one ADD version :one` · condición `attribute_exists(PK) AND version = :v` (o `attribute_not_exists(#version)` si es legado) |
| 2 | `Update` | `USER#<id>` / `RESET#<sha256(token)>` | `SET used=:true, usedAt=:t` · condición `attribute_not_exists(used) OR used = :false` |

Un solo `Update` sobre `PROFILE` cambia hash, `passwordChangedAt`, `sessionEpoch` y `version`.
Condición de carrera: dos resets simultáneos → uno gana; el otro falla por `version` o por token
consumido → `409` genérico.

### 5.3 Compatibilidad con sesiones y JWT anteriores a la época

| Caso | Tratamiento |
|---|---|
| PROFILE sin `sessionEpoch` (legado) | se lee como `0` |
| Sesión sin `sessionEpoch` | se trata como `0` |
| Tras el primer cambio de contraseña | `sessionEpoch = 1` ⇒ toda sesión con época `0` queda inválida |
| Sesiones nuevas | deben persistir la época vigente al emitirse |
| JWT ya emitidos | se validan por `sub`/`sid`; la época se comprueba contra PROFILE, no contra el token |

### 5.4 Casos verificados en el diseño

| Caso | Resultado esperado |
|---|---|
| 1 sesión | revocada por época |
| 100 sesiones | revocada (no se recorren) |
| >100 sesiones | revocada: el mecanismo es O(1), no hay límite |
| Reset concurrente | 1 gana, otro `409` |
| Token usado | `409` genérico |
| Token expirado | `400` genérico |
| Sesión antigua tras el cambio | rechazada |
| Login nuevo tras el cambio | válido con la época nueva |

### 5.5 A6 — cambio compartido

Para que la época **surtа efecto** hay que tocar código de E002, E003 y el middleware de sesiones.
**No se modifica ahora.** A6 describe el cambio exacto y exige **RFC + Gate propios** durante la
implementación de E006. **Congelar el contrato de E006 no equivale a haber implementado A6.**

**Alternativa si A6 se rechaza:** revocar hasta 30 sesiones en una transacción (30 acciones) y
documentar el límite; el resto se revoca en la limpieza perezosa. Menos robusta, explícita.

## 6. E052 — Insights

Sin `enoughData`/`minimumSamples`, `items: []` es ambiguo (sin historial vs sin patrones).
`items[].explanation`, `evidenceCount` y `modelVersion` **ya existen**. Enmienda **A5**:
añadir `enoughData` (boolean, requerido) y `minimumSamples` (integer, requerido) en la raíz.

## 7. E055 — Recomendaciones (K-Means)

Contrato vigente: `items[]{id,title,reason,disclaimer,modelVersion}` + `nextCursor`.
Enmienda **A2**: añadir `hasEnoughData` (boolean, requerido). **`recommendations[]` se rechaza**
(renombrar rompe el cliente).

| Estado | `hasEnoughData` | `items` |
|---|---|---|
| Historial insuficiente | `false` | `[]` |
| Sin patrones | `true` | `[]` |
| Con recomendaciones | `true` | `items.length > 0` |

Diseño completo: `sdd/_control/H08/ML_E055_CONTRATO_Y_DISENO.md` (K-Means, features, semilla,
artefacto JSON, inferencia Go, explicabilidad determinista, prohibición de cifras inventadas).

## 8. E058 — Preferencias de notificación

```
PUT /notification-preferences   (Bearer)
{
 "pushEnabled":bool*, "emailEnabled":bool*, "reminderEnabled":bool*,
 "budgetAlerts":bool=true, "goalUpdates":bool=true, "recurringReminders":bool=true,
 "streakReminders":bool=false,
 "quietHours":{"enabled":bool,"start":"HH:MM","end":"HH:MM"} | ausente,
 "timeZone":"America/Mexico_City" (IANA), "version":int (opcional)
}
200 → mismo objeto + version + updatedAt
```
`start != end`; `HH:MM` válido; `timeZone` IANA; `version` desalineada ⇒ `409`.
`GET` devuelve los defectos con `version: 0` en lugar de `404`. Enmienda **A3**.
`E019`/`E021` **leen** estas preferencias, no las duplican.

## 9. E075 — Voz

`{text, locale}` → `{intent, requiresConfirmation, parsedAmount, currency}`.
`transcript` se rechaza. Reconocimiento en Android. **E075 no crea movimientos**: tras confirmar,
el cliente llama a E028. `text` vacío o > 512 ⇒ `400`; intención no reconocida ⇒ `422`.

## 10. E043 / E044 / E047 — reparto y reversión (verificado)

| Necesidad | Contrato vigente | ¿Falta? |
|---|---|---|
| Importe solicitado | `amount` | No |
| Reparto | `allocations[]{goalId, amount}` | No |
| Sin asignar (preview) | `remaining` (E043) | No |
| Aplicado | `amount` + `allocations[]`; si no cabe, `422` (sin aplicación parcial) | No |
| Id de aportación | `id` (E044/E047) | No |
| Reversión | `status: active\|reversed` + `version` + recarga de E038/E040 | No |
| Idempotencia | `Idempotency-Key` obligatoria en E044/E047 | No |
| Concurrencia | `version` + condición | No |

`mode`, `splitMode`, `goalIds[]` son formas de entrada alternativas: el cliente puede expresar el
mismo reparto con `allocations[]`. **`OPCIONAL` con motivo, no por omisión.**

## 11. Dinero (transversal)

String decimal en el contrato, **enteros de centavos** en Go; `MXN`; redondeo HALF_UP documentado;
`Idempotency-Key` en mutaciones; `TransactWriteItems` + versión; reversión que netea; 100 acciones
máximo por transacción; `Scan` prohibido; aislamiento por `PK`.

## 12. Persistencia — A1

`SESSIONHASH#digest` → `USER#<id>/SESSION#<id>` + `REFRESH#<sha256>/SESSION` en **E005–E011**
(28 referencias, 4 por spec). Enmienda editorial, sin cambio de tabla ni código. No toca E003/E004.

Factibilidad verificada sin índices nuevos: `Query` por prefijo, `GSI1` existente, `GetItem` por
clave, `TransactWriteItems` con versión, TTL `expiresAt`.

## 13. Enmiendas

| # | Alcance | Archivos | Aplicada |
|---|---|---|---|
| A1 | Persistencia E005–E011 | 7× `spec.md`/`plan.md`/`tasks.md`/`checklist.md` | **NO** |
| A2 | `hasEnoughData` E055 | OpenAPI YAML+JSON, specs E055 | **NO** |
| A3 | Contrato E058 (§8) | OpenAPI YAML+JSON, specs E058/E057 | **NO** |
| A4 | Registrar 5 decisiones HTTP | `DECISIONES_CONTRATO_*.json` | **NO** |
| A5 | `enoughData`+`minimumSamples` E052 | OpenAPI YAML+JSON, specs E052 | **NO** |
| A6 | Época de revocación (código E002/E003/middleware/E006) | código Go | **NO** — RFC y Gate propios |
| A7 | `openingSaved` E040 (+E039) | OpenAPI YAML+JSON, specs E039/E040 | **NO** |

## 14. Gates

| Gate | Autoriza | Estado |
|---|---|---|
| `APRUEBO_RFC_016_CONTRATOS_E005_E075` | clasificación, decisiones y alcance de A1–A7 | **PENDIENTE** |
| `APRUEBO_ENMIENDA_A1_PERSISTENCIA` | editar 7 specs | **PENDIENTE** |
| `APRUEBO_ENMIENDA_A2_E055` | editar E055 + OpenAPI | **PENDIENTE** |
| `APRUEBO_ENMIENDA_A3_E058` | editar E058/E057 + OpenAPI | **PENDIENTE** |
| `APRUEBO_ENMIENDA_A5_E052` | editar E052 + OpenAPI | **PENDIENTE** |
| `APRUEBO_ENMIENDA_A7_E040` | editar E039/E040 + OpenAPI | **PENDIENTE** |
| `APRUEBO_RFC_A6_EPOCA_SESION` | modificar código compartido | **PENDIENTE** |
| `APRUEBO_MERGE_PR_65_SHA_<SHA40>` | merge | **PENDIENTE** |

A4 no edita contrato: se aplica con la aprobación de esta RFC.

## 15. Criterio de congelación

`CONTRACT_FROZEN=VERIFIED` por endpoint exige: contrato completo, seguridad, validaciones, reglas,
persistencia factible, concurrencia e idempotencia, dependencias, errores, pruebas de aceptación,
**cero ESENCIAL/COMPATIBILIDAD pendientes**, enmiendas aplicadas y CI documental verde.
Los `PENDIENTE_DE_DECISION` opcionales deben cerrarse en el Gate de ese endpoint.

Estados separados y no intercambiables: `CONTRACT_FROZEN` · `AWS_READY` · `IMPLEMENTED` ·
`MERGED` · `DEPLOYED` · `AWS_VERIFIED` · `USER_ACCEPTED`.

## 16. Riesgos

| Riesgo | Control |
|---|---|
| Duplicado de correo por caída tras aceptación de SES | Se documenta at-least-once; enlace de un solo uso; no se promete unicidad |
| Enumeración por latencia | Suelo + jitter + prueba estadística; límites por IP y correo |
| Worker sin líder (Redis caído) | Fail-safe: no envía, no pierde; admisión sigue en `202` |
| A6 no aprobado | E006 queda con la alternativa de 30 sesiones y se declara el límite |
| 319 opcionales pendientes | Registrados por endpoint; se cierran en su Gate |
| Cambio silencioso futuro | `DECISIONES_CONTRATO_*.json` + CI |

---

# 17. Versión 4.0.0 — decisiones finales y correcciones de E005/E006

> **Sustituye a §2 (cifras), §4.3–§4.4 (E005) y §5 (E006).** El resto de v3 sigue vigente.
> **`PENDIENTE_DE_DECISION = 0`**: no queda ninguna diferencia sin decisión adoptada.

## 17.1 Cifras finales (sustituyen a §2)

```
71 operaciones · 812 diferencias · 0 sin clasificar · 0 pendientes de decisión
ESENCIAL=2 · COMPATIBILIDAD=206 · OPCIONAL=597 · OBSOLETA=7
```

**ESENCIAL (2)** — únicos datos que solo el servidor tiene y el contrato no expone:

| Endpoint | Campo | Por qué |
|---|---|---|
| E040 | `openingSaved` | E039 acepta `initialSavedAmount` y no lo devuelve; sin él `contributedNet` es incalculable (enmienda A7) |
| E055 | `hasEnoughData` | sin él, `items: []` no distingue «sin historial» de «sin patrones» (enmienda A2) |

Los otros siete candidatos que se habían marcado ESENCIALES quedaron **resueltos por el contrato
real** y pasan a CONSERVAR: `email` (E010 exige Bearer), `verificationToken` (E011 ya exige `token`),
`deviceId` y `deviceRegistrationId` (≡ `PushDevice.id`), `platform` (≡ `provider`),
`relatedEntityId` (el recordatorio no tiene vínculo) y `updatedCount` (un `202` no devuelve conteo).

Instrumentos sobre las 206 diferencias de compatibilidad: A8=40 entradas/31 campos · A3=26 · A5=2 ·
A4=5 · CONSERVAR=33 · CALCULAR=66 · DESCARTAR=34. **OPCIONAL=597**, todas con derivación.

## 17.2 E005 — correcciones de diseño

**A. Sin `ConditionCheck` sobre un PROFILE inexistente.** La transacción de admisión pasa a
**2 acciones sobre 2 ítems** y es idéntica para cuenta existente e inexistente:

| # | Acción | Ítem | Nota |
|---|---|---|---|
| 1 | `Put` | `RESET#<sha256(token)>` / `RESET` `{userId|null, expiresAt, used:false}` | cuenta inexistente ⇒ `userId=null` |
| 2 | `Put` | `QUEUE#PWRESET` / `<dueAt ISO>#<ulid>` `{userId|null, destinatario|null, tokenCifrado|null, estado:"PENDIENTE", intentos:0, fence:0}` | misma forma en ambos casos |

El perfil se lee **antes** con `ConsistentRead`; si no existe, la transacción se ejecuta igual sin
`destinatario` ni `tokenCifrado`. No se crea ningún perfil falso. **No hay condición que pueda
fallar solo para correos inexistentes.**

**B. Descubrimiento de trabajos sin `Scan`.** Los ítems viven en la partición
`QUEUE#PWRESET` con `SK = <dueAt>#<ulid>`. El worker consulta:

```
Query TABLE
  KeyConditionExpression: PK = :q AND SK <= :now
  Limit: 25
  ExclusiveStartKey: paginación
```
`SK <= :now` es un rango ordenado por fecha ISO-8601: devuelve exactamente los vencidos, sin `Scan`
y sin conocer claves de usuario. La ventana está acotada por `Limit` y por el cursor.

> **Dependencia real:** el rol runtime tiene `LeadingKeys` limitado a `USER#*`, `REFRESH#*` y
> `SESSION#*`. Se necesita ampliar a `QUEUE#*` y `RESET#*` → **A10 (IAM)**, con Gate propio.
> Sin A10 el worker no puede leer la cola: se declara explícitamente, no se asume.

**C. Identidad única del token.** Un solo ítem para ambos endpoints:

| Fase | Operación |
|---|---|
| Emisión (E005) | `PutItem RESET#<sha256(T)>` con `userId`, `expiresAt`, `used=false` |
| Consulta (E006) | `GetItem RESET#<sha256(T)>` → `userId` (público, sin sesión, sin `Scan`) |
| Validación | `expiresAt > now` **explícita** + `used=false` + hash coincidente por construcción |
| Consumo | `UpdateItem` condicional `SET used=true` (acción 2 de la transacción de E006) |
| Expiración | TTL del ítem **y** comprobación funcional de `expiresAt` (el TTL no sustituye la validación) |
| Reintentos | reenvían el **mismo** token; no se emite uno nuevo |
| Revocación | `DeleteItem` del ítem en el borrado de cuenta |

Se elimina `PWRESET#<hmacEmail>`: **una sola identidad** (`RESET#<sha256(T)>`) compartida por E005 y
E006. Requiere `RESET#*` en A10.

**D. Expiración frente a backoff.** Antes de enviar, el worker comprueba `expiresAt > now`. Si el
backoff supera la vigencia, el trabajo pasa a `EXPIRADO` **sin enviar** y se borra `tokenCifrado`.
Nunca se emite un enlace ya caducado. El backoff se trunca en `expiresAt`.

**E. Clave de cifrado: no existe hoy.** El bundle RFC-004 contiene `APP_ENV`, `AWS_REGION`,
`DYNAMODB_TABLE`, `REDIS_ADDR`, `REDIS_PASSWORD`, `REDIS_TLS`, `REDIS_USER`, `JWT_PRIVATE_KEY_PEM_B64`,
`JWT_KEY_ID` y `LOGIN_RATE_LIMIT_HMAC_KEY_B64`. **`PASSWORD_RESET_SEAL_KEY_B64` no está** → alta
como **A9**: 32 bytes CSPRNG, custodia por el procedimiento RFC-004 (escritura condicional al
bundle, SSE, versión), rotación documentada (re-cifrado o descarte de trabajos pendientes), acceso
solo al rol runtime. Sin A9 el worker **no puede descifrar**: se declara como bloqueo.

**F. Lease + fencing.** El liderazgo Redis **no** sustituye la reclamación atómica:

| Caso | Comportamiento |
|---|---|
| Dos workers | solo el líder reclama; si ambos intentan, la condición `estado=:pendiente` deja uno fuera |
| Caída del líder | el lease expira en ≤ 15 s; el otro nodo toma el liderazgo |
| Expiración del lease de un trabajo | el ítem vuelve a `:pendiente`; al reclamarlo se **incrementa `fence`** |
| Worker anterior que sigue vivo | su `Update` final lleva condición `owner=:me AND fence=:f`; si perdió el lease, **falla** y no corrompe el estado |
| Fallo de DynamoDB | el trabajo no se marca; queda `:pendiente` y se reintenta |
| SES acepta y la EC2 se reinicia | el ítem sigue `:enviando`; al expirar el lease se reenvía el **mismo** enlace (at-least-once) |

**G. Sin infraestructura nueva.** No se añade NAT, SQS, Lambda ni instancias. Las dos únicas
dependencias son **A9** (secreto) y **A10** (IAM), ambas con Gate.

## 17.3 E006 — contrato y transacción definitivos

```
POST /api/v1/auth/password/reset    (pública)
403 → n/a · Request { token, newPassword } · Response 200 Accepted { status, requestId }
```

Secuencia: `GetItem RESET#<sha256(token)>` (→ `userId`, `expiresAt`, `used`) y después:

| # | Acción | Ítem | Expresión |
|---|---|---|---|
| 1 | `Update` | `USER#<id>` / `PROFILE` | `SET passwordHash=:h, passwordChangedAt=:t, sessionEpoch=if_not_exists(sessionEpoch,:zero)+:one ADD version :one` · condición `attribute_exists(PK) AND version=:v` |
| 2 | `Update` | `RESET#<sha256(token)>` / `RESET` | `SET used=:true, usedAt=:t` · condición `attribute_not_exists(used) OR used=:false AND expiresAt > :now` |

Dos ítems distintos, una acción por ítem: **válido** para `TransactWriteItems`.

**A6 — diseño completo (cambio compartido, no se ejecuta ahora):**

| Componente | Cambio |
|---|---|
| E002 login | persistir `sessionEpoch` en el ítem de sesión y emitir claim `epo` en el JWT |
| E003 refresh | propagar `epo` de la sesión al nuevo access token |
| Middleware | tras validar la firma, `GetItem PROFILE` (`ConsistentRead`) y comparar `epo` con `sessionEpoch`; desalineado ⇒ `401` |
| E006 | ya incrementa la época en su transacción; añade `DEL` de la caché de época si se usa |
| Compatibilidad | JWT o sesión sin `epo` ⇒ `0`; perfil sin `sessionEpoch` ⇒ `0`; tras el primer cambio, `1` invalida todo lo anterior |
| Coste | una lectura extra por petición autenticada; alternativa con caché Redis TTL 30 s + `DEL` en E006, con ventana declarada |

**Revocación real, sin límite artificial:** no se adopta la variante de «30 sesiones». La época
invalida **todas** las sesiones anteriores sea cual sea su número. Casos cubiertos: 1, 100 y >100
sesiones; reset concurrente; token usado/expirado; sesión antigua tras el cambio; login nuevo;
refresh posterior (rechazado); logout posterior (idempotente).

## 17.4 Enmiendas definitivas

| # | Alcance | Archivos | Bloquea congelar |
|---|---|---|---|
| A1 | Persistencia E005–E011 (28 refs) | 7 specs × 4 docs | Sí |
| A2 | E055 `hasEnoughData` | OpenAPI + specs E055 | Sí |
| A3 | E057/E058 preferencias (§8) | OpenAPI + specs | Sí |
| A4 | 5 decisiones HTTP (registro) | `DECISIONES_CONTRATO_*.json` | Sí |
| A5 | E052 `enoughData`+`minimumSamples` | OpenAPI + specs E052 | Sí |
| A7 | E039/E040 `openingSaved` | OpenAPI + specs | Sí |
| **A8** | **37 campos depurados** (§17.6) | OpenAPI + specs afectadas | Sí |
| **A9** | **secreto `PASSWORD_RESET_SEAL_KEY_B64`** | bundle RFC-004 | No (despliegue E005) |
| **A10** | **IAM `QUEUE#*` y `RESET#*`** | módulo IAM (I-060) | No (despliegue E005) |
| A6 | época de revocación (código) | E002/E003/middleware/E006 | No (implementación E006) |

## 17.5 Estado de los contratos

`DECISION_READY = VERIFIED` para los 71 (todas las diferencias tienen decisión).
`CONTRACT_FROZEN = NOT_VERIFIED` mientras A1–A5, A7 y A8 no estén **aprobadas y aplicadas**.
`AWS_READY = NOT_READY` (SES sin remitente, sin preflight y con coste de endpoint sin aprobar).
`IMPLEMENTED / MERGED / DEPLOYED / AWS_VERIFIED / USER_ACCEPTED = NOT_RUN`.

## 17.7 Respuestas 204 (E031, E042) — resuelto sin añadir cuerpo

**Hecho verificado:** `nexum_e031` y `nexum_e042` declaran `204` con `content: {}`. Añadir
`affectedGoalId`, `reversedContributionId`, `releasedAmount`, `reversedAllocations` o
`affectedPeriods` a su respuesta **contradice HTTP**. Se retiran de A8.

**Decisión (opción A):** se conserva `204` sin cuerpo y los efectos se consultan con operaciones
existentes, en este orden:

| Efecto tras | Se consulta con | Qué devuelve |
|---|---|---|
| E031 borrar movimiento | `GET /movements/{movementId}` (E029) → `404` | confirma el borrado |
| E031 efecto en meta | `GET /goals` (E038) / `GET /goals/{id}` (E040) | `savedAmount` ya recalculado |
| E031 efecto en aportación | `GET /goal-contributions` (E045) | aportaciones vigentes |
| E042 borrar meta | `GET /goals/{id}` (E042→404) | confirma el borrado |
| E042 importe liberado | `GET /financial-summary` (E026) | `availableEstimated` recalculado |
| E042 aportaciones revertidas | `GET /goal-contributions` (E045) + `GET .../{id}` (E046) | `status: reversed` |

**Invariante automatizada:** el clasificador emite `A8_SOBRE_RESPUESTA_SIN_CUERPO` si algún campo
A8 se propone sobre una respuesta sin `application/json`. Hoy: **0 problemas**.

## 17.8 Filtros y paginación — verificado contra OpenAPI

Filtrar una página en Android **no** equivale a filtrar el conjunto. Revisión con los parámetros
reales del contrato:

| Operación | Filtros que YA existen | Decisión |
|---|---|---|
| E027 movimientos | `limit`, `cursor`, `type`, `from`, `to` | conservar; nada que añadir |
| E032 recurrentes | `limit`, `cursor`, `active` | **falta `type`** → A8 |
| E038 metas | `limit`, `cursor`, `status` | conservar |
| E045 aportaciones | `limit`, `cursor`, `goalId`, `from`, `to` | **falta `status`** → A8 |
| E053 categorías | `period` | `limit` no se añade: el servidor acota el top-N |
| E061 notificaciones | `limit`, `cursor`, `isRead` | `unreadOnly` ≡ `isRead` → conservar; **falta `type`** → A8 |
| E064 recordatorios | `limit`, `cursor`, `active` | `activeOnly` ≡ `active` → conservar; **`type` no se añade**: el recordatorio no tiene tipo en el modelo |
| E071 tips | — | catálogo pequeño; el servidor acota; no se añade |

**Contadores sobre listas paginadas:** `unreadCount` (E061, E062) y `totals.{activeCount,saved}`
(E038) **no** son calculables desde una página → A8. `updatedCount` de E063 **no** entra: la operación
es `202` y no puede devolver el conteo final; Android lo obtiene con `GET /notifications` (E061) —
`unreadCount` — o filtrando por `isRead`.

## 17.9 A8 definitiva — 31 incorporaciones con schema

> Sustituye por completo a la tabla anterior de A8. Se eliminan las duplicidades detectadas
> verificando el contrato real: `email` (E010 exige Bearer), `verificationToken` (E011 ya exige
> `token`), `deviceId`/`deviceRegistrationId` (≡ `PushDevice.id`), `platform` (≡ `provider`),
> `triggerAt`/`timeZone` (≡ `dueAt`/`timezone`), `relatedEntityId` y `E064:type` (no existen en el
> modelo), `updatedCount` (un 202 no puede devolver el conteo final).

Tipos abreviados: `s` string, `b` boolean, `i` integer, `dt` date-time, `d` date, `u` uuid, `m` dinero.
`Req` = requerido en el schema. Todos son aditivos y no rompen clientes que ignoren campos nuevos.

| # | Endpoint | Dónde | Campo | Tipo | Req | Null | Restricciones / enum | Pantalla | Enmienda |
|---|---|---|---|---|---|---|---|---|---|
| 1 | E014 `DELETE /users/me` | 202 body | `deletionRequestId` | u | sí | no | uuid v7 | Cuenta | A8 |
| 2 | E015 `GET /privacy/notices/current` | 200 body | `content` | s | sí | no | 1..20000 | Aviso | A8 |
| 3 | E015 idem | 200 body | `requiresAcceptance` | b | sí | no | — | Aviso | A8 |
| 4 | E018 `GET /users/me/onboarding` | 200 body | `microphone` | b | sí | no | — | Onboarding | A8 |
| 5 | E018 idem | 200 body | `notifications` | b | sí | no | — | Onboarding | A8 |
| 6 | E018 idem | 200 body | `configured` | b | sí | no | — | Onboarding | A8 |
| 7 | E018 idem | 200 body | `completedAt` | dt | sí | sí | null si no completado | Onboarding | A8 |
| 8 | E019 `PATCH /users/me/onboarding` | request | `microphone` | b | no | no | — | Onboarding | A8 |
| 9 | E019 idem | request | `notifications` | b | no | no | — | Onboarding | A8 |
| 10 | E019 idem | request | `configured` | b | no | no | — | Onboarding | A8 |
| 11 | E019 idem | 200 body | `completedAt` | dt | sí | sí | — | Onboarding | A8 |
| 12 | E029 `GET /movements/{id}` | 200 body | `contributionId` | u | no | sí | uuid v7 | Movimiento | A8 |
| 13 | E029 idem | 200 body | `recurringPaymentId` | u | no | sí | uuid v7 | Movimiento | A8 |
| 14 | E032 `GET /recurring-payments` | query | `type` | s | no | — | enum `income|expense|saving` | Recurrentes | A8 |
| 15 | E033 `POST /recurring-payments` | request | `type` | s | sí | no | mismo enum | Crear recurrente | A8 |
| 16 | E033 idem | request | `startDate` | d | sí | no | ISO `YYYY-MM-DD` | Crear recurrente | A8 |
| 17 | E033 idem | request | `dueDay` | i | no | sí | 1..28 | Crear recurrente | A8 |
| 18 | E034 `GET /recurring-payments/{id}` | 200 body | `startDate` | d | sí | no | ISO | Detalle recurrente | A8 |
| 19 | E035 `PATCH /recurring-payments/{id}` | request | `type` | s | no | no | mismo enum | Editar recurrente | A8 |
| 20 | E035 idem | request | `startDate` | d | no | no | ISO | Editar recurrente | A8 |
| 21 | E035 idem | request | `dueDay` | i | no | sí | 1..28 | Editar recurrente | A8 |
| 22 | E038 `GET /goals` | 200 body `totals` | `activeCount` | i | sí | no | ≥0 | Lista de metas | A8 |
| 23 | E038 idem | 200 body `totals` | `saved` | m | sí | no | 2 decimales, ≥0 | Lista de metas | A8 |
| 24 | E045 `GET /goal-contributions` | query | `status` | s | no | — | enum `active|reversed` | Aportaciones | A8 |
| 25 | E061 `GET /notifications` | query | `type` | s | no | — | enum `budget|goal|recurring|streak|system` | Bandeja | A8 |
| 26 | E061 idem | 200 body | `unreadCount` | i | sí | no | ≥0, total global | Bandeja | A8 |
| 27 | E062 `PATCH /notifications/{id}/read` | 200 body | `unreadCount` | i | sí | no | ≥0, total global | Bandeja | A8 |
| 28 | E065 `POST /reminders` | request | `frequency` | s | no | sí | enum `daily|weekly|monthly`; ausente = disparo único | Crear recordatorio | A8 |
| 29 | E066 `PATCH /reminders/{id}` | request | `frequency` | s | no | sí | mismo enum | Editar recordatorio | A8 |
| 30 | E068 `GET /users/me/preferences` | 200 body | `timeZone` | s | sí | no | IANA (`America/*`) | Preferencias | A8 |
| 31 | E069 `PATCH /users/me/preferences` | request | `timeZone` | s | no | no | IANA | Preferencias | A8 |

> **Ida y vuelta:** el destino exacto de cada campo escribible (request, respuesta compartida, lista
> y detalle) está en §17.11.

**Ejemplo válido (E065):**
```json
{"title":"Pagar renta","dueAt":"2026-12-01T18:00:00Z","timezone":"America/Mexico_City",
 "frequency":"monthly"}
```

**No se cuentan en A8:** `hasEnoughData` (A2), preferencias y `quietHours` (A3),
`enoughData`/`minimumSamples` (A5), `openingSaved` (A7). **Fuera de A8 por HTTP:** los efectos de
E031/E042 (§17.7). **Conservados** por existir ya en el contrato: `email`, `verificationToken`,
`deviceId`, `deviceRegistrationId`, `platform`, `triggerAt`, `timeZone` de recordatorios y
`updatedCount`.

**Correcciones de conteo:** 31 campos nuevos únicos · 31 incorporaciones (una por campo y endpoint) ·
2 ESENCIALES ajenos a A8 (E040, E055) · 5 campos de E031/E042 fuera por 204 · 26 de E057/E058 en A3 ·
2 de E052 en A5 · 1 de E055 en A2 · 1 de E040 en A7.

## 17.11 Ida y vuelta — todo campo escribible se puede volver a leer

**Regla:** ningún campo que Android pueda crear o modificar puede quedar solo en el request.

**Componentes compartidos verificados en el contrato:** `RecurringPayment` (usado por E032 lista,
E033 response, E034 detalle y E035 response) y `Reminder` (E064 lista, E065 response, E066 response).
Añadir allí el campo lo hace visible en LIST y DETAIL sin duplicar modelos.

| Campo | Request | Respuesta compartida | Lista | Detalle / PATCH | Regla |
|---|---|---|---|---|---|
| `type` | `RecurringCreate` | `RecurringPayment` | E032 (item) + filtro `type` | E034 / `RecurringPatch` | enum `income|expense|saving` idéntico |
| `startDate` | `RecurringCreate` | `RecurringPayment` | E032 | E034 / `RecurringPatch` | ISO `YYYY-MM-DD` |
| `dueDay` | `RecurringCreate` (opt) | `RecurringPayment` | E032 | E034 / `RecurringPatch` (opt) | entero 1..28, null = sin día fijo |
| `frequency` (recordatorio) | `ReminderCreate` (opt) | `Reminder` | E064 | E066 / `ReminderPatch` (opt) | `daily|weekly|monthly`; **ausente ≠ null**: ausente = disparo único, `null` prohibido |
| `microphone`, `notifications`, `configured` | `OnboardingPatch` | `Onboarding` | — | E018 GET | booleanos |
| `completedAt` | — | `Onboarding` | — | E018/E019 | date-time, null si no completado |
| `timeZone` | `PreferencesPatch` (E069) | `Preferences` | — | E068 GET | IANA |

**Semántica de `frequency` (explícita):**

| Estado | Representación | Efecto en el servidor |
|---|---|---|
| Disparo único | campo **ausente** | `dueAt` se dispara una vez y el recordatorio queda `active=false` |
| Recurrente | `daily`/`weekly`/`monthly` | se recalcula `dueAt` tras cada disparo |
| Pasar a único | `frequency: null` en PATCH | se cancela la recurrencia; el próximo `dueAt` es el último |
| Editar frecuencia | valor del enum | se reprograma desde `dueAt` vigente |

`null` **solo** es válido en PATCH para cancelar; en CREATE la ausencia ya significa disparo único.
Así «ausente» y «null» no se confunden.

**Campos de solo lectura (no aplican ida y vuelta):** `content`, `requiresAcceptance`,
`deletionRequestId`, `contributionId`, `recurringPaymentId`, `totals.activeCount`, `totals.saved`,
`unreadCount` y los filtros `status`/`type`.

**Invariante automatizada:** `validar_ida_vuelta()` exige que cada campo escribible tenga al menos
un destino de request y uno de respuesta declarados. Resultado actual: **0 problemas**.

## 17.10 Frases de aprobación

| Instrumento | Frase |
|---|---|
| RFC completa | `APRUEBO_RFC_016_CONTRATOS_E005_E075` |
| Persistencia | `APRUEBO_ENMIENDA_A1_PERSISTENCIA` |
| E055 | `APRUEBO_ENMIENDA_A2_E055` |
| E057/E058 | `APRUEBO_ENMIENDA_A3_E058` |
| E052 | `APRUEBO_ENMIENDA_A5_E052` |
| E039/E040 | `APRUEBO_ENMIENDA_A7_E040` |
| A8 (38) | `APRUEBO_ENMIENDA_A8_ADITIVOS` |
| Merge | `APRUEBO_MERGE_PR_65_SHA_<SHA40>` |
