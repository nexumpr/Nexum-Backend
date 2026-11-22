# Checklist de calidad — E052

**Estado:** `CODE_IMPLEMENTADO_LOCAL=SI` · `CI_LOCAL=PASS` · `CI_LINUX=PENDIENTE` · `AWS_DEPLOYED=NO` · `HTTPS_VERIFIED=NO` · `FUNCTIONAL_VERIFIED=NO`.

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
- [ ] API en dos instancias, TLS público, CI/CD y IaC: **NOT_RUN** sin Gate ni AWS.
- [ ] Gate individual de release y rollback aprobados antes de `terraform apply`: **BLOQUEADO**.
- [x] Dependencias del cliente Mobile no constituyeron Gate del Backend.
- [x] Anexo PDF↔guía Oleada 4 aplicado en la guía externa; sin cambios a `spec.md`/`plan.md` ni OpenAPI.

## Criterios de aceptación A5

- [x] `enoughData` y `minimumSamples` aparecen en OpenAPI YAML/JSON y en la respuesta.
- [x] Se devuelven en el listado con el mismo tipo y formato (no hay creación/edición en E052).
- [x] Validación negativa de `period` devuelve 400 sin filtrar datos internos.
- [ ] HTTP real post-despliegue: **BLOQUEADO** hasta autorización de release.

**Cierre:** no se declara `PASS` de despliegue ni certificación funcional; la evidencia disponible es local y CI Linux queda como autoridad.
