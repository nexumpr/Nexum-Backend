# Tareas — E063 (`POST /api/v1/notifications/read-all`)

> Estado: implementación local y evidencia unitaria **COMPLETADAS**. Integración real/AWS/dos nodos **BLOQUEADA** sin Gate H08 ni autorización de despliegue.
>
> Evidencia de ejecución real (2026-11-22):
> `go test ./internal/application/notifications ./internal/adapters/dynamo ./tests/unit/transport ./cmd/server -run 'E061|E062|E063' -count=1 -timeout 300s` → `ok` en los cuatro paquetes.
> `go vet ./internal/application/notifications ./internal/adapters/dynamo ./internal/transport/http ./cmd/server` → sin salida.

## Completado con evidencia local

- [x] `E063-T01` Contrato revisado: `operationId=nexum_e063`, `POST /api/v1/notifications/read-all`, sin parámetros ni body, `202 Accepted` con `status/requestId`.
- [x] `E063-T02` DTO estricto (`WireAccepted`) con whitelist exacta `status/requestId`; query y body rechazados con 400.
- [x] `E063-T03` Autorización por `sub` del JWT verificado; el fan-out se restringe a `USER#<sub>`.
- [x] `E063-T04` Invariantes: trabajo durable real antes del 202; páginas de `ReadAllBatchSize=25` con cursor y lease; `unreadCount` final 0 para A, B intacto; reintento con el mismo `requestId` reanuda. Sin `Scan`.
- [x] `E063-T05` Adaptador DynamoDB: registro `USER#<sub> / READALL#<requestId>` con `entityType=READALL`, `status`, `createdAt`, `processedCount`, `initialUnread`, `cursor`, `leaseOwner`, `leaseUntil`, `completedAt`; `PutItem` condicional, `UpdateItem` de avance/cierre con condiciones de lease. Sin `Scan`.
- [x] `E063-T06` Redis sólo para cuota por usuario (`analytics`); no guarda progreso ni autoriza la operación.
- [x] `E063-T07` Errores 400/401/409/429/500/503 mapeados a `ErrorEnvelope`; 422 reservado por contrato, no simulado; 202 sólo tras completar el trabajo persistido.
- [x] `E063-T08` Unit tests reales con fixture sintético: 0, 1 y 137 pendientes, fallo a mitad + reanudación con el mismo `requestId`, usuario B intacto, `processedCount` derivado del diferencial de no leídas y fail-closed.
- [x] `E063-T09` HTTP local: 202 con `requestId` UUID, trabajo real comprobado (`unreadCount=0` para A, B=4), segundo 202 seguro, `Cache-Control` y `ErrorEnvelope`.
- [x] `E063-T10` Negativas locales de auth/cuota/store caído/body/query y carrera de lease con fake client.
- [x] `E063-T11` `gofmt -w` y `go vet` sobre los archivos propios, sin salida de error.

## Bloqueado (requiere autorización humana / H08)

- [ ] `E063-T09b` Integración DynamoDB real y prueba HTTP desde cliente externo: **BLOCKED_AWS**.
- [ ] `E063-T10b` Misma operación en dos nodos Go reales (lease distribuido): **BLOCKED_TWO_NODE**.
- [ ] `E063-T11b` CI/seguridad/revisión/rollback con trazas del despliegue: **BLOCKED_GATE_H08**.
- [ ] `E063-T12` Release: **BLOCKED_RELEASE**; código commitado y pusheado en `9b5d950`, pero sin merge, Terraform, AWS ni despliegue; requiere Gate humano.

## Casos concretos

- [x] `E063-CT01` 202 con esquema exacto verificado por HTTP local.
- [x] `E063-CT02` Body/query inesperados -> 400 `INVALID_REQUEST`; sin secretos en respuesta ni logs.
- [x] `E063-CT03` Usuario B no es alcanzado por el fan-out de A; verificado con dos sujetos.
- [x] `E063-CT04` Store caído o clave ausente -> 503; condición de lease en conflicto -> 409; nunca se confirma ack falso.
- [x] `E063-CT05` Dos nodos reales: **BLOCKED_TWO_NODE**; el lease y las condiciones están probados con fake client.
- [x] `E063-CT06` Push/opt-out: **NO APLICA** a E063; este endpoint no envía push.

## Enmienda / alcance E063

- [x] El `202` no es un ack simulado: el registro READALL se persiste y todas las páginas se marcan antes de responder.
- [x] Reintento con el mismo `requestId` reanuda desde el cursor persistido y termina con `status=completed`.
- [x] Pruebas positivas y negativas en capa aplicación, adaptador y HTTP.
- [ ] Verificación por HTTP real tras despliegue autorizado: **BLOCKED_AWS**.
