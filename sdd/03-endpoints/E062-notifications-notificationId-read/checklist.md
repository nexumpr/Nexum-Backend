# Checklist de calidad — E062

**Diseño H06:** review. **Implementación y evidencia local:** COMPLETADA. **Respuesta prevista:** 200 con `isRead=true`. **Despliegue/HTTP real:** BLOCKED_AWS / BLOCKED_TWO_NODE.

> Nota de contrato: el ejemplo `200` del OpenAPI muestra `isRead: false`, coherente con el estado previo, pero la semántica de E062 y la instrucción humana vigente exigen devolver el item real ya leído (`isRead=true`). Se implementa y se prueba ese valor; no se modifica el OpenAPI.

Ejecución real (2026-11-22):
```
go test ./internal/application/notifications ./internal/adapters/dynamo ./tests/unit/transport ./cmd/server -run 'E061|E062|E063' -count=1 -timeout 300s
ok  	github.com/nexumpr/Nexum-Backend/internal/application/notifications	1.743s
ok  	github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo	1.998s
ok  	github.com/nexumpr/Nexum-Backend/tests/unit/transport	0.288s
ok  	github.com/nexumpr/Nexum-Backend/cmd/server	0.170s [no tests to run]
```
`go vet ./internal/application/notifications ./internal/adapters/dynamo ./internal/transport/http ./cmd/server` → sin salida.

- [x] Método/path conservan `E062` y `operationId=nexum_e062`.
- [x] Payload válido/negativo y `Notification` revisados; sin body de petición; whitelist exacta de campos de respuesta.
- [x] Semántica de propiedad: sujeto JWT y búsqueda por `USER#<sub>`; ID ajeno/inexistente -> 404 sin filtrar existencia.
- [x] Idempotencia real: condición `#isRead = :falso`, `AllNew`; repetición devuelve el item ya leído y `unreadCount` no se decrementa dos veces.
- [x] Concurrencia: condición atómica y relectura `GetItem` ante `ConditionalCheckFailedException`; carreras probadas con fake client.
- [x] DynamoDB usa `Query` acotado y `UpdateItem`/`GetItem`; la interfaz del adaptador no expone `Scan`.
- [x] Redis: cuota por usuario `analytics`; no interviene en propiedad ni en el estado `isRead`.
- [x] Timestamps RFC3339Nano en UTC (`createdAt` y `readAt` interno).
- [x] Errores cubiertos localmente: 400/401/404/409/429/503; 500 mapeado; 422 no se fabrica para una operación idempotente.
- [x] PII/secretos ausentes en logs y fixtures; clave HMAC de cursores inyectada, nunca efímera.
- [x] Tests unitarios reales, adaptador con fake DynamoDB y HTTP local: PASS.
- [ ] Integración DynamoDB real y HTTP desde cliente externo: **BLOCKED_AWS**.
- [ ] API en dos instancias, TLS público, CI/CD e IaC: **BLOCKED_TWO_NODE / BLOCKED_GATE_H08**.
- [ ] Gate individual de release H08 y rollback aprobados antes de `terraform apply`: **BLOCKED_RELEASE**.
- [x] Dependencias del cliente Mobile no constituyen Gate del Backend; no se modificaron.

## Criterios de aceptación de la enmienda (A8)

- [x] `unreadCount` aparece en la respuesta con el mismo tipo que en `PageOfNotification`.
- [x] Se persiste el cambio real de `isRead` y se devuelve `true` tras la transición.
- [x] Se consulta el detalle con la misma entidad `Notification` que el listado, sin campos extra.
- [ ] Verificación por HTTP real tras despliegue autorizado: **BLOCKED_AWS**.

**Criterio de cierre:** evidencia local ejecutada y registrada; queda REVIEW hasta aprobación del Gate H08. No se declara AWS_DEPLOYED sin despliegue real autorizado.
