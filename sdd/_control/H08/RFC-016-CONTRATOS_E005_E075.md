# RFC-016 — Cierre contractual E005–E075 (SDD V6.0.2)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/RFC-016-CONTRATOS_E005_E075.md` |
| Estado | **PROPUESTO — NO APROBADO — NO APLICADO** |
| Fecha | 2026-11-16 |
| Baseline | `main` `8bc2548` · rama `chore/pre-e005-contratos-permisos` · PR #65 |
| Alcance | Reconciliación transversal de los 71 contratos E005–E075 |
| Autoriza | **Nada por sí mismo.** No aplica enmiendas, no habilita implementación |
| Aprobación | `APRUEBO_RFC_016_CONTRATOS_E005_E075` |

---

## 1. Autoridad y regla de decisión

1. **OpenAPI 3.1 aprobado (H06) es la autoridad de transporte.** El YAML y su espejo JSON
   mandan sobre cualquier otra fuente.
2. **La auditoría V6.0.2 establece necesidades funcionales del producto**, no contrato.
3. Cuando discrepan, se decide y se registra; **nunca** se sustituye un contrato aprobado en
   silencio.
4. La app Android **no está publicada**: puede adaptarse al contrato aprobado. Por tanto
   «compatibilidad con Android» significa *no romper lo ya implementado* (E001–E004) y *no
   imponer al cliente una forma que el contrato ya resuelve de otro modo*.

## 2. Método y resultado

Herramientas versionadas y deterministas (sin AWS, sin red):

| Herramienta | Función |
|---|---|
| `herramientas/verificar_preparacion_e005.py` | integridad matriz↔OpenAPI, detección, decisiones, deuda documental |
| `herramientas/clasificar_contratos_e005.py` | clasifica **todas** las diferencias y emite `CLASIFICACION_E005_E075.json` |
| `sdd/_control/H08/DECISIONES_CONTRATO_E005_E075.json` | valor vigente/objetivo por diferencia; detecta cambios sin Gate |

Resultado real:

```
71 operaciones · 812 diferencias · 0 sin clasificar
ESENCIAL=1 · COMPATIBILIDAD=20 · OPCIONAL=784 · OBSOLETA=7
```

**Ninguna diferencia queda sin clasificar.** `OPCIONAL` **no** significa resuelto: significa
**POSPUESTO y registrado**, reversible por Gate. Las 784 opcionales no bloquean la congelación;
las 21 esenciales/compatibilidad **sí**.

Distribución de las diferencias materiales:

| Endpoint | ESENCIAL | COMPATIBILIDAD | Motivo |
|---|---|---|---|
| E017 | 0 | 1 | HTTP `201` vs `200` |
| E031 | 0 | 1 | HTTP `204` vs `200` |
| E037 | 0 | 1 | HTTP `200` vs `201` |
| E042 | 0 | 1 | HTTP `204` vs `200` |
| E063 | 0 | 1 | HTTP `202` vs `200` |
| E006 | 0 | 1 | `resetToken` vs `token` |
| E055 | **1** | 1 | `hasEnoughData` (ESENCIAL) + `recommendations[]` vs `items[]` |
| E058 | 0 | 12 | preferencias granulares vs 3 booleanos |
| E075 | 0 | 1 | `transcript` vs `text` |
| Otros (62) | 0 | 0 | solo opcionales pospuestos |

## 3. Códigos de éxito — decisión propuesta

| ID | Operación | Vigente | Decisión | Motivo |
|---|---|---|---|---|
| E017 | `POST /users/me/consents` | `201` | **Conservar 201** | crea un consentimiento: recurso nuevo |
| E031 | `DELETE /movements/{id}` | `204` | **Conservar 204** | borrado idempotente sin cuerpo |
| E037 | `POST /recurring-payments/{id}/payments` | `200` | **Conservar 200** | ya devuelve el movimiento creado con `id`; `201` sería correcto pero el cambio no aporta y rompe al cliente |
| E042 | `DELETE /goals/{id}` | `204` | **Conservar 204** | borrado idempotente sin cuerpo |
| E063 | `POST /notifications/read-all` | `202` | **Conservar 202** | no se ha demostrado procesamiento síncrono; si se demuestra, `200` |

Ninguna de estas cinco requiere enmienda: **la decisión es conservar el contrato vigente**.
Se registran como `COMPATIBILIDAD` porque un cambio futuro sería una ruptura.

## 4. E005 — `POST /api/v1/auth/password/forgot` (SES)

### 4.1 Contrato (definitivo)

```
Request  EmailRequest { email: string (requerido, format email, maxLength 254) }
Response 202 Accepted { status: "accepted", requestId: uuid }
Errores  400 409 422 429 500 503 (envelope común con traceId)
```

**No se añade `locale` ni se sustituye `Accepted`.** `locale` queda **POSPUESTO** (el idioma puede
derivarse del perfil); `message`/`retryAfterSeconds` quedan **POSPUESTOS**. Cambiar la respuesta
rompería el patrón `requestId` y la no-enumeración.

### 4.2 Semántica exacta de `202`

`202` significa **«la solicitud fue aceptada para su procesamiento»**, **no** «el correo fue
entregado». El mensaje al usuario debe decir «si el correo existe, recibirás instrucciones»,
nunca «correo enviado». Prohibido registrar o exponer el resultado del envío al cliente.

### 4.3 Reglas de comportamiento

| Aspecto | Regla |
|---|---|
| No enumeración | Misma respuesta, mismo tiempo y mismo código exista o no la cuenta |
| Token | 32 bytes CSPRNG, `base64url`; se persiste **solo** su SHA-256 con TTL |
| Uso único | consumo con escritura condicional (`attribute_not_exists`); segundo uso ⇒ `409` genérico |
| Expiración | 30 minutos (TTL físico `expiresAt` + comprobación funcional) |
| Rate limit | Redis distribuido: 3/hora por correo (HMAC) y 10/hora por IP; fail-closed en este endpoint |
| Idempotencia | no aplica `Idempotency-Key` (no crea recurso); el reenvío reutiliza el token vigente o lo rota sin filtrar estado |
| Fallo DynamoDB | `503` genérico, sin detalle interno |
| Fallo SES | se registra métrica y **se responde igualmente `202`** (no se filtra), con reintento acotado |
| Cuenta eliminada | misma respuesta; no se envía nada |
| Enlace | `BASE_URL` + token de un uso; sin correo ni `userId` en la URL |

### 4.4 Infraestructura SES — requisitos (no ejecutados)

| Requisito | Estado | Bloquea contrato | Bloquea despliegue |
|---|---|---|---|
| Identidad SES verificada (dominio o correo) | **PENDIENTE** | No | **Sí** |
| Salida del sandbox de SES (producción) | **PENDIENTE** (aprobación de AWS) | No | **Sí** |
| Permiso IAM `ses:SendEmail` acotado a la identidad | **NO EXISTE** (I-060) | No | **Sí** |
| Conectividad desde subred privada a la API SES | **NO DEMOSTRADA** | No | **Sí** |
| Coste dentro de créditos | **NO VERIFICADO** | No | **Sí** |
| Procedimiento de recuperación (rebote, queja) | Documentado en el Gate de implementación | No | No |

**`CONTRACT_READY` sí · `AWS_READY` no.** El contrato puede congelarse; el endpoint no puede
declararse publicado hasta cerrar la tabla anterior. **No se asume** que exista VPC endpoint de
SES ni que AWS haya aprobado producción: ambas cosas se verifican en su Gate con evidencia.

## 5. E006 — `POST /api/v1/auth/password/reset`

```
Request  PasswordResetRequest { token: string, newPassword: string (12..128) }
Response 200 Accepted { status, requestId }
```

| Elemento | Decisión |
|---|---|
| `token` vs `resetToken` | **Conservar `token`**. Renombrar rompe el contrato (`COMPATIBILIDAD`) |
| Respuesta | **Conservar `Accepted`**. `revokedSessions` queda **POSPUESTO** |
| Validación | token vigente, no usado, hash coincidente; `400` genérico si no |
| Uso único | consumo condicional atómico |
| Revocación | **se revocan todas las sesiones e identificadores `REFRESH#` del usuario** (R-033) en la misma transacción |
| Concurrencia | dos reset simultáneos: uno gana; el otro `409` sin filtrar |
| Reutilización | el mismo token no puede restablecer dos veces |

## 6. E055 — Recomendaciones con K-Means

Diseño completo en **`sdd/_control/H08/ML_E055_CONTRATO_Y_DISENO.md`**.

Decisión contractual propuesta:

| Elemento | Decisión |
|---|---|
| `items[]` + `nextCursor` | **Conservar** (paginación ya aprobada) |
| `recommendations[]` | **Rechazado** (renombrar el contenedor rompe el cliente) |
| `hasEnoughData` (boolean, requerido) | **Añadir** — único campo ESENCIAL del cierre |
| `items[].modelVersion` | **Ya existe**: acredita versión real del modelo |
| `items[].category`, `items[].evidence`, `items[].confidence` | **POSPUESTOS** |

K-Means **agrupa patrones**; la recomendación se redacta en una capa determinista sobre
centroides y estadísticas verificables. Prohibido inventar porcentajes o ahorros.

`E052` (insights) mantiene su responsabilidad: describe ventanas y suficiencia de datos; **no**
absorbe el contrato ML ni se convierte en recomendaciones.

## 7. E075 — Voz

```
Request  { text: string, locale: string }
Response 200 { intent, requiresConfirmation, parsedAmount, currency }
```

| Elemento | Decisión |
|---|---|
| `transcript` vs `text` | **Conservar `text`** (`COMPATIBILIDAD`) |
| Reconocimiento | **en Android**, con sus permisos y APIs. No requiere Amazon Transcribe |
| `confidence` del reconocimiento | No viaja al backend; queda **POSPUESTO** |
| Confirmación | `requiresConfirmation=true` siempre que `parsedAmount` exista; el cliente confirma antes de crear |
| Creación de movimientos | **no** se hace en E075: tras confirmar, el cliente llama a E028 |
| Idioma | `locale` opcional; por defecto el del perfil |
| Errores | `400` si `text` vacío o > 512 caracteres; `422` si no se reconoce intención |

## 8. E058 — Preferencias de notificación

El contrato vigente son **3 booleanos** (`pushEnabled`, `emailEnabled`, `reminderEnabled`) +
`version`. La auditoría pide `enabled`, `budgetAlerts`, `goalUpdates`, `recurringReminders`,
`streakReminders`, `quietHours`.

**Doce diferencias de COMPATIBILIDAD**: es un cambio de forma, no un campo suelto. Decisión
propuesta: **adoptar la forma granular** solo con Gate, porque tres booleanos no pueden expresar
la pantalla; mientras tanto el contrato vigente se mantiene y el endpoint **no se congela**.

## 9. Dinero (transversal, no negociable)

Válido para E028, E031, E033, E037, E039–E047, E049–E051, E074:

- Importes **siempre string decimal** en el contrato y **enteros de centavos** en Go. Cero `float`.
- Moneda explícita `MXN`; redondeo **HALF_UP** a centavos, documentado por operación.
- `Idempotency-Key` obligatoria en mutaciones financieras (E028, E044, E037…).
- Escrituras con `TransactWriteItems` + condición de versión; sin descuentos dobles.
- Reversión que netea exactamente; nunca parcial silenciosa.
- Límite duro: **100 acciones por transacción DynamoDB**; el contrato de reparto debe acotar N metas.
- Aislamiento por usuario en `PK`; `Scan` prohibido.

## 10. Persistencia — 28 referencias obsoletas

Modelo vigente verificado en producción (E003/E004):

```
USER#<userId> / SESSION#<sessionId>
REFRESH#<sha256(refreshToken)> / SESSION
```

Se sustituye la referencia documental `SESSIONHASH#digest` en `spec.md`/`plan.md`/`tasks.md`/
`checklist.md` de **E005–E011** mediante **una sola enmienda transversal**. No cambia tabla,
índice ni código, y **no toca E003/E004**.

## 11. DynamoDB y Redis — factibilidad

| Patrón | Implementación | Índice nuevo |
|---|---|---|
| Movimientos por fecha | `Query` sobre `USER#<id>` con prefijo `MOV#` | No |
| Movimientos por tipo/fecha | `GSI1` (`GSI1PK`/`GSI1SK`) ya desplegado | No |
| Sesiones y refresh | `Query` por prefijo + `GetItem` | No |
| Preferencias, onboarding, consentimientos | `GetItem`/`PutItem` por clave compuesta | No |
| Aportaciones y metas | `TransactWriteItems` + condición de versión | No |
| Paginación | `ExclusiveStartKey` sobre `SK` | No |
| Dashboard / bootstrap / export | agregación en aplicación + caché Redis 30 s | No |

**No se propone ningún índice nuevo**: no hay patrón demostrado que lo exija. `GSI2` sigue
desactivado. Si un patrón futuro lo requiriera, se demostraría con la matriz de accesos antes de
proponerlo (I-040).

## 12. Integración Android

Contrato operativo en `sdd/_control/H08/CONTRATO_INTEGRACION_ANDROID.md`. Resumen:

- Base: `https://d2lpmqy5w6x9es.cloudfront.net/api/v1/` (sin cambios).
- `Authorization: Bearer <accessToken>` en todo salvo E001, E002 y E005.
- `Idempotency-Key` en mutaciones financieras.
- Errores: envelope único con `error.code` (máquina) y `error.message` (humano) + `traceId`.
- Sesión: `401` ⇒ refresh (E003); refresh fallido ⇒ login. `429` respeta `Retry-After`.
- Fechas ISO-8601 UTC; importes string decimal; cursores opacos (`X-Cursor-Next`/`nextCursor`).

## 13. Enmiendas exactas que se aplicarían al aprobar

| # | Archivo(s) | Cambio | Endpoints | Clase |
|---|---|---|---|---|
| A1 | `E005`–`E011` `spec.md`/`plan.md`/`tasks.md`/`checklist.md` | `SESSIONHASH#digest` → modelo real | 7 | OBSOLETA |
| A2 | OpenAPI YAML+JSON, `spec.md`/`plan.md` de E055 | añadir `hasEnoughData` (boolean, requerido) | E055 | ESENCIAL |
| A3 | `spec.md`/`plan.md` de E058 + OpenAPI | preferencias granulares + `quietHours` | E058 | COMPATIBILIDAD |
| A4 | `DECISIONES_CONTRATO_E005_E075.json` | cerrar las 5 HTTP como «conservar» | 5 | COMPATIBILIDAD |

**Nada más se modifica.** E001–E004 intactos; sin cambios de código, Terraform ni AWS.

## 14. Criterio de congelación

Un endpoint es `CONTRACT_FROZEN=VERIFIED` solo si: contrato completo (request, response, errores,
seguridad, validaciones), reglas de negocio y persistencia factibles documentadas, concurrencia e
idempotencia definidas, compatibilidad Android resuelta, pruebas de aceptación escritas, **cero
diferencias ESENCIAL/COMPATIBILIDAD pendientes**, enmiendas aplicadas y CI documental verde.

Separación de estados obligatoria: `CONTRACT_FROZEN` ≠ `IMPLEMENTED` ≠ `MERGED` ≠ `DEPLOYED` ≠
`AWS_VERIFIED` ≠ `USER_ACCEPTED`.

## 15. Gates

| Gate | Alcance | Estado |
|---|---|---|
| `APRUEBO_RFC_016_CONTRATOS_E005_E075` | Clasificación + decisiones + A1–A4 | **PENDIENTE** |
| `APRUEBO_ENMIENDA_A1_PERSISTENCIA` | Aplicar A1 (7 specs) | **PENDIENTE** |
| `APRUEBO_ENMIENDA_A2_E055` | Aplicar A2 | **PENDIENTE** |
| `APRUEBO_ENMIENDA_A3_E058` | Aplicar A3 | **PENDIENTE** |
| `APRUEBO_MERGE_PR_65_SHA_<SHA40>` | Merge del PR | **PENDIENTE** |
| Gates AWS SES / ML | Identidad, sandbox, IAM, conectividad | **PENDIENTE** (no bloquean congelar) |

## 16. Riesgos

| Riesgo | Control |
|---|---|
| Clasificar 784 campos como opcionales y perder necesidad real | Registrados con motivo; cualquier cliente puede reabrir con RFC; el Gate por endpoint revisa su lista |
| Romper E001–E004 | No se tocan; `comparar_contrato_api.py` en CI |
| Congelar sin evidencia | `CONTRACT_FROZEN=NOT_VERIFIED` mientras falte una aprobación |
| SES no disponible | `CONTRACT_READY` separado de `AWS_READY` |
| ML tratado como caja negra | Artefacto JSON auditable + explicación determinista + prohibición de cifras inventadas |
| Cambios silenciosos futuros | Baseline reproducible + `DECISIONES_CONTRATO_*.json` + CI |

---

# 17. Correcciones de revisión 2 (2026-11-16)

> **Precedencia:** esta sección sustituye a lo que contradiga en §4–§8 y §13. El cuerpo anterior se
> conserva para trazabilidad; no se reescribe la historia.

## 17.1 (A) Revisión material de los 784 campos pospuestos

La clasificación automática agrupó como OPCIONAL todo candidato cuya base no apareciera en el
schema. Ese criterio **no basta**: se revisaron las siete categorías de riesgo con el contrato real
delante. Resultado de la revisión:

| Categoría | Evidencia verificada | Conclusión |
|---|---|---|
| **Dinero** (E028, E031, E037, E039–E047, E049–E051) | El contrato ya devuelve importe (`amount`), moneda, `allocations[]`, `remaining` (E043), `status` y `version`. Los campos extra son agregados o sumas derivables | OPCIONAL confirmado; invariantes de dinero viven en el servidor (§9), no en campos nuevos |
| **Autenticación** (E005–E011) | E006/E011 son públicas; el token de recuperación no es JWT; las sesiones se modelan con `USER#/SESSION#` y `REFRESH#` | Solo OBSOLETA (persistencia); ningún campo nuevo ESENCIAL salvo `hasEnoughData` en ML |
| **Estados funcionales** (E014, E017, E042, E047, E063) | El contrato ya expone `status` (`active`/`reversed`), códigos HTTP y `version` | OPCIONAL; la reversión se acredita con `status` + recarga de E038 |
| **Relaciones entre recursos** (E029, E037, E040) | SDD devuelve el recurso creado con `id`; las referencias cruzadas se resuelven consultando el recurso | OPCIONAL |
| **Flujos que dependen de otros endpoints** (E020, E021, E043, E051, E074) | Son agregados componibles desde endpoints atómicos ya aprobados | OPCIONAL; sin campo nuevo |
| **ML** (E052, E055) | **Gap real**: ni `items: []` ni `hasEnoughData` distinguen «sin historial» de «sin patrones» | **E055 `hasEnoughData` → ESENCIAL**; `E052 enoughData/minimumSamples` → COMPATIBILIDAD (misma ambigüedad, resolver en su Gate) |
| **Notificaciones** (E057, E058) | 3 booleanos no expresan la pantalla: faltan alertas por presupuesto/meta, recordatorios y horario de silencio | **12 campos de E058 → COMPATIBILIDAD** (§17.6) |
| **Voz** (E075) | Reconocimiento en Android; `text` cubre la entrada | `transcript` → COMPATIBILIDAD (renombrado) |

**Reclasificaciones efectuadas en esta revisión:** `E052:enoughData` y `E052:minimumSamples` pasan
de OPCIONAL a **COMPATIBILIDAD**. El resto se mantiene, ahora con evidencia explícita por categoría.
Sigue habiendo **0 diferencias sin clasificar**.

> `OPCIONAL` sigue significando **POSPUESTO y registrado**, nunca «resuelto». Cada endpoint puede
> reabrir su lista en su Gate de implementación con RFC mínima.

## 17.2 (B) Operaciones públicas — corrección del contrato de integración

**Verificado en OpenAPI: 13 operaciones tienen `security: []`.** La guía anterior decía «todo salvo
E001, E002 y E005», lo que era incorrecto. Lista real:

```
E001 POST /auth/register              E002 POST /auth/login
E003 POST /auth/refresh               E005 POST /auth/password/forgot
E006 POST /auth/password/reset        E011 POST /auth/email-verification/confirm
E015 GET  /privacy/notices/current    E022 GET  /catalogs/categories
E023 GET  /catalogs/frequencies       E070 GET  /help/faq
E071 GET  /education/tips             E072 GET  /health/live
E073 GET  /health/ready
```

**E006 es pública y no exige `Bearer`**: el usuario que olvidó su contraseña no tiene sesión. El
`token` de recuperación es un **secreto de un solo uso** (32 bytes CSPRNG, `base64url`), **no** un
JWT: no lleva claims, no se valida con la clave RS256 y no sirve para autenticar ninguna otra
operación. E003 (refresh) también es pública porque se usa precisamente cuando el access token
expiró.

## 17.3 (C) E005 — aceptación durable del trabajo

**Problema:** `202` no puede prometer entrega, y un fallo de SES no debe perder el trabajo aceptado.

**Solución adoptada (sin SQS ni Lambda):** el alta del token y la del **trabajo durable** ocurren
en **una sola `TransactWriteItems`** sobre DynamoDB antes de responder:

```
Acción 1: PutItem  USER#<id> / PWRESET#<hmac>   {tokenHash, expiresAt, used:false, version}
Acción 2: PutItem  USER#<id> / OUTBOX#<ulid>   {tipo:"PASSWORD_RESET", estado:"PENDIENTE",
                                                intentos:0, proximoIntento, expiraEn}
Acción 3: ConditionCheck USER#<id> / PROFILE   (existe y ACTIVE)   [solo cuentas reales]
```

Reglas:

1. **`202` se emite solo si la transacción confirmó**: en ese instante el trabajo está persistido y
   es recuperable. `202` = «aceptado y persistido», **nunca** «correo enviado».
2. Tras el commit se intenta el envío con SES. El resultado **no** altera la respuesta (evita
   enumeración por código): éxito ⇒ `estado=ENVIADO`; fallo ⇒ queda `PENDIENTE` con `intentos+1`.
3. **Reintento sin infraestructura nueva:** cada petición a E005 procesa hasta **5** trabajos
   `PENDIENTES` vencidos de *ese mismo usuario* (barrido acotado, con backoff exponencial y tope de
   6 intentos). Sin scheduler dedicado; la limitación se documenta y el runbook permite forzar el
   barrido.
4. **Cuenta inexistente:** se ejecuta igualmente la transacción (token señuelo y outbox con
   `destinatario=null`), de modo que el coste de escritura y el número de acciones son idénticos.
   No se envía nada.
5. **No enumeración (mitigaciones, no promesas):** misma respuesta, mismo cuerpo, mismo código y
   misma *forma* de trabajo; **suelo de latencia** igualando el p95 del camino con envío mediante
   espera acotada (≤ 250 ms) más jitter; límites por IP y por correo (HMAC) que encarecen el
   muestreo. **No se afirma latencia idéntica**: las pruebas exigen una **prueba estadística**
   (comparación de distribuciones de tiempo con y sin cuenta, α=0,01, sin diferencia significativa
   en p50/p95 sobre 200 muestras por rama).
6. **Fallo total de DynamoDB** ⇒ `503` genérico (no hay trabajo persistido, no se puede aceptar).
7. Si el `TransactWriteItems` supera el límite de acciones (no ocurre: 3 acciones fijas) el endpoint
   responde `503` y no `202`.

Con esto, **toda respuesta `202` implica trabajo durable**: no se pierde una solicitud aceptada.

## 17.4 (D) E006 — revocación de sesiones por encima del límite transaccional

**Límite real:** `TransactWriteItems` admite **100 acciones**. Un usuario con 120 sesiones no cabe.

**Mecanismo adoptado — época de revocación (O(1)):**

1. `USER#<id> / PROFILE` guarda `sessionEpoch` (entero) y `passwordChangedAt`.
2. E006, en una transacción de **3 acciones**, (a) consume el token de recuperación con condición de
   uso único, (b) actualiza el hash de contraseña y `passwordChangedAt`, y (c) incrementa
   `sessionEpoch` con `version` como condición de concurrencia.
3. La validación de sesión (middleware y E003) rechaza toda sesión cuyo `createdAt` sea anterior a
   `passwordChangedAt` **o** cuyo `sessionEpoch` guardado sea menor que el vigente. No hace falta
   recorrer ni marcar N sesiones.
4. Limpieza perezosa: las sesiones antiguas se marcan `REVOKED` cuando alguien intenta usarlas.

**Consecuencia de gobierno:** el punto 3 toca código **compartido con E003/E004**. **No se modifica
ahora**: se registra como dependencia obligatoria de E006 y exigirá su propia RFC + Gate antes de
tocar código productivo (`R-002`, `A-110`). Mientras no se implemente, E006 **no puede declararse
`CONTRACT_FROZEN` con revocación garantizada** si el usuario supera 30 sesiones.

## 17.5 (E) E055 — tres estados sin ambigüedad

| Estado | `hasEnoughData` | `items` | Significado para Android |
|---|---|---|---|
| `INSUFFICIENT_DATA` | `false` | `[]` | No hay historial mínimo: mensaje de «registra más movimientos» |
| `NO_PATTERNS` | `true` | `[]` | Hay historial, el modelo **no encontró patrones** (silueta < 0,35 o clusters sin masa) |
| `OK` | `true` | `items.length > 0` | Hay recomendaciones |

`hasEnoughData=false` **nunca** representa «no encontró clusters»: eso es `true` + `items: []`.
La combinación es unívoca con los dos campos ya decididos, sin añadir `modelState`.

## 17.6 (F) E058 — contrato exacto

```
PUT /api/v1/notification-preferences        (Bearer)
Request (application/json, additionalProperties=false)
{
  "pushEnabled":        boolean,   requerido
  "emailEnabled":       boolean,   requerido
  "reminderEnabled":    boolean,   requerido
  "budgetAlerts":       boolean,   opcional  (defecto: true)
  "goalUpdates":        boolean,   opcional  (defecto: true)
  "recurringReminders": boolean,   opcional  (defecto: true)
  "streakReminders":    boolean,   opcional  (defecto: false)
  "quietHours": {                  opcional
    "enabled": boolean,            requerido si existe el objeto
    "start":   "HH:MM",            requerido si enabled=true, 00:00–23:59
    "end":     "HH:MM"             requerido si enabled=true, 00:00–23:59
  },
  "timeZone": "America/Mexico_City",  opcional (IANA), defecto el del perfil
  "version": integer                opcional: concurrencia optimista
}
200 → mismo objeto + "version" (incremental) + "updatedAt" (ISO-8601)
```

| Regla | Valor |
|---|---|
| Semántica | `PUT` = reemplazo completo de las preferencias presentes; las ausentes conservan su defecto |
| Validación | `start != end`; `HH:MM` con minutos 00–59; `timeZone` debe ser zona IANA válida |
| `version` | si se envía y no coincide ⇒ `409 CONFLICT`; si se omite, se aplica sin control optimista |
| Errores | `400` JSON inválido · `401` sin sesión · `409` versión · `422` regla de negocio · `429` · `500` · `503` |
| Persistencia | `USER#<id> / PREFS#NOTIFICATIONS`, `GetItem`/`PutItem` condicional por `version` |
| Consumidores | E058 es la única fuente; E019 (onboarding) y E021 (bootstrap) **leen** estas preferencias, no las duplican |

`GET /notification-preferences` (E057) devuelve el mismo objeto; si no existen, devuelve los
defectos con `version: 0` en lugar de `404`.

## 17.7 (G) E043 / E044 / E047 — verificación con el contrato delante

| Necesidad | Contrato vigente | ¿Falta algo? |
|---|---|---|
| Importe solicitado | `amount` (string decimal) | No |
| Reparto por meta | `allocations[] {goalId, amount}` | No |
| Importe sin asignar en el preview | `remaining` (E043) | No: equivale a `unallocatedAmount` |
| Importe aplicado tras el commit | `amount` + `allocations[]` aceptados; si no cabe, `422` (no hay aplicación parcial silenciosa) | No |
| Identificador de la aportación | `id` (E044/E047) | No |
| Reversión | `status: active|reversed` + `version` + recarga de E038/E040 | No |
| Integridad del reparto | invariantes de servidor (§9) | No |
| Idempotencia | `Idempotency-Key` obligatoria en E044 y E047 | No |
| Concurrencia | `version` + condición | No |

**Conclusión:** `totalAmount` ≡ `amount`; `allocatedAmount` = suma de `allocations.amount`;
`unallocatedAmount` ≡ `remaining`; `contribution.id` ≡ `id`; `affectedGoals`/`reversedAmount` son
derivables de `status` + recarga. **No se reclassifican a ESENCIAL** porque no hay necesidad
demostrada, y así queda escrito con la comparación campo a campo. `mode`/`splitMode`/`goalIds[]`
son formas de entrada alternativas: **OPCIONAL**.

## 17.8 Enmiendas actualizadas

| # | Cambio | Aplicado |
|---|---|---|
| A1 | 28 referencias `SESSIONHASH` → modelo real (7 specs) | **NO** |
| A2 | E055: `hasEnoughData` (boolean, requerido) en OpenAPI YAML+JSON y specs | **NO** |
| A3 | E058: contrato de §17.6 en OpenAPI YAML+JSON y specs | **NO** |
| A4 | Registrar las 5 decisiones HTTP como «conservar» | **NO** |
| A5 | E052: `enoughData` + `minimumSamples` (COMPATIBILIDAD) | **NO** |
| A6 | Época de revocación compartida para E006 | **NO** — requiere RFC propia; toca código de E003/E004 |

**Ninguna enmienda está aplicada.** El detalle exacto de A1–A5 está listo para aplicarse en bloque
en cuanto se concedan los Gates.
