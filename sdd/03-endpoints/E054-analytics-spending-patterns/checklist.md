# Checklist de calidad — E054

**Estado:** `CODE_IMPLEMENTADO_LOCAL=SI` · `CI_LOCAL=PASS` · `CI_LINUX=PENDIENTE` · `AWS_DEPLOYED=SI` · `HTTPS_RUTA_PRESENTE=SI` · `HTTPS_FUNCIONAL_CON_JWT=NO` · `FUNCTIONAL_VERIFIED_AWS=NO` · `RELEASE=v0.20.0+4bf6758d372`.

- [x] Método/path conservan el ID del inventario H04 (`GET /api/v1/analytics/spending-patterns`, `nexum_e054`).
- [x] Payload válido/negativo revisado: sin cuerpo; `period` validado con `^\d{4}-(0[1-9]|1[0-2])$`.
- [x] Respuesta `PageOfSpendingPattern` y errores de OpenAPI implementados (`ErrorEnvelope`, `Cache-Control`).
- [x] Semántica de propiedad: `sub` verificado + sesión autoritativa + consulta `USER#<sub>`; dos sujetos probados.
- [x] Idempotencia no exigida; lectura sin efectos; cuota por usuario con orden correcto.
- [x] DynamoDB usa `Query` acotada, nunca `Scan`; sin recursos ni índices nuevos.
- [x] Redis: limitador real con ventana de 60s y código `RATE_LIMITED` del contrato.
- [x] Umbral de muestra explícito (`MinimumPatternSamples=3`) y fechas UTC: sin afirmar certeza estadística no calculada.
- [x] Pruebas de error 400/401/405/429/500/503 ejecutadas; 403/404 no aplican a lectura agregada sin ID de recurso.
- [x] PII y secretos ausentes en logs, fixtures y descripciones; sólo conteos y categorías sintéticas.
- [x] Tests unitarios, de transporte y validadores H06/SDD ejecutados en local con resultado PASS.
- [x] `go vet ./...`, `go build ./...` y `go test ./... -count=1` PASS.
- [ ] `go test -race`: **NOT_RUN_LOCAL** — requiere cgo/toolchain C ausente en Windows; CI Linux debe ejecutarlo.
- [x] Despliegue real en dos instancias con TLS público e IaC aplicada bajo Gate: release `v0.20.0+4bf6758d372`, LT v19, ASG 2/2/2, refresh `914d3fc7-f808-4865-a744-689cfe749b61` Successful 100 %.
- [ ] CI/CD ejecutado en Linux sobre el SHA exacto: **CI_LINUX=PENDIENTE**.
- [x] Gate humano de release ejercido para el conjunto E052–E063: plan `tfplan-o4b` aplicado con `0 added, 4 changed, 0 destroyed`; rollback preservado (`v1`, `v0.19.2+3098f379d7ab`).
- [ ] Gate individual por E-ID: **NO_EJERCIDO**; se aplicó un Gate agregado de Oleada 4.
- [x] Dependencias del cliente Mobile no constituyeron Gate del Backend.
- [x] Sin cambios a `spec.md`, `plan.md` ni OpenAPI.

- [x] Ruta HTTPS desplegada: sin JWT responde 401 en `https://d2lpmqy5w6x9es.cloudfront.net`; ruta inexistente de control responde 404. Verificado tras el despliegue.
- [ ] Prueba funcional con JWT contra AWS: **NO_EJECUTADA**; `FUNCTIONAL_VERIFIED_AWS=NO`.
**Cierre: despliegue y presencia de ruta verificados por HTTP sin JWT; no se declara `FUNCTIONAL_VERIFIED_AWS=SI`. Pendientes: prueba con JWT, `go test -race` y CI Linux.**
