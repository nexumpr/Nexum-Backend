# Tareas — E003 (`POST /api/v1/auth/refresh`)

**Estado de evidencia (2026-11-13):** implementación local en `feat/h08e-e003-auth-refresh`.
La compilación y las pruebas se validan en el **CI Linux** del PR: en el host Windows actual
`gofmt.exe` y el toolchain Go 1.27.4 están bloqueados por *Device Guard*
(`BLOCKED_POLICY_APPLICATION_CONTROL`). Ninguna casilla se marca por existir el documento.

- [x] `E003-T01` Contrato revisado: `operationId=nexum_e003`, `POST /api/v1/auth/refresh`,
      `RefreshRequest` → `AuthTokens` (200). Diferencias con el PDF V6.0.2 aprobadas por
      **RFC-014** (`APRUEBO_RFC_014_E003_REFRESH`, 2026-11-13T18:18Z) y aplicadas en el
      OpenAPI: `401`, `deviceId` opcional y `session.id`.
- [x] `E003-T02` DTO estricto: `RefreshRequest` (`refreshToken` obligatorio, `deviceId`
      opcional) con `additionalProperties` cerrado vía `BindJSONLimit` (8 KiB) y
      `DisallowUnknownFields`. `AuthTokens` construido en el borde de transporte.
- [x] `E003-T03` El token opaco es la credencial; la identidad se deriva del localizador
      durable y de la sesión, nunca del cuerpo. Sin modo invitado.
- [x] `E003-T04` Caso de uso e invariantes: rotación de un solo uso por compare-and-swap;
      reutilización revoca la familia (`ErrRefreshReplay`); conflicto sin evidencia no revoca.
- [x] `E003-T05` Adaptador DynamoDB: `REFRESH#<sha256>/SESSION` + `USER#<id>/SESSION#<id>` +
      `USER#<id>/PROFILE`; `ConsistentRead=true`; TTL `expiresAt` Number; transacción de cuatro
      acciones; sin `Scan`. Condición de consumo compatible con localizadores de E002 (sin
      `used`) y de E003 (`used=false`).
- [x] `E003-T06` Redis: cuotas 30/min/IP y 12/min/digest con HMAC-SHA256 y ventana atómica;
      indisponibilidad ⇒ 503. La caché nunca autoriza.
- [x] `E003-T07` Códigos: 400 (`INVALID_REQUEST`), 401 (`UNAUTHORIZED`, RFC-014), 409
      (`CONFLICT` replay/concurrencia), 429 (`RATE_LIMITED` con `Retry-After`), 500, 503.
      Sin `Idempotency-Key` (no exigida por el contrato) y sin `ETag`.
- [x] `E003-T08` Pruebas unitarias de aplicación: contrato, validación, cuenta no activa,
      perfil incoherente o ausente, replay, conflicto, fallo cerrado y concurrencia.
- [x] `E003-T09` Pruebas del adaptador con doble con estado que interpreta las condiciones
      emitidas: cadena A→B→C→D, consumo durable, familia ajena intacta, concurrencia.
- [x] `E003-T10` Negativas de autenticación, propiedad, reutilización y cuotas en el handler.
- [ ] `E003-T11` CI + seguridad + revisión de contratos: **PASS** en el run `43478609688`
      (HEAD `04799eb`) con el contrato aún sin modificar. Tras aplicar RFC-014 se reejecuta CI
      sobre el HEAD nuevo; hasta entonces no se declara verde para el contrato final.
- [ ] `E003-T12` Autorización humana de release, despliegue, prueba en dos nodos y evidencia
      real: **NOT_RUN**; requiere Gates AWS separados.

## Casos concretos

- [x] `E003-CT01` Respuesta 200 con `accessToken`, `refreshToken`, `tokenType=Bearer`,
      `expiresIn=900`, `sessionId` y alias `session.id` (RFC-014 pendiente).
- [x] `E003-CT02` JSON malformado, campos desconocidos, tipo incorrecto, token vacío,
      excesivamente largo o con caracteres de control ⇒ 400 sin reflejar el token.
- [x] `E003-CT03` No aplica recurso ajeno por ruta: la identidad sale del token. Se verifica
      que una familia no revoca a otra.
- [x] `E003-CT04` DynamoDB o Redis no disponibles ⇒ 503; nunca tokens ficticios ni escritura
      parcial.
- [ ] `E003-CT05` Dos nodos Go **reales**: simulado localmente con dos servicios sobre la
      misma persistencia; AWS `NOT_RUN`.
- [x] `E003-CT06` Reutilización de un token consumido ⇒ rechazo y revocación durable
      observable por cualquier nodo que lea la sesión.
