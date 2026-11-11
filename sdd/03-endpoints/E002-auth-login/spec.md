# E002 — Iniciar sesión y emitir tokens (V6.0.2)

| Propiedad | Contrato |
|---|---|
| Método y ruta | `POST /api/v1/auth/login` |
| `operationId` | `nexum_e002` |
| Origen H04 | Clase `M` |
| Versión de contrato | **V6.0.2** (RFC-013, aprobada) |
| Seguridad | `public`; sin `Authorization` previo ni `Idempotency-Key` |
| Respuesta éxito | **HTTP 200**; `LoginResponse` (esquema propio) |
| Persistencia | DynamoDB `NEXUM_CORE_<ENTORNO>`; sin `Scan`; Redis para cuotas |
| Cifrado | JWT RS256 900 s + refresh opaco de 32 bytes CSPRNG |

## 1. Problema y alcance

El contrato H06 sólo admitía `email` y `password` y devolvía `AuthTokens` de cinco campos.
El prototipo V6.0.2 exige, desde el primer release, los metadatos opcionales del
dispositivo, la identidad real del usuario y el estado real de onboarding, además de una
sesión persistida y compartida por los dos nodos.

Fuera de alcance: E003 (refresh), E004 (logout), E005/E006 y el resto del catálogo.

## 2. Entrada — `LoginRequest`

| Campo | Obligatorio | Regla |
|---|:---:|---|
| `email` | Sí | Email válido, máximo 254; normalizado **exactamente** como E001 (`domain.ValidateEmail`). |
| `password` | Sí | No vacía, máximo 128; **sin** trim, minúsculas ni normalización Unicode. Verificación Argon2id. No se exige la política de alta a cuentas antiguas. |
| `deviceId` | No | Máximo 128 caracteres; identificador de instalación, no sensible. Nunca autoriza acceso. |
| `deviceName` | No | Máximo 80 caracteres; etiqueta legible del dispositivo. |
| `platform` | No | Enum cerrado `android` (minúsculas). |

JSON estricto (`additionalProperties:false`), cuerpo máximo **256 KiB**. Se rechazan con
400 `userId`, `name`, `income`, `recurringExpenses`, `privacy`, `confirmPassword`,
`signupConfirm`, `accountStatus` y cualquier propiedad no declarada. La ausencia de
metadatos opcionales nunca provoca rechazo.

## 3. Salida — `LoginResponse` (200)

`accessToken`, `refreshToken`, `tokenType` (`Bearer`), `expiresIn` (**900**), `sessionId`,
`session.id` (**igual a `sessionId`**), `user.id`, `user.name` (desde `PROFILE.displayName`),
`user.email`, `user.emailVerified`, `onboarding.configured`.

Los datos de `user` y `onboarding` proceden de DynamoDB, nunca de valores fijos. Un usuario
histórico sin ítem `ONBOARDING` devuelve `configured=false` (RFC-013) y nunca `true`
inventado. No se devuelven contraseñas, hashes, refresh en claro persistido, datos
financieros ni identificadores de infraestructura. `Cache-Control: private, no-store`.

## 4. Errores

| Código | `code` | Cuándo |
|---|---|---|
| 400 | `INVALID_REQUEST` | JSON malformado, email inválido, campos faltantes o desconocidos, tipos o longitudes incorrectas, plataforma no admitida, cuerpo > 256 KiB |
| 401 | `UNAUTHORIZED` | Mensaje genérico e indistinguible para correo inexistente, contraseña incorrecta y cuenta deshabilitada/eliminada/revocada |
| 429 | `RATE_LIMITED` | Cuota superada, con `Retry-After` |
| 500 | `INTERNAL_ERROR` | Fallo inesperado saneado |
| 503 | `DEPENDENCY_UNAVAILABLE` | DynamoDB/Redis no disponible o falta de material criptográfico |

Nunca se usa 404 para "correo inexistente" ni se entra en modo invitado ante un email
desconocido.

## 5. Identidad y estados

Reserva `EMAIL#<sha256(email normalizado)>`/`OWNER` → `USER#<id>`/`PROFILE` con
`ConsistentRead`, comprobando coherencia reserva↔perfil. Sólo `status = ACTIVE` puede
autenticarse. Argon2id con el hasher de E001 (parámetros sin cambios). Ante correo
inexistente se ejecuta una verificación señuelo con Argon2id real y contraseña aleatoria,
sin prometer igualdad temporal matemática. Sin `Scan` ni búsqueda global.

## 6. JWT RS256 y keyring

Cabecera JOSE: `alg=RS256`, `typ=JWT`, `kid` conocido. Claims: `iss=https://api.nexum.finance`,
`aud=nexum-client`, `sub`, `sid`, `jti` (128 bits CSPRNG), `iat`, `nbf`, `exp` con
`exp-iat ≤ 900 s`. Validación: firma RSA ≥2048 bits, rechazo de `alg` distinto, `kid`
desconocido, `iss`/`aud` incorrectos, `sub`/`sid`/`jti` ausentes, `nbf`/`iat` futuros fuera
de tolerancia, expirado, coherencia temporal y vida excesiva. Leeway máximo 30 s. La clave
firmante es única y compartida por los dos nodos, cargada desde el bundle RFC-004
(`JWT_PRIVATE_KEY_PEM_B64`, `JWT_KEY_ID`) y decodificada sólo en memoria; sin clave, 503.

## 7. Sesiones y persistencia

Una sola `TransactWriteItems` condicional con `USER#<id>`/`SESSION#<id>` y
`REFRESH#<sha256(refresh)>`/`SESSION`. Sólo se persiste el digest del refresh. Atributos:
`userId`, `sessionId`, `familyId`, `status` (`ACTIVE`/`REVOKED`), `refreshHash`, `accessJti`,
`createdAt`, `lastUsedAt`, **`expiresAt` Number (epoch seconds, TTL físico de la tabla)**,
`expiresAtISO` (legibilidad) y metadatos opcionales del dispositivo. La expiración lógica se
comprueba con `now.Unix() >= expiresAt`: un ítem caducado no autentica aunque el TTL no lo
haya purgado. Si la transacción falla, no se devuelve 200 ni se emiten tokens.

## 8. Redis y cuotas

Dos cuotas simultáneas e independientes de E001 (**5/min/IP** y **10/min/email
normalizado**), evaluadas en un único script Lua atómico con TTL por clave e identificadores
seudonimizados. Redis no disponible ⇒ 503. Cuota excedida ⇒ 429 con `Retry-After`. La IP
efectiva debe proceder de la cadena de proxies confiable (CloudFront → ALB), nunca de un
`X-Forwarded-For` arbitrario del cliente.

## 9. Compatibilidad

E001 V6.0.2 no se modifica: registro, contrato, idempotencia y persistencia intactos.
`AuthTokens` se conserva para E003. Los usuarios existentes no se migran ni se reescriben.

## 10. Aceptación observable

`201` de E001 con una cuenta sintética; `200` de E002 con esa cuenta (JSON mínimo y
completo) devolviendo los ocho bloques del contrato, `session.id = sessionId`,
`expiresIn=900`, `tokenType=Bearer`; `401` genérico con contraseña errónea y con correo
inexistente; sesión real consultable en DynamoDB con `expiresAt` numérico; `429` al superar
cualquiera de las dos cuotas. **AWS real: NOT_RUN** hasta el despliegue autorizado.
