# Checklist de calidad — E058

**Estado:** `CODE_IMPLEMENTADO_LOCAL=SI` · `CI_LOCAL=PASS` · `CI_LINUX=PENDIENTE` · `AWS_DEPLOYED=SI` · `HTTPS_RUTA_PRESENTE=SI` · `HTTPS_FUNCIONAL_CON_JWT=NO` · `FUNCTIONAL_VERIFIED_AWS=NO` · `RELEASE=v0.20.0+4bf6758d372`.

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
- [x] Despliegue real en dos instancias con TLS público e IaC aplicada bajo Gate: release `v0.20.0+4bf6758d372`, LT v19, ASG 2/2/2, refresh `914d3fc7-f808-4865-a744-689cfe749b61` Successful 100 %.
- [ ] CI/CD ejecutado en Linux sobre el SHA exacto: **CI_LINUX=PENDIENTE**.
- [x] Gate humano de release ejercido para el conjunto E052–E063: plan `tfplan-o4b` aplicado con `0 added, 4 changed, 0 destroyed`; rollback preservado (`v1`, `v0.19.2+3098f379d7ab`).
- [ ] Gate individual por E-ID: **NO_EJERCIDO**; se aplicó un Gate agregado de Oleada 4.
- [x] Sin cambios a `spec.md`, `plan.md` ni OpenAPI.

- [x] Ruta HTTPS desplegada: sin JWT responde 401 en `https://d2lpmqy5w6x9es.cloudfront.net`; ruta inexistente de control responde 404. Verificado tras el despliegue.
- [ ] Prueba funcional con JWT contra AWS: **NO_EJECUTADA**; `FUNCTIONAL_VERIFIED_AWS=NO`.
**Cierre: despliegue y presencia de ruta verificados por HTTP sin JWT; no se declara `FUNCTIONAL_VERIFIED_AWS=SI`. Pendientes: prueba con JWT, `go test -race` y CI Linux.**
