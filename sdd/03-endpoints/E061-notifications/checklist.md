# Checklist de calidad — E061

**Diseño H06:** review. **Implementación y evidencia local:** COMPLETADA. **Respuesta prevista:** 200. **Despliegue/HTTP real:** BLOCKED_AWS / BLOCKED_TWO_NODE.

Ejecución real (2026-11-22):
```
go test ./internal/application/notifications ./internal/adapters/dynamo ./tests/unit/transport ./cmd/server -run 'E061|E062|E063' -count=1 -timeout 300s
ok  	github.com/nexumpr/Nexum-Backend/internal/application/notifications	1.743s
ok  	github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo	1.998s
ok  	github.com/nexumpr/Nexum-Backend/tests/unit/transport	0.288s
ok  	github.com/nexumpr/Nexum-Backend/cmd/server	0.170s [no tests to run]
```
`go vet ./internal/application/notifications ./internal/adapters/dynamo ./internal/transport/http ./cmd/server` → sin salida.

- [x] Método/path conservan `E061` y `operationId=nexum_e061` del inventario/OpenAPI.
- [x] Payload válido/negativo y `PageOfNotification` revisados contra OpenAPI; respuesta sin campos extra y `items` siempre arreglo.
- [x] Semántica de propiedad: `sub` verificado por middleware; `USER#<sub>` en `Query`, sin `Scan` y sin fuga cross-user.
- [x] Cursor firmado HMAC-SHA256: caducidad no requerida, pero ligado a identidad, filtros, límite y `LastEvaluatedKey`; tampered/cross-user → 400.
- [x] `unreadCount` global por `Query/Select COUNT`, no derivado de la página.
- [x] Concurrencia e idempotencia: E061 es sólo lectura; paginación estable por clave, sin duplicados.
- [x] DynamoDB usa `Query` con `begins_with` y `ExclusiveStartKey`; la interfaz del adaptador no expone `Scan`.
- [x] Redis: cuota por usuario `analytics`; fallo del limitador no concede privilegios ni datos.
- [x] Timestamps RFC3339Nano en UTC en claves y proyección.
- [x] Errores pertinentes cubiertos localmente: 400/401/404/429/503; 403/500 mapeados en el traductor, 422 no aplicable a E061.
- [x] PII/secretos ausentes en logs y fixtures; clave de firma inyectada, nunca generada en caliente.
- [x] Tests unitarios reales, adaptador con fake DynamoDB y HTTP local: PASS.
- [ ] Integración DynamoDB real y HTTP desde cliente externo: **BLOCKED_AWS**.
- [ ] API en dos instancias, TLS público, CI/CD e IaC: **BLOCKED_TWO_NODE / BLOCKED_GATE_H08**.
- [ ] Gate individual de release H08 y rollback aprobados antes de `terraform apply`: **BLOCKED_RELEASE**.
- [x] Dependencias del cliente Mobile no constituyen Gate del Backend; no se modificaron.

## Criterios de aceptación de la enmienda (A8)

- [x] `type` aparece en OpenAPI y se valida con el mismo enum en HTTP y aplicación.
- [x] `unreadCount` se devuelve en el listado con tipo `integer` y valor global.
- [x] Los filtros se aplican en la consulta y se reflejan al releer las páginas.
- [x] Validación negativa de `type/isRead/limit/cursor` → 400 sin filtrar datos internos.
- [ ] Verificación por HTTP real tras despliegue autorizado: **BLOCKED_AWS**.

**Criterio de cierre:** la evidencia local está ejecutada y registrada; la documentación contractual queda REVIEW hasta aprobación del Gate H08. No se marca AWS_DEPLOYED ni PASS de despliegue sin ejecución real autorizada.
