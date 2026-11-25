# Tareas — E061 (`GET /api/v1/notifications`)

> Estado: implementación local y evidencia unitaria **COMPLETADAS**. Integración real/AWS/dos nodos **BLOQUEADA** sin Gate H08 ni autorización de despliegue.
>
> Evidencia de ejecución real (2026-11-22):
> `go test ./internal/application/notifications ./internal/adapters/dynamo ./tests/unit/transport ./cmd/server -run 'E061|E062|E063' -count=1 -timeout 300s` → `ok` en los cuatro paquetes.
> `go vet ./internal/application/notifications ./internal/adapters/dynamo ./internal/transport/http ./cmd/server` → sin salida.

## Completado con evidencia local

- [x] `E061-T01` Contrato revisado en `sdd/03-endpoints/openapi/nexum-api-v1.yaml`: `operationId=nexum_e061`, `GET /api/v1/notifications`, parámetros `limit/cursor/isRead/type`, `200 PageOfNotification`, `Cache-Control: private, no-store`.
- [x] `E061-T02` DTO estrictos en `internal/application/notifications/service.go` (`ListParams`, `WirePage`, `WireNotification`): whitelist exacta, `items` nunca `null`, `nextCursor` omitido cuando no hay más páginas.
- [x] `E061-T03` Sujeto obtenido de `middleware.UserIDFromContext` tras `middleware.AuthenticateBearerWithSession`; la partición `USER#<sub>` nunca la elige el cliente.
- [x] `E061-T04` Invariantes: cursor HMAC-SHA256 ligado a `userID+limit+isRead+type+LastEvaluatedKey`; `unreadCount` calculado con `Query/Select COUNT` sobre toda la bandeja; sin notificaciones sintéticas.
- [x] `E061-T05` Adaptador `internal/adapters/dynamo/notifications_store.go`: `USER#<sub> / NOTIFICATION#<RFC3339Nano>#<id>`, `Query begins_with`, `ExclusiveStartKey`, `Select COUNT`, sin `Scan`. Los prefijos `DEVICE#/REMINDER#/PREFERENCES` y GSI2 pertenecen a otras especificaciones y no se tocaron.
- [x] `E061-T06` Cuota Redis real por sujeto: `LimitarPorUsuarioContrato(limitador, "analytics", handler)` con `cacheinfra.NewAuthRateLimiter(d.Redis, cuotaAnalyticsPorMinuto, time.Minute)`; Redis no se usa como fuente de privilegios ni de paginación.
- [x] `E061-T07` Errores mapeados a `ErrorEnvelope`: 400 `INVALID_REQUEST`, 401 `UNAUTHORIZED`, 404 `NOT_FOUND`, 429 `RATE_LIMITED`, 503 `DEPENDENCY_UNAVAILABLE`, 500 `INTERNAL_ERROR`. No se declara ETag porque E061 no lo define en OpenAPI.
- [x] `E061-T08` Unit tests reales con fixture sintético: `service_test.go` (vacío, 30 paginadas, filtros, cursor válido/tamper/cross-user) y `notifications_store_test.go` (fake DynamoDB con `ExclusiveStartKey`, `Select COUNT`, fallos cerrados).
- [x] `E061-T09` HTTP local conforme OpenAPI en `tests/unit/transport/notifications_test.go`: esquema exacto y whitelist de campos, 400 (`limit=0/101`, cursor corto/tamper, `isRead`, `type`), 401, 429, 503, 404 homogéneo, `Cache-Control` y `ErrorEnvelope`.
- [x] `E061-T10` Negativas locales de auth/ownership/cuota/fallo de store y paginación sin duplicados (25+5 de 30).
- [x] `E061-T11` `gofmt -w` y `go vet` sobre los archivos propios, sin salida de error.

## Bloqueado (requiere autorización humana / H08)

- [ ] `E061-T09b` Integración contra DynamoDB real y prueba HTTP desde cliente externo: **BLOCKED_AWS**, sin credenciales ni Tabla real; se usó fake client y `httptest`.
- [ ] `E061-T10b` Misma operación en dos nodos Go reales con consistencia de sesión/caché: **BLOCKED_TWO_NODE**, no autorizado.
- [ ] `E061-T11b` CI/seguridad/revisión de contratos/rollback con trazas del despliegue: **BLOCKED_GATE_H08**.
- [ ] `E061-T12` Release: **BLOCKED_RELEASE**; código commitado y pusheado en `9b5d950`, pero sin merge, Terraform, AWS ni despliegue; requiere Gate humano.

## Casos concretos

- [x] `E061-CT01` 200 con esquema exacto verificado por HTTP local (whitelist `items/nextCursor/unreadCount` e `id/title/message/isRead/createdAt`).
- [x] `E061-CT02` Parámetros inválidos y cursor alterado → 400 `INVALID_REQUEST` sin secretos en la respuesta.
- [x] `E061-CT03` Cursor de otro usuario y filtro modificado → 400; ningún item de otra partición aparece.
- [x] `E061-CT04` Store caído o clave HMAC ausente → 503 `DEPENDENCY_UNAVAILABLE`, fail-closed.
- [x] `E061-CT06` Paginación 25/100, filtro `type/isRead` aplicado antes de paginar, sin duplicados; `unreadCount` global.
- [ ] `E061-CT05` Dos nodos reales: **BLOCKED_TWO_NODE**.
- [x] `E061-CT07` Opt-in/opt-out push: **NO APLICA verificado** a E061 (pertenece a E059/E060); este endpoint no envía push.

## Enmienda (A8)

- [x] Filtro `type` aplicado en `FilterExpression` y verificado de nuevo al proyectar; `unreadCount` global en la página.
- [x] Validación negativa de `type`, `isRead`, `limit` y cursor con 400 `INVALID_REQUEST`; E061 no define 422 en OpenAPI, por lo que no se emite un 422 artificial.
- [x] Pruebas positivas y negativas en capa aplicación, adaptador y HTTP.
- [ ] Verificación por HTTP real tras despliegue autorizado: **BLOCKED_AWS**.
