# Checklist de calidad — E002 V6.0.2

**Contrato:** V6.0.2 aprobado por RFC-013. **Implementación local:** completa y con CI verde.
**AWS:** `NOT_RUN`. Cada casilla refleja el estado real, no el deseado.

## Contrato

- [x] Ruta y `operationId` (`nexum_e002`) conservados; 75 operaciones intactas.
- [x] `LoginRequest` con `email`/`password` obligatorios y tres campos opcionales.
- [x] `LoginResponse` propio con `session.id`, `user` y `onboarding`; `AuthTokens` sin cambios.
- [x] `401 UNAUTHORIZED` añadido al contrato; 400/429/500/503 documentados.
- [x] Ejemplos sintéticos, sin credenciales reales.
- [x] `additionalProperties: false` y `required` exactos en YAML y JSON.
- [x] E001 V6.0.2 sin cambios de contrato.

## Seguridad

- [x] Argon2id verificado sin normalizar la contraseña; 401 genérico e indistinguible.
- [x] Hash señuelo derivado del hasher productivo (formato y coste válidos).
- [x] JWT RS256 con `typ`, `kid`, `iss`, `aud`, `sub`, `sid`, `jti`, `iat`, `nbf`, `exp`.
- [x] Rechazo de `alg != RS256`, firma inválida, `kid` desconocido, `iss`/`aud` erróneos,
      claims ausentes, `nbf`/`iat` futuros, expirado y vida > 900 s + leeway.
- [x] Leeway máximo de 30 s.
- [x] Refresh opaco de 32 bytes CSPRNG; sólo su digest se persiste.
- [x] `sessionId`/`familyId` UUID v4 criptográficos; `session.id = sessionId`.
- [x] Clave firmante única y compartida, cargada del bundle RFC-004 en memoria.
- [x] Sin clave ⇒ 503; nunca tokens ficticios.
- [x] Cuota dual atómica (Lua) 5/min/IP y 10/min/email, separada de E001.
- [x] `Cache-Control: private, no-store` en éxitos y errores.
- [ ] HMAC-SHA256 con secreto protegido para el identificador de la cuota por email
      (hoy SHA-256): **pendiente**, requiere Gate de bundle.
- [ ] IP efectiva validada contra la topología real CloudFront→ALB: **pendiente**.

## Persistencia

- [x] `USER#<id>`/`SESSION#<id>` y `REFRESH#<digest>`/`SESSION` en una transacción condicional.
- [x] `expiresAt` **Number** (TTL físico de la tabla) y `expiresAtISO` sólo legible.
- [x] Expiración lógica con `now.Unix() >= expiresAt`.
- [x] `status`, `version`, `createdAt`, `lastUsedAt`, `accessJti` y metadatos de dispositivo.
- [x] Sin `Scan`, sin tablas ni índices nuevos, sin sesiones huérfanas.
- [x] Fallo de transacción ⇒ sin 200 y sin tokens.
- [ ] Pruebas dedicadas del adaptador con dobles: **pendiente**.

## Pruebas y calidad

- [x] `gofmt`, `go build ./...`, `go vet ./...`, `go test ./... -count=1`.
- [x] `staticcheck`, `govulncheck`, `gosec`, `gitleaks` y Semgrep en verde (CI).
- [x] Validadores SDD/OpenAPI/paridad/comparador y manifiesto SHA256.
- [x] Regresión de E001 (registro 201, idempotencia, persistencia) en suites existentes.
- [x] **CI del PR #56: 13/13 checks `SUCCESS`.**
- [ ] Pruebas de handler HTTP, limitador dual y criptografía negativa exhaustiva:
      **pendiente**.

## Release

- [ ] Merge con `APRUEBO_MERGE_PR_56_SHA_<SHA40>`: **pendiente**.
- [ ] Release ARM64 canónico y publicación S3 bajo Gate: **NOT_RUN**.
- [ ] Bundle con la clave RSA compartida bajo Gate RFC-004: **NOT_RUN**.
- [ ] Despliegue gradual por ASG y restauración de capacidad/protecciones: **NOT_RUN**.
- [ ] Verificación HTTPS real (200 mínimo y completo, 401, sesión en DynamoDB, dos nodos):
      **NOT_RUN**.
- [ ] `E002_PUBLICADO=VERIFIED`: **no declarado**.

**Criterio de cierre:** el PR puede considerarse *listo para revisión* con los pendientes
declarados arriba; el endpoint **no** está publicado y no se anunciará a Android hasta la
verificación real en AWS.

## Cierre en producción (2026-11-12)

- [x] Release `v0.9.0+7d1ebf3d5357` publicado en S3 (binario 19 041 073 B, SHA-256
      `7ff35efe…4d79`, `If-None-Match: *`, SSE AES256, checksum verificado).
- [x] Bundle RFC-004 con `JWT_PRIVATE_KEY_PEM_B64`, `JWT_KEY_ID` y
      `LOGIN_RATE_LIMIT_HMAC_KEY_B64` (versión `vD2diulCaPMUWEFR8N6RlCx3KRow5ehM`,
      escritura con `If-Match`).
- [x] Launch Template `nexum-h08i-lt-20261111034054695000000001` **v3** con URI y digests
      del release E002; `SECRETOS_URI` intacto; IAM runtime con 6 ARNs exactos.
- [x] `instance refresh` `74a340eb-ee47-4d08-9b9f-8d4ff94ae4f5` **Successful**; dos
      instancias `InService`/`Healthy` en LT v3; ASG restaurado a 2/2/2; dos targets
      `healthy`; sin refresh pendiente.
- [x] `GET /health/live` 200 y `/health/ready` 200 en la API pública.
- [x] E001 `POST /auth/register` 201 con persistencia DynamoDB y replay idempotente.
- [x] E002 `POST /auth/login` 200 (mínimo y completo) con `session.id = sessionId`,
      `expiresIn = 900` y sesión real en DynamoDB.
- [x] JWT: **estructura y claims** comprobados en producción (decodificación del payload:
      `iss`, `aud`, `sub`, `sid`, `exp-iat = 900`, `jti` y `nbf` presentes, `sub`/`sid`
      coincidentes con usuario y sesión reales).
- [ ] **Verificación criptográfica de la firma RS256 en producción**: no ejecutada con una
      clave pública independiente del servidor. La firma se cubre con pruebas unitarias de Go
      en CI (`tests/unit/security`), que no equivalen a validación externa del token emitido
      en AWS.
- [~] **Sesión compartida entre nodos**: comprobado que ambas instancias están `Healthy` en
      LT v3, comparten el mismo bundle de claves y persisten la sesión en DynamoDB; **pendiente**
      la prueba cruzada emitiendo en un nodo y validando contra el otro.
- [x] `401 UNAUTHORIZED` genérico con contraseña incorrecta y con correo inexistente.
- [x] Prueba personal del propietario en Postman: 200 con credenciales correctas y 401 con
      incorrectas. `E002_USER_ACCEPTED=VERIFIED` para ese alcance.
- [x] `memoria.md` actualizada con la cronología y las evidencias del despliegue.
- [ ] Deuda diferida (no bloqueante): drift Redis, verificación empírica de la cadena de
      proxies, suite del adaptador DynamoDB con dobles del SDK, endurecimiento del manejo
      local de credenciales temporales.
