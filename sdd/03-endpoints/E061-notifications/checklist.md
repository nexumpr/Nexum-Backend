# Checklist de calidad — E061

**Diseño H06:** review. **Implementación y evidencia local:** COMPLETADA. **Respuesta prevista:** 200. **Despliegue/HTTP real:** SI / 401_SIN_JWT.

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
- [ ] Integración funcional con DynamoDB real y cliente autenticado: **NO_EJECUTADA**; `/api/v1/health/ready` reporta `dynamodb=UP` y la ruta responde 401 sin JWT.
- [x] Despliegue real en dos instancias con TLS público e IaC aplicada bajo Gate: release `v0.20.0+4bf6758d372`, LT v19, ASG 2/2/2, refresh `914d3fc7-f808-4865-a744-689cfe749b61` Successful 100 %.
- [ ] CI/CD ejecutado en Linux sobre el SHA exacto: **CI_LINUX=PENDIENTE**.
- [x] Gate humano de release ejercido para el conjunto E052–E063: plan `tfplan-o4b` aplicado con `0 added, 4 changed, 0 destroyed`; rollback preservado (`v1`, `v0.19.2+3098f379d7ab`).
- [ ] Gate individual por E-ID: **NO_EJERCIDO**; se aplicó un Gate agregado de Oleada 4.
- [x] Dependencias del cliente Mobile no constituyen Gate del Backend; no se modificaron.

## Criterios de aceptación de la enmienda (A8)

- [x] `type` aparece en OpenAPI y se valida con el mismo enum en HTTP y aplicación.
- [x] `unreadCount` se devuelve en el listado con tipo `integer` y valor global.
- [x] Los filtros se aplican en la consulta y se reflejan al releer las páginas.
- [x] Validación negativa de `type/isRead/limit/cursor` → 400 sin filtrar datos internos.
- [x] Ruta HTTPS desplegada: sin JWT responde 401 en `https://d2lpmqy5w6x9es.cloudfront.net`; ruta inexistente de control responde 404. Verificado tras el despliegue.
- [ ] Prueba funcional con JWT contra AWS: **NO_EJECUTADA**; `FUNCTIONAL_VERIFIED_AWS=NO`.

**Cierre: despliegue y presencia de ruta verificados por HTTP sin JWT; no se declara `FUNCTIONAL_VERIFIED_AWS=SI`. Pendientes: prueba con JWT, `go test -race` y CI Linux. Gate transversal E014/E020 pendiente.**
