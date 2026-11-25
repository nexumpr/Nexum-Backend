# Checklist de calidad — E063

**Diseño H06:** review. **Implementación y evidencia local:** COMPLETADA. **Respuesta prevista:** 202 `Accepted` con trabajo durable real. **Despliegue/HTTP real:** SI / 401_SIN_JWT.

Ejecución real (2026-11-22):
```
go test ./internal/application/notifications ./internal/adapters/dynamo ./tests/unit/transport ./cmd/server -run 'E061|E062|E063' -count=1 -timeout 300s
ok  	github.com/nexumpr/Nexum-Backend/internal/application/notifications	1.743s
ok  	github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo	1.998s
ok  	github.com/nexumpr/Nexum-Backend/tests/unit/transport	0.288s
ok  	github.com/nexumpr/Nexum-Backend/cmd/server	0.170s [no tests to run]
```
`go vet ./internal/application/notifications ./internal/adapters/dynamo ./internal/transport/http ./cmd/server` → sin salida.

- [x] Método/path conservan `E063` y `operationId=nexum_e063`; sin parámetros ni body.
- [x] Payload válido/negativo y `Accepted` revisados; whitelist exacta `status/requestId`, `requestId` UUID.
- [x] Semántica de propiedad: sujeto JWT y fan-out restringido a `USER#<sub>`; usuario B intacto.
- [x] Idempotencia/reintento: mismo `requestId` reanuda desde `cursor` persistido; `status=completed` final; no resurrece notificaciones.
- [x] Concurrencia: `leaseOwner/leaseUntil` y condiciones en `UpdateItem`; dueño distinto recibe 409 y no avanza el registro.
- [x] DynamoDB usa `Query`/`UpdateItem`/`PutItem` con prefijo `NOTIFICATION#` y paginación de 25; la interfaz del adaptador no expone `Scan`.
- [x] Redis: cuota por usuario `analytics`; el progreso durable vive en DynamoDB, no en Redis ni en memoria.
- [x] Timestamps RFC3339Nano en UTC en `createdAt/leaseUntil/completedAt`.
- [x] Errores cubiertos localmente: 400/401/409/429/503; 500 mapeado; 422 no se fabrica artificialmente.
- [x] PII/secretos ausentes en logs y fixtures; clave HMAC inyectada, nunca efímera.
- [x] Tests unitarios reales, adaptador con fake DynamoDB y HTTP local: PASS.
- [x] El 202 se emite sólo después de persistir READALL y completar todas las páginas; `unreadCount=0` comprobado.
- [ ] Integración funcional con DynamoDB real y cliente autenticado: **NO_EJECUTADA**; `/api/v1/health/ready` reporta `dynamodb=UP` y la ruta responde 401 sin JWT.
- [x] Despliegue real en dos instancias con TLS público e IaC aplicada bajo Gate: release `v0.20.0+4bf6758d372`, LT v19, ASG 2/2/2, refresh `914d3fc7-f808-4865-a744-689cfe749b61` Successful 100 %.
- [ ] CI/CD ejecutado en Linux sobre el SHA exacto: **CI_LINUX=PENDIENTE**.
- [x] Gate humano de release ejercido para el conjunto E052–E063: plan `tfplan-o4b` aplicado con `0 added, 4 changed, 0 destroyed`; rollback preservado (`v1`, `v0.19.2+3098f379d7ab`).
- [ ] Gate individual por E-ID: **NO_EJERCIDO**; se aplicó un Gate agregado de Oleada 4.
- [x] Dependencias del cliente Mobile no constituyen Gate del Backend; no se modificaron.

## Criterios de aceptación

- [x] `202` con `{status:"accepted",requestId:<uuid>}` y trabajo real ejecutado antes de responder.
- [x] Cero, una y cientos de pendientes procesados en páginas de 25, con reanudación tras fallo a mitad.
- [x] `unreadCount` de A queda en 0 y B permanece intacto.
- [x] Reintento concurrente con el mismo `requestId` no duplica ni pierde trabajo.
- [x] Ruta HTTPS desplegada: sin JWT responde 401 en `https://d2lpmqy5w6x9es.cloudfront.net`; ruta inexistente de control responde 404. Verificado tras el despliegue.
- [ ] Prueba funcional con JWT contra AWS: **NO_EJECUTADA**; `FUNCTIONAL_VERIFIED_AWS=NO`.

**Cierre: despliegue y presencia de ruta verificados por HTTP sin JWT; no se declara `FUNCTIONAL_VERIFIED_AWS=SI`. Pendientes: prueba con JWT, `go test -race` y CI Linux. Gate transversal E014/E020 pendiente.**
