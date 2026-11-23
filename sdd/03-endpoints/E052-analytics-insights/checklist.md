# Checklist de calidad — E052

**Estado:** `CODE_IMPLEMENTADO_LOCAL=SI` · `CI_LOCAL=PASS` · `CI_LINUX=PENDIENTE` · `AWS_DEPLOYED=SI` · `HTTPS_RUTA_PRESENTE=SI` · `HTTPS_FUNCIONAL_CON_JWT=NO` · `FUNCTIONAL_VERIFIED_AWS=NO` · `RELEASE=v0.20.0+4bf6758d372`.

- [x] Método/path conservan el ID del inventario H04 (`GET /api/v1/analytics/insights`, `nexum_e052`).
- [x] Payload válido/negativo revisado: sin cuerpo; `period` validado con `^\d{4}-(0[1-9]|1[0-2])$`.
- [x] Respuesta `PageOfInsight` y errores de OpenAPI revisados e implementados (`ErrorEnvelope`).
- [x] Semántica de propiedad: `sub` verificado + sesión autoritativa + consulta `USER#<sub>`; dos sujetos probados.
- [x] Idempotencia no exigida y concurrencia resuelta con Redis compartido y fail-closed de persistencia.
- [x] DynamoDB usa `Get/Query`, nunca `Scan`; sin recursos ni índices nuevos.
- [x] Redis: limitador real por usuario con TTL de ventana; sin caché de datos financieros.
- [x] Montos en `int64` centavos y fechas UTC con límites de mes IANA-neutrales en el servidor.
- [x] Pruebas de error 400/401/405/503/500 ejecutadas en local; 404/409/412/422 no aplican a esta operación de lectura sin recurso por ID.
- [x] PII y secretos ausentes en logs, capturas y fixtures; `id` derivado por hash, nunca el `sub`.
- [x] Tests unitarios, de transporte y validadores H06/SDD ejecutados en local con resultado PASS.
- [x] `go vet ./...`, `go build ./...` y `go test ./... -count=1` PASS.
- [ ] `go test -race`: **NOT_RUN_LOCAL** — requiere cgo/toolchain C ausente en Windows; CI Linux debe ejecutarlo.
- [x] Despliegue real en dos instancias con TLS público e IaC aplicada bajo Gate: release `v0.20.0+4bf6758d372`, LT v19, ASG 2/2/2, refresh `914d3fc7-f808-4865-a744-689cfe749b61` Successful 100 %.
- [ ] CI/CD ejecutado en Linux sobre el SHA exacto: **CI_LINUX=PENDIENTE**.
- [x] Gate humano de release ejercido para el conjunto E052–E063: plan `tfplan-o4b` aplicado con `0 added, 4 changed, 0 destroyed`; rollback preservado (`v1`, `v0.19.2+3098f379d7ab`).
- [ ] Gate individual por E-ID: **NO_EJERCIDO**; se aplicó un Gate agregado de Oleada 4.
- [x] Dependencias del cliente Mobile no constituyeron Gate del Backend.
- [x] Anexo PDF↔guía Oleada 4 aplicado en la guía externa; sin cambios a `spec.md`/`plan.md` ni OpenAPI.

## Criterios de aceptación A5

- [x] `enoughData` y `minimumSamples` aparecen en OpenAPI YAML/JSON y en la respuesta.
- [x] Se devuelven en el listado con el mismo tipo y formato (no hay creación/edición en E052).
- [x] Validación negativa de `period` devuelve 400 sin filtrar datos internos.
- [x] Ruta HTTPS desplegada: sin JWT responde 401 en `https://d2lpmqy5w6x9es.cloudfront.net`; ruta inexistente de control responde 404. Verificado tras el despliegue.
- [ ] Prueba funcional con JWT contra AWS: **NO_EJECUTADA**; `FUNCTIONAL_VERIFIED_AWS=NO`.

**Cierre: despliegue y presencia de ruta verificados por HTTP sin JWT; no se declara `FUNCTIONAL_VERIFIED_AWS=SI`. Pendientes: prueba con JWT, `go test -race` y CI Linux.**
