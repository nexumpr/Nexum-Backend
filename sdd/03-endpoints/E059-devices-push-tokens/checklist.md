# Checklist de calidad — E059 (implementación local)

**Diseño H06:** review (contrato existente, no modificado). **Implementación local:** PASS. **Respuesta prevista:** 201. **AWS/despliegue:** SI (release `v0.20.0+4bf6758d372`, ruta HTTPS 401 sin JWT).

- [ ] Firma del Gate H06 por el responsable; no confundir instalación con aprobación. Pendiente; el código quedó en `9b5d950` y no se declara Gate firmado.
- [x] Método/path conservan el ID del inventario H04: `POST /api/v1/devices/push-tokens`, `operationId=nexum_e059` (OpenAPI sin cambios).
- [x] Payload válido/negativo, ejemplo y JSON Schema `PushTokenRegister` revisados: `provider=fcm`, `token>=8`, `deviceLabel<=80`, `additionalProperties:false`; caso `campo_extra` → 400.
- [x] Respuesta `PushDevice` y errores de OpenAPI revisados: exactamente `id`, `provider`, `deviceLabel`, `registeredAt`; `Location` y ErrorEnvelope en `TestE059HTTPContratoValido` / `TestE059HTTPNegativas`.
- [x] Semántica de propiedad y JWT: `UserIDFromContext`; partición `USER#<sub>` en todas las operaciones; sin excepción pública.
- [x] Idempotencia obligatoria y concurrencia resueltas: `IDEM#PUSH#<sha256(clave)>`, payloadHash SHA-256 canónico, condición transaccional y resolución replay/409.
- [x] DynamoDB tiene GetItem/Query/Transact/UpdateItem sobre la partición del usuario, **nunca Scan**: ver `push_devices_store.go` y `TestE059AdaptadorQueryPorEtiquetaSinScan`.
- [x] Redis previsto para cuota por usuario con `NewAuthRateLimiter(..., "analytics", ...)`; TTL de idempotencia en Dynamo; fallo cerrado sin sealer/store (503). Servicio Redis real: verificado en AWS (ítem siguiente).
- [x] Servicio Redis real verificado en AWS (`/api/v1/health/ready` → `redis=UP`); prueba de cuota con JWT no ejecutada.
- [x] Timestamps UTC/RFC3339Nano (`registeredAt`, `expiresAt` epoch, `revokedAt`); sin montos en este contrato.
- [x] Pruebas locales de 400/401/404/409/429/503; 403/422 y 500 quedan contempladas en el mapa de errores del handler, sin caso sintético exigido por el contrato de este endpoint.
- [x] PII y secretos ausentes en logs, capturas y fixture: claves AES sintéticas en memoria, tokens de prueba no reales, nunca impresos; respuestas sin token/cipher/usuario.
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
