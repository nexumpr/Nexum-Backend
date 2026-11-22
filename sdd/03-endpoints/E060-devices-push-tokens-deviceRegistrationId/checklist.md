# Checklist de calidad — E060 (implementación local)

**Diseño H06:** review (contrato existente, no modificado). **Implementación local:** PASS. **Respuesta prevista:** 204. **AWS/despliegue:** NOT_RUN / BLOQUEADO.

- [ ] Firma del Gate H06 por el responsable; no confundir instalación con aprobación. Pendiente; el código quedó en `9b5d950` y no se declara Gate firmado.
- [x] Método/path conservan el ID del inventario H04: `DELETE /api/v1/devices/push-tokens/{deviceRegistrationId}`, `operationId=nexum_e060` (OpenAPI sin cambios).
- [x] Payload nulo revisado: sin cuerpo, sin lectura de JSON; `deviceRegistrationId` validado como UUID canónico y extraído de `PathValue`/último segmento, nunca de query o cabecera.
- [x] Respuesta `204` sin cuerpo ni `Location` y errores de OpenAPI revisados: `TestE060HTTPBajaExitosaEIdempotente`, `TestE060HTTPNegativas`, `TestE060HTTPFailClosed`.
- [x] Semántica de propiedad y JWT: todas las escrituras usan `USER#<sub>/DEVICE#<id>`; ajeno e inexistente devuelven el mismo `NOT_FOUND`; ver `TestE060HTTPAjenoInexistenteYAislamientoAB`.
- [x] Idempotencia de baja: `UpdateItem` condicional `active = true`; repetir baja propia devuelve 204; relectura ante carrera. No se exige `Idempotency-Key` en este contrato.
- [x] DynamoDB usa GetItem/UpdateItem sobre la partición del usuario, **nunca Scan**; sin tablas ni índices nuevos.
- [x] Redis previsto para cuota por usuario con `NewAuthRateLimiter(..., "analytics", ...)`; fallo cerrado del servicio principal: 503. Servicio Redis real: NOT_RUN.
- [x] `revokedAt` en RFC3339Nano; sin montos en este contrato.
- [x] Pruebas locales de 400/401/404/429/503; 403/409/422 y 500 contemplados en el mapa de errores del handler, sin caso sintético exigido por este endpoint.
- [x] PII y secretos ausentes en logs, capturas y fixture: sobre cifrado y huella se retiran del item; ningún token, usuario ni clave se imprime.
- [x] Tests unitarios reales, contrato OpenAPI local e integración Dynamo con fake client: PASS (comando abajo). AWS real: NOT_RUN.
- [ ] API en dos instancias, TLS público, CI/CD e IaC: **BLOQUEADO sin AWS**.
- [ ] Gate individual de release H08 y rollback aprobados antes de `terraform apply`: **BLOQUEADO sin AWS**.
- [ ] Dependencias del cliente Mobile nunca constituyen Gate del Backend: reverificado documentalmente; aprobación móvil ajena.

**Evidencia real:** `go test ./internal/application/pushdevices ./internal/adapters/dynamo ./tests/unit/transport ./cmd/server -run 'E059|E060' -count=1 -timeout 300s` → PASS en los cuatro paquetes (`cmd/server`: sin tests propios).

**Criterio de cierre:** la documentación contractual no se modifica; esta implementación queda REVIEW hasta aprobación humana y evidencia de despliegue real. No se afirma `AWS_DEPLOYED`.
