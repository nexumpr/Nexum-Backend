# Checklist de calidad — E059 (implementación local)

**Diseño H06:** review (contrato existente, no modificado). **Implementación local:** PASS. **Respuesta prevista:** 201. **AWS/despliegue:** NOT_RUN / BLOQUEADO.

- [ ] Firma del Gate H06 por el responsable; no confundir instalación con aprobación. Fuera del alcance de este subagente.
- [x] Método/path conservan el ID del inventario H04: `POST /api/v1/devices/push-tokens`, `operationId=nexum_e059` (OpenAPI sin cambios).
- [x] Payload válido/negativo, ejemplo y JSON Schema `PushTokenRegister` revisados: `provider=fcm`, `token>=8`, `deviceLabel<=80`, `additionalProperties:false`; caso `campo_extra` → 400.
- [x] Respuesta `PushDevice` y errores de OpenAPI revisados: exactamente `id`, `provider`, `deviceLabel`, `registeredAt`; `Location` y ErrorEnvelope en `TestE059HTTPContratoValido` / `TestE059HTTPNegativas`.
- [x] Semántica de propiedad y JWT: `UserIDFromContext`; partición `USER#<sub>` en todas las operaciones; sin excepción pública.
- [x] Idempotencia obligatoria y concurrencia resueltas: `IDEM#PUSH#<sha256(clave)>`, payloadHash SHA-256 canónico, condición transaccional y resolución replay/409.
- [x] DynamoDB tiene GetItem/Query/Transact/UpdateItem sobre la partición del usuario, **nunca Scan**: ver `push_devices_store.go` y `TestE059AdaptadorQueryPorEtiquetaSinScan`.
- [x] Redis previsto para cuota por usuario con `NewAuthRateLimiter(..., "analytics", ...)`; TTL de idempotencia en Dynamo; fallo cerrado sin sealer/store (503). Servicio Redis real: NOT_RUN.
- [x] Timestamps UTC/RFC3339Nano (`registeredAt`, `expiresAt` epoch, `revokedAt`); sin montos en este contrato.
- [x] Pruebas locales de 400/401/404/409/429/503; 403/422 y 500 quedan contempladas en el mapa de errores del handler, sin caso sintético exigido por el contrato de este endpoint.
- [x] PII y secretos ausentes en logs, capturas y fixture: claves AES sintéticas en memoria, tokens de prueba no reales, nunca impresos; respuestas sin token/cipher/usuario.
- [x] Tests unitarios reales, contrato OpenAPI local e integración Dynamo con fake client: PASS (comando abajo). AWS real: NOT_RUN.
- [ ] API en dos instancias, TLS público, CI/CD e IaC: **BLOQUEADO sin AWS**.
- [ ] Gate individual de release H08 y rollback aprobados antes de `terraform apply`: **BLOQUEADO sin AWS**.
- [ ] Dependencias del cliente Mobile nunca constituyen Gate del Backend: reverificado documentalmente; aprobación móvil ajena.

**Evidencia real:** `go test ./internal/application/pushdevices ./internal/adapters/dynamo ./tests/unit/transport ./cmd/server -run 'E059|E060' -count=1 -timeout 300s` → PASS en los cuatro paquetes (`cmd/server`: sin tests propios).

**Criterio de cierre:** la documentación contractual no se modifica; esta implementación queda REVIEW hasta aprobación humana y evidencia de despliegue real. No se afirma `AWS_DEPLOYED`.
