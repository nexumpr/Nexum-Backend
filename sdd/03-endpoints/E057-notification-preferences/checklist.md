# Checklist de calidad — E057

**Estado:** `CODE_IMPLEMENTADO_LOCAL=SI` · `CI_LOCAL=PASS` · `CI_LINUX=PENDIENTE` · `AWS_DEPLOYED=NO` · `HTTPS_VERIFIED=NO` · `FUNCTIONAL_VERIFIED=NO`.

- [x] Método/path conservan el ID del inventario H04 (`GET /api/v1/notification-preferences`, `nexum_e057`).
- [x] Payload válido/negativo revisado: sin parámetros ni cuerpo; no hay campos desconocidos que rechazar en GET.
- [x] Respuesta `NotificationPreferences` y errores de OpenAPI implementados (`ErrorEnvelope`, `Cache-Control: private, no-store`).
- [x] Semántica de propiedad: `sub` verificado + sesión autoritativa + item `USER#<sub>/NOTIFICATION_PREFERENCES`; dos sujetos probados.
- [x] Idempotencia de la materialización de defaults: creación condicional; la segunda lectura no incrementa versión ni falla.
- [x] DynamoDB usa `GetItem` consistente y `PutItem` condicional por clave exacta, nunca `Scan`; sin recursos ni índices nuevos.
- [x] Redis: limitador real con ventana de 60s y código `RATE_LIMITED` del contrato.
- [x] Zona IANA `timeZone` devuelta tal como se persistió; `quietHours` conserva `enabled=false` cuando el objeto existe.
- [x] Pruebas de error 400/401/405/429/500/503 ejecutadas; 403/404 no aplican a lectura agregada sin ID de recurso; 409 se reserva a E058; 422 no existe en GET y no se inventó.
- [x] PII y secretos ausentes en logs, fixtures y respuestas; la respuesta no expone el `sub`.
- [x] Defaults sin registro devueltos y persistidos; `pushEnabled=false`, `emailEnabled=false`, `reminderEnabled=true`, `version=1`, granular RFC-016.
- [x] Tests unitarios reales (3), adaptador Dynamo (5) e integración HTTP local (4) ejecutados; contrato verificado por JSON decode y whitelist de campos.
- [x] `go vet ./...`, `go build ./...` y `go test ./... -count=1` PASS.
- [ ] `go test -race`: **NOT_RUN_LOCAL** — requiere cgo/toolchain C ausente en Windows; CI Linux debe ejecutarlo.
- [ ] CI/CD, dos instancias y TLS público: **NOT_RUN** sin AWS autorizado.
- [ ] Gate individual de release y rollback: **PENDIENTE_APROBACION**; no se declara PASS de despliegue.
- [x] Sin cambios a `spec.md`, `plan.md` ni OpenAPI.

**Cierre:** evidencia local verificable; certificación funcional y de despliegue queda para CI Linux y Gate humano.
