# Checklist de calidad — E057

**Estado:** `CODE_IMPLEMENTADO_LOCAL=SI` · `CI_LOCAL=PASS` · `CI_LINUX=PENDIENTE` · `AWS_DEPLOYED=SI` · `HTTPS_RUTA_PRESENTE=SI` · `HTTPS_FUNCIONAL_CON_JWT=NO` · `FUNCTIONAL_VERIFIED_AWS=NO` · `RELEASE=v0.20.0+4bf6758d372`.

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
- [x] Despliegue real en dos instancias con TLS público e IaC aplicada bajo Gate: release `v0.20.0+4bf6758d372`, LT v19, ASG 2/2/2, refresh `914d3fc7-f808-4865-a744-689cfe749b61` Successful 100 %.
- [ ] CI/CD ejecutado en Linux sobre el SHA exacto: **CI_LINUX=PENDIENTE**.
- [x] Gate humano de release ejercido para el conjunto E052–E063: plan `tfplan-o4b` aplicado con `0 added, 4 changed, 0 destroyed`; rollback preservado (`v1`, `v0.19.2+3098f379d7ab`).
- [ ] Gate individual por E-ID: **NO_EJERCIDO**; se aplicó un Gate agregado de Oleada 4.
- [x] Sin cambios a `spec.md`, `plan.md` ni OpenAPI.

- [x] Ruta HTTPS desplegada: sin JWT responde 401 en `https://d2lpmqy5w6x9es.cloudfront.net`; ruta inexistente de control responde 404. Verificado tras el despliegue.
- [ ] Prueba funcional con JWT contra AWS: **NO_EJECUTADA**; `FUNCTIONAL_VERIFIED_AWS=NO`.
**Cierre: despliegue y presencia de ruta verificados por HTTP sin JWT; no se declara `FUNCTIONAL_VERIFIED_AWS=SI`. Pendientes: prueba con JWT, `go test -race` y CI Linux.**
