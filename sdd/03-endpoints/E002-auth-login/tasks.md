# Tareas — E002 V6.0.2

> Sólo se marca lo ejecutado y comprobable. `AWS` permanece `NOT_RUN` hasta el despliegue
> autorizado; ninguna casilla local implica prueba en la nube.

- [x] `E002-T01` RFC-013 aprobada (`APRUEBO_RFC_013_E002_V602`) y Gate local
      (`APRUEBO_GATE_H08E_E002_LOCAL`); contrato V6.0.2 fijado en spec/plan.
- [x] `E002-T02` `LoginRequest` estricto con `deviceId`, `deviceName` y `platform`
      opcionales; cuerpo máximo 256 KiB y rechazo de propiedades desconocidas
      (`login_handler.go`, `binding.go`).
- [x] `E002-T03` `LoginResponse` propio con los cinco campos heredados más `session.id`,
      `user` y `onboarding`; `AuthTokens` intacto para E003.
- [x] `E002-T04` `LoginService` con validación canónica, identidad real, estados y 401
      indistinguible; hash señuelo derivado del hasher productivo.
- [x] `E002-T05` Adaptador DynamoDB `SessionStore` con lecturas consistentes y
      `TransactWriteItems` de dos ítems condicionales; `expiresAt` Number como TTL físico.
- [x] `E002-T06` Emisión JWT RS256 de 900 s con `jti` persistido y carga de clave desde el
      bundle RFC-004; fallo cerrado sin material criptográfico.
- [x] `E002-T07` Validación estricta del JWT: `typ`, `kid`, `iss`, `aud`, `sub`, `sid`,
      `jti`, `iat`, `nbf`, `exp`, vida máxima y leeway de 30 s.
- [x] `E002-T08` Cuota dual atómica en Redis (5/min/IP y 10/min/email canónico) con script
      Lua, clases separadas de E001 y `Retry-After`.
- [x] `E002-T09` Composición real en `cmd/server` junto a `/auth/register`, `/health/live` y
      `/health/ready`, sin `MemorySessionStore` en producción.
- [x] `E002-T10` OpenAPI YAML/JSON sincronizados: `LoginResponse`, `LoginSession`,
      `LoginUser`, `LoginOnboarding`, `401`, ejemplos sintéticos y paridad verificada.
- [x] `E002-T11` Excepción de compatibilidad acotada (`ESQUEMA_RESPUESTA_CAMBIADO` del 200 de
      E002) cubierta por RFC-013 y por el registro de excepciones.
- [x] `E002-T12` Validadores verdes: `validar_h06.py`, paridad YAML/JSON, comparador de
      contratos, `validar_sdd_integral.py` y manifiesto SHA256.
- [x] `E002-T13` CI del PR #56 con los 13 checks en `SUCCESS`.
- [x] `E002-T14` Pruebas dedicadas del handler HTTP, del keyring RSA y de la clave HMAC
      (`tests/unit/transport/login_v602_test.go`): contrato 200 con `session.id = sessionId`,
      400 de validación y de cadena de proxies inválida, 401, 429 con `Retry-After`,
      `Cache-Control: no-store`, cuerpo >256 KiB y errores sin credenciales. El adaptador
      DynamoDB sigue cubierto por las suites de integración existentes; **pendiente** una
      suite dedicada con dobles del AWS SDK.
- [x] `E002-T15` Política de IP confiable implementada (`internal/transport/http/clientip.go`):
      se toma la IP escrita por CloudFront (penúltimo salto de `X-Forwarded-For`), se valida
      con `net/netip`, se rechaza la cadena inválida con 400 y **no** se agrupa a los
      clientes en una identidad compartida. La comprobación contra la configuración real de
      CloudFront/ALB es **AWS_NOT_RUN**.
- [x] `E002-T16` Seudonimización de las cuotas con **HMAC-SHA256** y clave propia
      (`LOGIN_RATE_LIMIT_HMAC_KEY_B64`, ≥32 bytes, fail-closed sin ella). El
      provisionamiento del secreto en el bundle RFC-004 es **AWS_GATE_PENDING**.
- [x] `E002-T17` Merge autorizado: PR **#56 MERGED** (`4e4060a` → main `7d1ebf3`), más los
      PR #57 (memoria), #58 (bucket policy) y #59 (wiring del gestor de secretos).
- [x] `E002-T18` Release ARM64 canónico publicado en
      `s3://nexum-h08i-artefactos-713797602090/releases/v0.9.0+7d1ebf3d5357/` (19 041 073 B,
      SHA-256 `7ff35efe…4d79`, `If-None-Match: *`, SSE AES256, checksum verificado) y bundle
      RFC-004 ampliado con RSA/HMAC (`If-Match`, versión `vD2diulCaPMUWEFR8N6RlCx3KRow5ehM`).
- [x] `E002-T19` Despliegue gradual verificado: LT v3, `instance refresh` `74a340eb…`
      **Successful**, dos instancias en LT v3 `Healthy`, dos targets `healthy`, ASG
      restaurado a 2/2/2 y pruebas HTTPS reales de E001 (201 + idempotencia) y E002
      (200 mínimo y completo, 401 genérico). `E002_AWS_VERIFIED=VERIFIED`;
      `E002_USER_ACCEPTED=VERIFIED` para el alcance de la prueba personal en Postman.

## Casos concretos

- [x] `E002-CT01` Contrato 200 con `session.id = sessionId` y `expiresIn = 900` verificado en
      pruebas de aplicación.
- [x] `E002-CT02` 401 genérico en contraseña incorrecta, cuenta eliminada y correo
      inexistente; sin sesión creada.
- [x] `E002-CT03` 400 de validación (email, contraseña vacía, `platform` no admitida).
- [x] `E002-CT04` Fallo de persistencia ⇒ sin 200 y sin tokens.
- [~] `E002-CT05` Dos nodos reales con la misma clave y sesión compartida: **parcial**.
      Comprobado en AWS: dos instancias `InService`/`Healthy` en LT v3, dos targets `healthy`,
      ambas con el mismo `SECRETOS_URI` (mismo bundle RSA/HMAC) y sesión persistida en
      DynamoDB consultada por `GetItem`. **Pendiente la validación empírica cruzada**: emitir
      un token en un nodo y consumirlo/validarlo contra el otro nodo identificado, que no se
      ejecutó.
