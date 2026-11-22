# Checklist de calidad — E058

**Estado:** `CODE_IMPLEMENTADO_LOCAL=SI` · `CI_LOCAL=PASS` · `CI_LINUX=PENDIENTE` · `AWS_DEPLOYED=NO` · `HTTPS_VERIFIED=NO` · `FUNCTIONAL_VERIFIED=NO`.

- [x] Método/path conservan el ID del inventario H04 (`PUT /api/v1/notification-preferences`, `nexum_e058`).
- [x] Payload válido/negativo revisado: tres booleanos requeridos, opcionales con defaults, `quietHours` objeto/nulo y `timeZone` IANA; campos desconocidos rechazados por `BindJSONLimit`.
- [x] Respuesta `NotificationPreferences` y errores de OpenAPI implementados (`ErrorEnvelope`, `Cache-Control: private, no-store`).
- [x] Semántica de propiedad: `sub` verificado + sesión autoritativa + item `USER#<sub>/NOTIFICATION_PREFERENCES`; dos sujetos probados.
- [x] Concurrencia resuelta con CAS interno por versión: relectura en conflicto, reintentos acotados y 409 al agotarse. `version` no se acepta del cliente porque OpenAPI no lo declara.
- [x] DynamoDB usa `UpdateItem` condicional por clave exacta, nunca `Scan`; sin recursos ni índices nuevos.
- [x] Redis: limitador real con ventana de 60s y código `RATE_LIMITED` del contrato.
- [x] `quietHours.start/end` con regex `HH:MM`, `start != end`, `enabled` requerido cuando el objeto existe; `timeZone` validada con `ValidarZonaIANA` (base embebida).
- [x] Pruebas de error 400/401/405/409/422/429/500/503 ejecutadas; 403/404 no aplican a preferencias del propio `sub`.
- [x] PII y secretos ausentes en logs, fixtures y respuestas; los mensajes de error no incluyen el valor rechazado.
- [x] Opt-out `pushEnabled=false` prevalece: se persiste, se devuelve y E057 lo relee sin recalcularlo.
- [x] Tests unitarios (3), adaptador Dynamo (3 nuevos) e integración HTTP (5 nuevos) ejecutados; contrato verificado por JSON decode y relectura real.
- [x] `go vet ./...`, `go build ./...` y `go test ./... -count=1` PASS.
- [ ] `go test -race`: **NOT_RUN_LOCAL** — requiere cgo/toolchain C ausente en Windows; CI Linux debe ejecutarlo.
- [ ] CI/CD, dos instancias y TLS público: **NOT_RUN** sin AWS autorizado.
- [ ] Gate individual de release y rollback: **PENDIENTE_APROBACION**; no se declara PASS de despliegue.
- [x] Sin cambios a `spec.md`, `plan.md` ni OpenAPI.

**Cierre:** evidencia local verificable; certificación funcional y de despliegue queda para CI Linux y Gate humano.
