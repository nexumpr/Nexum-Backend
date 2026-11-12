# RFC-013 — Alineación contractual de E002 (login) con el prototipo Nexum V6.0.2

| Campo | Valor |
|---|---|
| Identificador | `RFC-013` |
| Título | Contrato completo de `POST /api/v1/auth/login` (E002) V6.0.2 |
| Fecha de registro | 2026-11-11 |
| Estado | **PROPUESTO — requiere aprobación humana** |
| Base | `main` `40f64b6653ec1c0182c23bf0d9f7ac30d5e15a7e` |
| Alcance | Exclusivamente E002 y los componentes compartidos que necesita |
| Endpoint | `POST /api/v1/auth/login` (`operationId: nexum_e002`) |
| Fuente funcional | Transcripción del prompt de trabajo (pantalla «Acceso / Iniciar sesión»). **No se ha consultado el PDF original.** |

> Este documento **no** declara aprobación. La frase de Gate se solicita en §9.

## 1. Motivación

El contrato E002 vigente (H06) sólo admite `email` + `password` y responde `AuthTokens`
de cinco campos. El prototipo V6.0.2 exige además: metadatos opcionales del dispositivo,
identidad real del usuario en la respuesta y el estado real de onboarding. La ampliación se
hace **desde el primer release**, sin versiones intermedias.

## 2. Contrato anterior frente a V6.0.2

| Aspecto | Anterior (H06) | V6.0.2 propuesto |
|---|---|---|
| Request | `email`, `password` | + `deviceId` (≤128), `deviceName` (≤80), `platform` (`android`) — los tres opcionales |
| Response | `AuthTokens`: `accessToken`, `refreshToken`, `tokenType`, `expiresIn`, `sessionId` | **`LoginResponse`** nuevo: los cinco campos legacy **+** `session.id`, `user{id,name,email,emailVerified}`, `onboarding{configured}` |
| Éxito | 200 | 200 (misma ruta, mismo `operationId`) |
| Errores | 200/400/409/422/429/500/503 | **+ 401 `UNAUTHORIZED`** (credenciales inválidas, genérico e indistinguible) |
| `expiresIn` | 900 | 900 (coherente con `exp-iat`) |
| Sesión | sin persistencia DynamoDB conectada al runtime | sesión real en DynamoDB, atómica y compartida |
| `AuthTokens` | esquema compartido | **se conserva intacto** (lo usa E003) |

`sessionId` (plano) y `session.id` representan **la misma** sesión; el segundo es alias de
respuesta, no una sesión adicional.

## 3. Decisiones técnicas que este RFC fija

### 3.1 Convención definitiva de claves de sesión

Se adopta la del SDD H06, compatible con la política IAM vigente (`USER#*`, `SESSION#*`,
`REFRESH#*`):

| Elemento | PK | SK |
|---|---|---|
| Sesión (autoridad) | `USER#<userId>` | `SESSION#<sessionId>` |
| Localizador de refresh | `REFRESH#<sha256(refreshToken)>` | `SESSION` |

Ambas se escriben en **una sola `TransactWriteItems`** con
`attribute_not_exists(PK)`. El helper `SessionKeys(tokenHash)` de `keys.go`
(`SESSION#<hash>/METADATA`) queda **obsoleto para E002** y no se usa; no se elimina hasta
que E003/E004 fijen su reemplazo definitivo (documentado, sin borrar código en este PR).

El ítem de sesión contiene: `userId`, `sessionId`, `familyId`, `status` (`ACTIVE`/`REVOKED`),
`refreshHash`, `accessJti`, `createdAt`, `lastUsedAt`, `expiresAt` y metadatos opcionales
`deviceId`/`deviceName`/`platform`.

**Modelo TTL corregido (enmienda 2026-11-11):** `infra/terraform/modules/dynamodb` fija el
atributo TTL físico en **`expiresAt`** con tipo **Number** (segundos Unix) y su validación
rechaza cualquier otro nombre. Por tanto `expiresAt` se persiste como **número epoch** y,
opcionalmente, `expiresAtISO` (string RFC3339, sólo legibilidad). La comprobación lógica es
`now.Unix() >= expiresAt`, independiente de que DynamoDB no haya purgado el ítem: un ítem
caducado **no** se considera activo. La tabla no se modifica para adaptarla al código.
**Nunca** el refresh en claro. La expiración lógica se comprueba aunque el TTL no haya
purgado el ítem.

### 3.2 JWT RS256

- Algoritmo único `RS256`; se rechaza `alg=none` y cualquier otro.
- `iss = https://api.nexum.finance` y `aud = nexum-client` (valores ya fijados en
  `token.go`; **no** se sustituyen por el host de CloudFront).
- **JOSE header:** `alg=RS256`, `typ=JWT` y `kid`.
- **Claims del payload:** `iss`, `aud`, `sub` (userId), `sid` (sessionId), `jti` (128 bits
  CSPRNG), `iat`, `nbf` y `exp`. Sin datos financieros, email ni contraseñas.
- Vida del access token: **900 s**. Leeway de validación: **≤30 s**.
- Validación obligatoria de `iss`, `aud`, `exp`, `nbf`, `iat`, `kid` conocido, `sub`/`sid`/`jti`
  presentes y firma. Se rechaza `kid` desconocido.
- **Una única clave firmante compartida por las dos instancias**, provista por el bundle de
  secretos (RFC-004) como `JWT_PRIVATE_KEY_PEM_B64` + `JWT_KEY_ID`. Sin clave válida, E002
  falla cerrado (503), nunca emite tokens ficticios. Se prepara `keyring` para rotación: se
  acepta verificación con claves anteriores declaradas, pero sólo se firma con la vigente.
- El material decodificado vive **sólo en memoria** del proceso; nunca en disco, logs ni
  argumentos.

### 3.3 Rate limiting (dos cuotas simultáneas)

- `login:ip` → **5/min/IP** efectiva.
- `login:email` → **10/min/email normalizado**, con el email **seudonimizado** (HMAC-SHA256
  con secreto del bundle) en la clave Redis.
- Claves y ventanas independientes de las de E001 (que conserva 5/15 min/IP).
- Decisión **atómica** (script Lua o `MULTI`/`EXEC` con `INCR`+`PEXPIRE` por clave) para
  evitar carreras; si Redis no responde → **503**, nunca login sin limitar.
- IP efectiva: se toma del **último salto confiable** de la cadena de proxies
  (CloudFront → ALB interno), nunca de un `X-Forwarded-For` arbitrario del cliente. Se
  documenta la topología real y se rechaza el valor si no procede de la cadena.

### 3.4 Cuerpo y validación

Límite de cuerpo para E002: **256 KiB** (política transversal), independiente del 1 MiB
general. `DisallowUnknownFields`; `userId`, `name`, `income`, `recurringExpenses`,
`privacy`, `confirmPassword`, `signupConfirm`, `accountStatus` y cualquier campo no
declarado se rechazan con 400. La contraseña **no** se normaliza (sin trim ni lowercase) y
**no** se le exige la política mínima de alta.

### 3.5 Identidad y estados

1. Normalizar el email con `domain.ValidateEmail` (idéntico a E001).
2. `GetItem ConsistentRead` de `EMAIL#<sha256>` → `userId`.
3. `GetItem ConsistentRead` de `USER#<id>/PROFILE`; comprobar coherencia reserva↔perfil.
4. `status` debe ser `ACTIVE`; `DELETED`/`REVOKED`/desconocido → 401 genérico.
5. Verificar Argon2id con el hasher existente (parámetros sin cambios).
6. Leer `USER#<id>/ONBOARDING`; **si no existe** (usuario histórico), `configured=false` por
   política de compatibilidad explícita y probada — nunca `true` inventado.
7. `emailVerified=false` **no** bloquea el login (la política no lo exige).
8. Email inexistente: se ejecuta una verificación Argon2id señuelo para reducir la
   diferencia temporal observable. No se promete igualdad temporal matemática.

Sin `Scan`, sin búsqueda global, sin usuarios artificiales.

## 4. Impacto en compatibilidad (a medir con la herramienta del repositorio)

Cambios previstos en el contrato comparado con `origin/main`:
`CAMPO_REQUERIDO_NUEVO` no aplica (los tres campos nuevos son opcionales);
`ESQUEMA_RESPUESTA_CAMBIADO` en el `200` (`AuthTokens` → `LoginResponse`); y
`RESPUESTA_...`/nuevo código `401`. Se medirá con `herramientas/comparar_contrato_api.py`
antes de pedir el Gate de CI, y se registrará en
`EXCEPCIONES_COMPATIBILIDAD_API.json` **sólo** lo que resulte, con alcance a E002.

`AuthTokens` no se modifica: E003 sigue usándolo sin cambios.

## 5. Componentes a implementar

| Capa | Elemento |
|---|---|
| Dominio | `LoginRequest` normalizado; estados de cuenta; política de onboarding histórico |
| Aplicación | `LoginService` (identidad + verificación + sesión), `IssueSession` idempotente por transacción |
| Persistencia | `internal/adapters/dynamo/session_store.go` (sesión + localizador de refresh en una transacción) |
| Criptografía | carga de clave desde bundle, `keyring`, validación estricta de claims |
| Transporte | `LoginHandler` + DTO (`LoginRequest`/`LoginResponse`), registro en router y `cmd/server` |
| Infra compartida | limitador dual Redis (IP + email seudonimizado, atómico) |

`MemorySessionStore` **no** se compone en producción.

## 6. Fuera de alcance

E003 (refresh), E004 (logout), E005/E006 (contraseñas), E008/E009/E014 (sesiones):
sólo se dejan preparados los datos y localizadores. No se implementan endpoints nuevos ni
se tocan E001, E005–E075, ni la infraestructura (VPC/ALB/CloudFront/DynamoDB/Redis).

## 7. Riesgos

| Riesgo | Mitigación |
|---|---|
| Clave RSA mal preparada deja el login en 503 | precondición de despliegue: verificar bundle, `versionId` y acceso IAM antes del rollout |
| Doble cuota Redis mal implementada permite fuerza bruta | script atómico + prueba de concurrencia + fail-closed |
| Cambio de `expiresIn` 600→900 rompe tests previos | actualizar pruebas y `session_service` explícitamente |
| Deriva con `AuthTokens` | `LoginResponse` es un esquema nuevo y separado |
| Enumeración de cuentas | 401 genérico idéntico + señuelo Argon2id |
| Coste | Sin recursos nuevos previstos; el consumo operacional de DynamoDB/Redis/CloudFront sigue existiendo y **no se garantiza coste cero** |

## 8. Pruebas y criterios de cierre

Los de §4.2 del prompt: login válido (mínimo y completo), 400 por cada clase de entrada
inválida, 401 genérico en los cinco escenarios de credencial/estado, RS256 y rechazos
criptográficos, dos sesiones reales en DynamoDB, TTL y expiración lógica, atomicidad,
rate limit dual con `Retry-After`, 503 con dependencias caídas, regresión de E001 y
pruebas de dos procesos con la misma clave.

## 9. Aprobaciones solicitadas

| Token | Alcance |
|---|---|
| `APRUEBO_RFC_013_E002_V602` | Alineación contractual E002 descrita en §2–§3 |
| `APRUEBO_GATE_H08E_E002_LOCAL` | Implementación local de E002 (código, tests, SDD, OpenAPI) y PR técnico único |

Ninguno autoriza merge, escritura S3, cambio de secretos ni despliegue; esos tienen sus
propios Gates (§5 y §6 del prompt de trabajo).
