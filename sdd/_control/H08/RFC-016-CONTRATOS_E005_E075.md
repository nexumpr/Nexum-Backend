# RFC-016 — Cierre contractual E005–E075 (versión normativa v3)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/RFC-016-CONTRATOS_E005_E075.md` |
| Versión | **3.0.0 — normativa única** (sustituye a v1 y v2) |
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
