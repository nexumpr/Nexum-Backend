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
- [ ] `E002-T14` Pruebas dedicadas del adaptador DynamoDB, del limitador dual y del handler
      HTTP con dobles: **pendiente**.
- [ ] `E002-T15` Política de IP efectiva verificada sobre la topología CloudFront→ALB y, si
      procede, endurecimiento de la cabecera: **pendiente**.
- [ ] `E002-T16` Seudonimización de la cuota por email mediante HMAC-SHA256 con secreto del
      bundle (hoy SHA-256): **pendiente**, requiere Gate de bundle.
- [ ] `E002-T17` Merge autorizado con `APRUEBO_MERGE_PR_56_SHA_<SHA40>`: **pendiente**.
- [ ] `E002-T18` Release ARM64 canónico, publicación S3 y bundle de claves bajo sus Gates:
      **pendiente**.
- [ ] `E002-T19` Despliegue gradual por ASG y verificación HTTPS real de E001 y E002:
      **pendiente** (`AWS_NOT_RUN`).

## Casos concretos

- [x] `E002-CT01` Contrato 200 con `session.id = sessionId` y `expiresIn = 900` verificado en
      pruebas de aplicación.
- [x] `E002-CT02` 401 genérico en contraseña incorrecta, cuenta eliminada y correo
      inexistente; sin sesión creada.
- [x] `E002-CT03` 400 de validación (email, contraseña vacía, `platform` no admitida).
- [x] `E002-CT04` Fallo de persistencia ⇒ sin 200 y sin tokens.
- [ ] `E002-CT05` Dos nodos reales con la misma clave y sesión compartida: **NOT_RUN** (AWS).
