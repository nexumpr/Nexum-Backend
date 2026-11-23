# Checklist de calidad — E055

**Estado:** `CODE_IMPLEMENTADO_LOCAL=SI` · `CI_LOCAL=PASS` · `CI_LINUX=PENDIENTE` · `AWS_DEPLOYED=SI` · `HTTPS_RUTA_PRESENTE=SI` · `HTTPS_FUNCIONAL_CON_JWT=NO` · `FUNCTIONAL_VERIFIED_AWS=NO` · `RELEASE=v0.20.0+4bf6758d372` · `ML_GATE_APPROVED=NO`.

- [x] Método/path conservan el ID del inventario H04 (`GET /api/v1/analytics/recommendations`, `nexum_e055`).
- [x] Payload válido/negativo revisado: sin cuerpo; `period` validado con `^\d{4}-(0[1-9]|1[0-2])$`.
- [x] Respuesta `PageOfRecommendation` y errores de OpenAPI implementados (`ErrorEnvelope`, `Cache-Control`).
- [x] Semántica de propiedad: `sub` verificado + sesión autoritativa + consulta `USER#<sub>`; dos sujetos probados.
- [x] Idempotencia no exigida; lectura sin efectos; cuota por usuario con orden correcto.
- [x] DynamoDB usa `Query` acotada, nunca `Scan`; sin recursos ni índices nuevos.
- [x] Redis: limitador real con ventana de 60s y código `RATE_LIMITED` del contrato.
- [x] Monto exacto en centavos y marcas UTC; la feature día usa la convención lunes=0 del pipeline Python sin inferir zona IANA del cliente.
- [x] Pruebas de error 400/401/405/429/500/503 ejecutadas; 403/404 no aplican a lectura agregada sin ID de recurso; 422 no existe en el contrato y no se inventó.
- [x] PII y secretos ausentes en logs, fixtures, features y respuestas; el id es UUID v5 derivado por hash.
- [x] K-Means real: artefacto JSON con SHA verificado, semilla 20261116, n_init 10, k=2, silueta 0.780658; inferencia Go sin Python/AWS/proveedor de pago.
- [x] Referencia Python↔Go: 12 asignaciones del pipeline reproducidas por `ml/inference_test.go`.
- [x] Umbrales ML-155 probados: 0/5/11/12 movimientos, una categoría, silueta 0.34, 500 movimientos y dos usuarios.
- [x] Latencia local p95 `1.724 ms` con 500 movimientos (objetivo <= 300 ms) y benchmark `718127 ns/op`.
- [x] `go vet ./...`, `go build ./...` y `go test ./... -count=1` PASS.
- [ ] `go test -race`: **NOT_RUN_LOCAL** — requiere cgo/toolchain C ausente en Windows; CI Linux debe ejecutarlo.
- [x] Despliegue real en dos instancias con TLS público e IaC aplicada bajo Gate: release `v0.20.0+4bf6758d372`, LT v19, ASG 2/2/2, refresh `914d3fc7-f808-4865-a744-689cfe749b61` Successful 100 %.
- [ ] CI/CD ejecutado en Linux sobre el SHA exacto: **CI_LINUX=PENDIENTE**.
- [x] Gate humano de release ejercido para el conjunto E052–E063: plan `tfplan-o4b` aplicado con `0 added, 4 changed, 0 destroyed`; rollback preservado (`v1`, `v0.19.2+3098f379d7ab`).
- [ ] Gate ML-155 y Gate individual por E-ID: **PENDIENTE_APROBACION**; `ML_GATE_APPROVED=NO`.
- [ ] Python 3.13 exacto: **DEVIACION_REGISTRADA** — el equipo tiene 3.14.5; el script es compatible con 3.13 y debe regenerarse en ese runtime si el Gate lo exige.
- [x] Dependencias del cliente Mobile no constituyeron Gate del Backend.
- [x] Sin cambios a `spec.md`, `plan.md` ni OpenAPI.

- [x] Ruta HTTPS desplegada: sin JWT responde 401 en `https://d2lpmqy5w6x9es.cloudfront.net`; ruta inexistente de control responde 404. Verificado tras el despliegue.
- [ ] Prueba funcional con JWT contra AWS: **NO_EJECUTADA**; `FUNCTIONAL_VERIFIED_AWS=NO`.
**Cierre: despliegue y presencia de ruta verificados por HTTP sin JWT; no se declara `FUNCTIONAL_VERIFIED_AWS=SI`. Pendientes: prueba con JWT, `go test -race` y CI Linux. Gate ML-155 sin aprobación.**
