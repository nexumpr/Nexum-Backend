# Gate H08-E — Implementación E007–E011 (secciones por endpoint)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/GATE_H08E_E007_E011_IMPLEMENTACION.md` |
| Fecha | 2026-11-17 |
| Base | `main` `4ba790a9ffa99b15bb73a6b2e93e691a2ba716b6` |
| Instrumento de gobierno | `RFC-019-OLEADA_E007_E011.md` y `GATE_H08E_E007_E011_OLEADA.md` |
| Alcance | `AUTHORIZED_E007_E011_LOCAL` |
| Autoridad de contrato | OpenAPI 3.1 aprobado (`nexum-api-v1.yaml` y espejo `.json`) + RFC-016 v4 y enmiendas |

Cada endpoint conserva su **sección de Gate** con contrato, decisiones y pruebas. El control de
merge sigue siendo individual por PR y HEAD SHA; el de apply, por SHA-256 del plan.

---

## E007 — `POST /api/v1/auth/password/change`

| Campo | Valor |
|---|---|
| Rama | `feat/h08e-e007-password-change` |
| Worktree | `.worktrees/worktree-auth-change` |
| Estado | `IMPLEMENTING` |

**Contrato.** Bearer obligatorio. Request `PasswordChangeRequest` `{currentPassword, newPassword}`
con `additionalProperties: false`. Éxito **200** `Accepted` `{status:"accepted", requestId}`.
Códigos 200/400/401/403/404/409/422/429/500/503.

**Decisiones.**
- **D-E007-01 (época de revocación).** Se incrementa `sessionEpoch` en la misma transacción que el
  nuevo `passwordHash`, reutilizando A6/RFC-018. Consecuencia verificada en código: `refresh.go`
  rechaza `session.SessionEpoch != user.SessionEpoch` y el middleware compara el claim `epo`, por lo
  que **todas** las sesiones dejan de autorizar, incluida la actual, y el cliente reautentica. Es la
  única opción atómica; el contrato `Accepted` no admite devolver tokens nuevos. Descartada la
  revocación sesión por sesión (fan-out no atómico sobre un límite de 100 acciones).
- **D-E007-02.** Contraseña actual incorrecta → 422 `BUSINESS_RULE_VIOLATION` con mensaje genérico.
- **D-E007-03.** Una sola acción `Update` sobre `USER#<id>/PROFILE` con CAS de `version` y de
  `sessionEpoch`, admitiendo perfiles legados sin `version`. Sin tablas ni índices nuevos.

**Pruebas exigidas.** Contraseña actual incorrecta → 422 sin escritura; nueva contraseña persistida
como Argon2id verificable y nunca en claro; `sessionEpoch` y `version` incrementadas; login con la
nueva contraseña válido y con la anterior 401; access y refresh previos rechazados por época
desalineada; 400 campo desconocido o JSON malformado; 401 sin Bearer; 429 cuota; 503 dependencia
ausente; la respuesta no incluye tokens, `changedAt` ni contadores.

---

## E008 — `GET /api/v1/auth/sessions`

| Campo | Valor |
|---|---|
| Rama | `feat/h08e-e008-sessions` |
| Worktree | `.worktrees/worktree-sessions` |
| Estado | `IMPLEMENTING` |

**Contrato.** Bearer obligatorio. Query `limit` (1..100, default 25) y `cursor` opaco. Éxito **200**
`PageOfSession` `{items[], nextCursor?}` con `Session` de exactamente cinco campos requeridos:
`id`, `createdAt`, `lastSeenAt`, `current`, `revoked` (`additionalProperties: false`). Cabecera
`Cache-Control: private, no-store`. Códigos 200/400/401/403/404/429/500/503.

**Decisiones.**
- **D-E008-01.** `Query` sobre `PK=USER#<sub>` con `SK begins_with SESSION#`, `Limit` y
  `ExclusiveStartKey`. Prohibido `Scan`. La identidad sale solo de `sub`.
- **D-E008-02.** `lastSeenAt` se mapea desde el atributo `lastUsedAt` que escriben E002/E003; si
  falta se usa `createdAt`. No se expone `refreshHash`, `accessJti`, `deviceId`, IP ni user-agent.
- **D-E008-03.** `current` = `sessionId == sid`; `revoked` = `status != ACTIVE` (ítem sin `status`
  se considera activo, coherente con `statusMatchesActive`).
- **D-E008-04.** Cursor opaco firmado con HMAC-SHA256, ligado a `userId` y `limit`, con expiración;
  manipulado, vencido o de otro usuario → 400.

**Pruebas exigidas.** 200 con el esquema exacto de cinco campos y sin campos extra; paginación real
con `nextCursor` sin duplicados ni saltos; cursor inválido, vencido, manipulado o de otro usuario →
400; aislamiento entre dos usuarios; `current` correcto para la sesión del token; `revoked` correcto;
401 sin Bearer; 503 sin persistencia; sin `Scan`.

---

## E009 — `DELETE /api/v1/auth/sessions/{sessionId}`

| Campo | Valor |
|---|---|
| Rama | `feat/h08e-e009-sessions-revoke` (apilada sobre E008) |
| Worktree | `.worktrees/worktree-sessions` |
| Estado | `IMPLEMENTING` |

**Contrato.** Bearer obligatorio. Path `sessionId` uuid requerido. Éxito **204 sin cuerpo**.
Códigos 204/400/401/403/404/409/422/429/500/503. Sesión ajena o inexistente → 404 homogéneo.

**Decisiones.**
- **D-E009-01 (router).** Soporte mínimo de rutas con `{param}` en `internal/transport/http/router.go`,
  con prioridad absoluta de rutas literales, un segmento por parámetro, extracción segura y plantilla
  correcta para métricas. Sin framework nuevo y sin alterar 404/405/`Allow`.
- **D-E009-02.** Propiedad verificada leyendo `USER#<sub>/SESSION#<id>`; `userId` distinto de `sub`
  → 404 sin revelar existencia.
- **D-E009-03.** Revocación con CAS de `version` y cierre del localizador `REFRESH#<digest>/SESSION`
  en la misma transacción; ya revocada → 204 idempotente sin reescritura.
- **D-E009-04.** Revocar la sesión actual deja el access token en 401 en la siguiente petición
  (el middleware consulta estado autoritativo en DynamoDB con lectura consistente).

**Pruebas exigidas.** 204 sin cuerpo al revocar una sesión propia; access token de la sesión revocada
→ 401 y refresh correspondiente → 401; sesión de otro usuario → 404; inexistente → 404; ya revocada →
204 idempotente; CAS perdido → 409; store caído → 503; regresión de rutas E001–E008 intacta, literal
gana sobre paramétrica y método incorrecto → 405 con `Allow`.

---

## E010 — `POST /api/v1/auth/email-verification/request`

| Campo | Valor |
|---|---|
| Rama | `feat/h08e-e010-email-verification` |
| Worktree | `.worktrees/worktree-email-verification` |
| Estado | `IMPLEMENTING` |

**Contrato.** Bearer obligatorio, **sin cuerpo JSON**. El destinatario se obtiene del perfil
autenticado. Éxito **202** `Accepted` `{status:"accepted", requestId}`. Códigos
202/400/401/403/404/409/422/429/500/503.

**Decisiones.**
- **D-E010-01 (namespace).** `ONE_USE#<sha256(token)>` / `VERIFY`, ya autorizado por IAM
  (`leading_key_patterns` incluye `ONE_USE#*`). **Sin cambio IAM.** Separado de `RESET#` de
  E005/E006 por propósito y ciclo de vida.
- **D-E010-02.** Token CSPRNG de 32 bytes base64url; solo se persiste su SHA-256. TTL 30 min
  (`expiresAt` Number, TTL físico) con caducidad lógica explícita en E011.
- **D-E010-03.** Envío real por SES v2 reutilizando la política vigente (`ses:SendEmail` sobre
  `identity/*` con `ses:FromAddress` fijo). Si SES falla no se responde 202.
- **D-E010-04.** Cuenta ya verificada → 409; no se envía correo. Sin Redis o sin SES → 503.
- **D-E010-05.** No se marca `emailVerified` en este endpoint.

**Pruebas exigidas.** 202 con esquema exacto y token persistido solo como hash; 409 si el correo ya
está verificado; 503 si SES o el limitador no están disponibles, sin 202 ficticio; 401 sin Bearer;
400 si se envía cuerpo; 429 por cuota; el token no aparece en la respuesta ni en los logs.

---

## E011 — `POST /api/v1/auth/email-verification/confirm`

| Campo | Valor |
|---|---|
| Rama | `feat/h08e-e011-email-verification-confirm` (apilada sobre E010) |
| Worktree | `.worktrees/worktree-email-verification` |
| Estado | `IMPLEMENTING` |

**Contrato.** **Público** (`security: []`). Request `VerifyEmailRequest` `{token}` (`minLength 10`,
`additionalProperties: false`). Éxito **200** `Accepted` `{status:"accepted", requestId}`.
Códigos 200/400/409/422/429/500/503.

**Decisiones.**
- **D-E011-01.** Resolución por `GetItem` consistente de `ONE_USE#<sha256(token)>/VERIFY`.
  Inexistente, propósito distinto o caducado → 400 genérico.
- **D-E011-02.** Transacción de dos acciones sobre dos ítems: `PROFILE` (`emailVerified=true`,
  `emailVerifiedAt`, `ADD version :one`, condición de estado activo y CAS de versión y de
  `emailVerified`) y el ítem `ONE_USE` (`used=true`, condición de no uso y `expiresAt > now`).
  DynamoDB prohíbe dos acciones sobre el mismo ítem.
- **D-E011-03.** Replay → 409; concurrencia resuelta por la condición del token (una 200, otra 409).
- **D-E011-04.** No se reutilizan tokens `RESET#`; no se marca verificado antes de consumir el token.

**Pruebas exigidas.** 200 y `emailVerified=true` en DynamoDB con login posterior (E002) reflejando el
cambio; replay → 409; token expirado, inexistente o de otro propósito → 400; perfil inactivo → 400;
store caído → 503; concurrencia: dos confirmaciones simultáneas → una 200 y otra 409; funciona sin
`Bearer`.

---

## Estado agregado

| Endpoint | Código | PR | CI | Merge | Release | AWS | HTTPS | Android |
|---|---|---|---|---|---|---|---|---|
| E007 | IMPLEMENTING | — | — | NOT_AUTHORIZED | NOT_RUN | NOT_RUN | NOT_RUN | NO |
| E008 | IMPLEMENTING | — | — | NOT_AUTHORIZED | NOT_RUN | NOT_RUN | NOT_RUN | NO |
| E009 | IMPLEMENTING | — | — | NOT_AUTHORIZED | NOT_RUN | NOT_RUN | NOT_RUN | NO |
| E010 | IMPLEMENTING | — | — | NOT_AUTHORIZED | NOT_RUN | NOT_RUN | NOT_RUN | NO |
| E011 | IMPLEMENTING | — | — | NOT_AUTHORIZED | NOT_RUN | NOT_RUN | NOT_RUN | NO |

Ninguna fila de este documento declara PASS sin ejecución real. `AWS_NOT_RUN`.
