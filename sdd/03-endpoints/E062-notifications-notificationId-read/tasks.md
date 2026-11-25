# Tareas — E062 (`PATCH /api/v1/notifications/{notificationId}/read`)

> Estado: implementación local y evidencia unitaria **COMPLETADAS**. Integración real/AWS/dos nodos **BLOQUEADA** sin Gate H08 ni autorización de despliegue.
>
> Evidencia de ejecución real (2026-11-22):
> `go test ./internal/application/notifications ./internal/adapters/dynamo ./tests/unit/transport ./cmd/server -run 'E061|E062|E063' -count=1 -timeout 300s` → `ok` en los cuatro paquetes.
> `go vet ./internal/application/notifications ./internal/adapters/dynamo ./internal/transport/http ./cmd/server` → sin salida.

## Completado con evidencia local

- [x] `E062-T01` Contrato revisado: `operationId=nexum_e062`, path con `{notificationId}` UUID, respuesta `200 Notification`, sin body de petición.
- [x] `E062-T02` DTO estricto de respuesta (`WireNotification`) con whitelist `id/title/message/isRead/createdAt/unreadCount`; body y query rechazados con 400.
- [x] `E062-T03` Autorización por `sub` del JWT verificado; `notificationId` se busca sólo dentro de `USER#<sub>`.
- [x] `E062-T04` Invariantes: transición atómica `isRead=false → true` con condición DynamoDB; repetición idempotente sin doble decremento; ajeno/inexistente → 404 homogéneo.
- [x] `E062-T05` Adaptador DynamoDB: `Query` acotado por prefijo `NOTIFICATION#` para resolver la SK, `UpdateItem` con `ConditionExpression: #isRead = :falso` y `ReturnValues: ALL_NEW`; relectura `GetItem` si otra petición ganó la carrera. Sin `Scan`.
- [x] `E062-T06` Redis sólo para cuota por usuario (`analytics`), nunca para decidir propiedad o estado de lectura.
- [x] `E062-T07` Errores 400/401/404/409/429/500/503 mapeados a `ErrorEnvelope`; 422 `BUSINESS_RULE_VIOLATION` queda disponible en el contrato pero no se fabrica para una repetición idempotente.
- [x] `E062-T08` Unit tests reales con fixture sintético: idempotencia, ajeno, condición fallback, `ReturnValues=ALL_NEW`, fallos de `UpdateItem` y fail-closed sin cliente/clave.
- [x] `E062-T09` HTTP local: 200 con `isRead=true` y `unreadCount` global, whitelist exacta, 404 para el usuario B, 400 para UUID inválido/body, `Cache-Control` y `ErrorEnvelope`.
- [x] `E062-T10` Negativas locales de auth/ownership/cuota/store caído y verificación de no doble decremento.
- [x] `E062-T11` `gofmt -w` y `go vet` sobre los archivos propios, sin salida de error.

## Bloqueado (requiere autorización humana / H08)

- [ ] `E062-T09b` Integración DynamoDB real y prueba HTTP desde cliente externo: **BLOCKED_AWS**.
- [ ] `E062-T10b` Misma operación en dos nodos Go reales (carrera de marcas de lectura): **BLOCKED_TWO_NODE**.
- [ ] `E062-T11b` CI/seguridad/revisión/rollback con trazas del despliegue: **BLOCKED_GATE_H08**.
- [ ] `E062-T12` Release: **BLOCKED_RELEASE**; código commitado y pusheado en `9b5d950`, pero sin merge, Terraform, AWS ni despliegue; requiere Gate humano.

## Casos concretos

- [x] `E062-CT01` 200 con esquema exacto verificado por HTTP local.
- [x] `E062-CT02` UUID inválido/body -> 400 `INVALID_REQUEST`; sin secretos en respuesta ni logs.
- [x] `E062-CT03` ID del usuario A solicitado por el usuario B -> 404 `NOT_FOUND`, sin filtrar existencia ni datos.
- [x] `E062-CT04` Store caído o clave ausente -> 503; condición fallida inesperada -> 409, nunca confirma escritura parcial.
- [x] `E062-CT05` Dos nodos reales: **BLOCKED_TWO_NODE**; la condición atómica y la relectura están probadas con fake client.
- [x] `E062-CT06` Push/opt-out: **NO APLICA** a E062; este endpoint no envía push.

## Enmienda (A8)

- [x] `unreadCount` global se calcula después de la transición con `CountUnread` y se devuelve en `Notification`.
- [x] Validación negativa de `notificationId` y ausencia de body con 400; 422 no se emite artificialmente porque la operación es idempotente.
- [x] Pruebas positivas y negativas en capa aplicación, adaptador y HTTP.
- [ ] Verificación por HTTP real tras despliegue autorizado: **BLOCKED_AWS**.
