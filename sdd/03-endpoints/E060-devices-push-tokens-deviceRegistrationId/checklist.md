# Checklist de calidad — E060 (implementación local)

**Diseño H06:** review (contrato existente, no modificado). **Implementación local:** PASS. **Respuesta prevista:** 204. **AWS/despliegue:** SI (release `v0.20.0+4bf6758d372`, ruta HTTPS 401 sin JWT).

- [ ] Firma del Gate H06 por el responsable; no confundir instalación con aprobación. Pendiente; el código quedó en `9b5d950` y no se declara Gate firmado.
- [x] Método/path conservan el ID del inventario H04: `DELETE /api/v1/devices/push-tokens/{deviceRegistrationId}`, `operationId=nexum_e060` (OpenAPI sin cambios).
- [x] Payload nulo revisado: sin cuerpo, sin lectura de JSON; `deviceRegistrationId` validado como UUID canónico y extraído de `PathValue`/último segmento, nunca de query o cabecera.
- [x] Respuesta `204` sin cuerpo ni `Location` y errores de OpenAPI revisados: `TestE060HTTPBajaExitosaEIdempotente`, `TestE060HTTPNegativas`, `TestE060HTTPFailClosed`.
- [x] Semántica de propiedad y JWT: todas las escrituras usan `USER#<sub>/DEVICE#<id>`; ajeno e inexistente devuelven el mismo `NOT_FOUND`; ver `TestE060HTTPAjenoInexistenteYAislamientoAB`.
- [x] Idempotencia de baja: `UpdateItem` condicional `active = true`; repetir baja propia devuelve 204; relectura ante carrera. No se exige `Idempotency-Key` en este contrato.
- [x] DynamoDB usa GetItem/UpdateItem sobre la partición del usuario, **nunca Scan**; sin tablas ni índices nuevos.
- [x] Redis previsto para cuota por usuario con `NewAuthRateLimiter(..., "analytics", ...)`; fallo cerrado del servicio principal: 503. Servicio Redis real: verificado en AWS (ítem siguiente).
- [x] Servicio Redis real verificado en AWS (`/api/v1/health/ready` → `redis=UP`); prueba de cuota con JWT no ejecutada.
- [x] `revokedAt` en RFC3339Nano; sin montos en este contrato.
- [x] Pruebas locales de 400/401/404/429/503; 403/409/422 y 500 contemplados en el mapa de errores del handler, sin caso sintético exigido por este endpoint.
- [x] PII y secretos ausentes en logs, capturas y fixture: sobre cifrado y huella se retiran del item; ningún token, usuario ni clave se imprime.
- [x] Tests unitarios reales, contrato OpenAPI local e integración Dynamo con fake client: PASS (comando abajo). AWS real: desplegado (release v0.20.0+4bf6758d372); prueba funcional con JWT no ejecutada.
- [x] Despliegue real en dos instancias con TLS público e IaC aplicada bajo Gate: release `v0.20.0+4bf6758d372`, LT v19, ASG 2/2/2, refresh `914d3fc7-f808-4865-a744-689cfe749b61` Successful 100 %.
- [ ] CI/CD ejecutado en Linux sobre el SHA exacto: **CI_LINUX=PENDIENTE**.
- [x] Gate humano de release ejercido para el conjunto E052–E063: plan `tfplan-o4b` aplicado con `0 added, 4 changed, 0 destroyed`; rollback preservado (`v1`, `v0.19.2+3098f379d7ab`).
- [ ] Gate individual por E-ID: **NO_EJERCIDO**; se aplicó un Gate agregado de Oleada 4.
- [x] Dependencias del cliente Mobile nunca constituyen Gate del Backend: reverificado documentalmente; aprobación móvil ajena.

**Evidencia real:** `go test ./internal/application/pushdevices ./internal/adapters/dynamo ./tests/unit/transport ./cmd/server -run 'E059|E060' -count=1 -timeout 300s` → PASS en los cuatro paquetes (`cmd/server`: sin tests propios).

- [x] Ruta HTTPS desplegada: sin JWT responde 401 en `https://d2lpmqy5w6x9es.cloudfront.net`; ruta inexistente de control responde 404. Verificado tras el despliegue.
- [ ] Prueba funcional con JWT contra AWS: **NO_EJECUTADA**; `FUNCTIONAL_VERIFIED_AWS=NO`.
**Cierre: despliegue y presencia de ruta verificados por HTTP sin JWT; no se declara `FUNCTIONAL_VERIFIED_AWS=SI`. Pendientes: prueba con JWT, `go test -race` y CI Linux. Gate H06 sin firma y FCM E2E con dispositivo no ejecutado.**
