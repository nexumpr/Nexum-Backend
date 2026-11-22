# Checklist de calidad — E063

**Diseño H06:** review. **Implementación y evidencia local:** COMPLETADA. **Respuesta prevista:** 202 `Accepted` con trabajo durable real. **Despliegue/HTTP real:** BLOCKED_AWS / BLOCKED_TWO_NODE.

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
- [ ] Integración DynamoDB real y HTTP desde cliente externo: **BLOCKED_AWS**.
- [ ] API en dos instancias, TLS público, CI/CD e IaC: **BLOCKED_TWO_NODE / BLOCKED_GATE_H08**.
- [ ] Gate individual de release H08 y rollback aprobados antes de `terraform apply`: **BLOCKED_RELEASE**.
- [x] Dependencias del cliente Mobile no constituyen Gate del Backend; no se modificaron.

## Criterios de aceptación

- [x] `202` con `{status:"accepted",requestId:<uuid>}` y trabajo real ejecutado antes de responder.
- [x] Cero, una y cientos de pendientes procesados en páginas de 25, con reanudación tras fallo a mitad.
- [x] `unreadCount` de A queda en 0 y B permanece intacto.
- [x] Reintento concurrente con el mismo `requestId` no duplica ni pierde trabajo.
- [ ] Verificación por HTTP real tras despliegue autorizado: **BLOCKED_AWS**.

**Criterio de cierre:** evidencia local ejecutada y registrada; queda REVIEW hasta aprobación del Gate H08. No se declara AWS_DEPLOYED sin despliegue real autorizado.
