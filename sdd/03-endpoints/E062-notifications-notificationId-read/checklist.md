# Checklist de calidad — E062

**Diseño H06:** review. **Implementación y evidencia local:** COMPLETADA. **Respuesta prevista:** 200 con `isRead=true`. **Despliegue/HTTP real:** SI / 401_SIN_JWT.

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
- [ ] Integración funcional con DynamoDB real y cliente autenticado: **NO_EJECUTADA**; `/api/v1/health/ready` reporta `dynamodb=UP` y la ruta responde 401 sin JWT.
- [x] Despliegue real en dos instancias con TLS público e IaC aplicada bajo Gate: release `v0.20.0+4bf6758d372`, LT v19, ASG 2/2/2, refresh `914d3fc7-f808-4865-a744-689cfe749b61` Successful 100 %.
- [ ] CI/CD ejecutado en Linux sobre el SHA exacto: **CI_LINUX=PENDIENTE**.
- [x] Gate humano de release ejercido para el conjunto E052–E063: plan `tfplan-o4b` aplicado con `0 added, 4 changed, 0 destroyed`; rollback preservado (`v1`, `v0.19.2+3098f379d7ab`).
- [ ] Gate individual por E-ID: **NO_EJERCIDO**; se aplicó un Gate agregado de Oleada 4.
- [x] Dependencias del cliente Mobile no constituyen Gate del Backend; no se modificaron.

## Criterios de aceptación de la enmienda (A8)

- [x] `unreadCount` aparece en la respuesta con el mismo tipo que en `PageOfNotification`.
- [x] Se persiste el cambio real de `isRead` y se devuelve `true` tras la transición.
- [x] Se consulta el detalle con la misma entidad `Notification` que el listado, sin campos extra.
- [x] Ruta HTTPS desplegada: sin JWT responde 401 en `https://d2lpmqy5w6x9es.cloudfront.net`; ruta inexistente de control responde 404. Verificado tras el despliegue.
- [ ] Prueba funcional con JWT contra AWS: **NO_EJECUTADA**; `FUNCTIONAL_VERIFIED_AWS=NO`.

**Cierre: despliegue y presencia de ruta verificados por HTTP sin JWT; no se declara `FUNCTIONAL_VERIFIED_AWS=SI`. Pendientes: prueba con JWT, `go test -race` y CI Linux. Gate transversal E014/E020 pendiente.**
