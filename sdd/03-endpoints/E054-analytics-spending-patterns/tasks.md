# Tareas — E054

> Estado: **CODE_IMPLEMENTADO_EN_LOCAL** en `liz-oleada4`. Evidencia real ejecutada; `AWS_DEPLOYED=NO`, `HTTPS_VERIFIED=NO`, `FUNCTIONAL_VERIFIED=NO` hasta Gate y prueba HTTP real.

- [x] `E054-T01` Revisado `operationId=nexum_e054` vs `GET /api/v1/analytics/spending-patterns`; OpenAPI 10755+ y esquema `SpendingPattern` 17507; guía V2 §6.3.
- [x] `E054-T02` DTO estricto implementado: sin cuerpo; `WireSpendingPattern`/`WireSpendingPatternPage` con los cuatro campos obligatorios y `additionalProperties=false` equivalente.
- [x] `E054-T03` Autorización por `sub` verificado + sesión autoritativa; propiedad por `USER#<sub>`. Evidencia: tests `TestE054HTTPNegativas`, `TestE054HTTPFailClosedYAislamiento`.
- [x] `E054-T04` Caso de uso con patrón verificable: categoría con gastos positivos en ≥3 movimientos y ≥2 días UTC distintos; `sampleSize` es el conteo real y `description` cita ese conteo. Sin probabilidades ni certeza estadística. Evidencia: `internal/application/analytics/patterns.go`, `MinimumPatternSamples=3`.
- [x] `E054-T05` Adaptador real reutilizado: `dynamo.MovementsStore` con `Query` acotada a usuario/mes, sin `Scan`, compartida con E052/E053. Evidencia: `cmd/server/e054.go`, `loadMovements`.
- [x] `E054-T06` Redis integrado como limitador por usuario (`analytics`, 60/min) con `LimitarPorUsuarioContrato`, orden correcto tras autenticar. Evidencia: `TestE054HTTPNegativas/cuota` (429 + `RATE_LIMITED` + `Retry-After`).
- [x] `E054-T07` Códigos y errores del contrato: 200/400/401/405/429/500/503 con `ErrorEnvelope` y `Cache-Control: private, no-store` también en error. 403/404 no se sintetizan: lectura agregada sin recurso por ID.
- [x] `E054-T08` Unit tests: muestra suficiente/insuficiente, un solo día, ingresos, reversas (importes negativos), aislamiento A/B, periodo, fallo de dependencia y orden determinista. Evidencia: `patterns_test.go` (6 pruebas).
- [x] `E054-T09` Integración HTTP local conforme OpenAPI con JWT/sesión simulados, validación de `period`, `sampleSize`, `description` y ausencia de PII. Evidencia: `analytics_patterns_test.go` (3 pruebas).
- [x] `E054-T10` Negativas ejecutadas:auth, periodo inválido, método, cuota, dependencia caída y usuario B sin filtración.
- [x] `E054-T11` Calidad local: `gofmt`, `go vet ./...`, `go build ./...`, `go test ./... -count=1`, validadores H06/SDD y baseline. Race `NOT_RUN_LOCAL` sin cgo; CI Linux autoridad. Evidencia: `memoria.md`.
- [ ] `E054-T12` Release/IaC/dos nodos/HTTP real: **BLOQUEADO** hasta autorización humana; no se hizo merge, AWS ni despliegue.

## Casos concretos

- [x] `E054-CT01` 200 con esquema exacto `PageOfSpendingPattern` verificado por JSON decode.
- [x] `E054-CT02` Forma inválida (`period=2026-13`) → 400 `INVALID_REQUEST`; sin cuerpo JSON en este endpoint.
- [x] `E054-CT03` Dos sujetos: B recibe `items=[]` sin datos de A.
- [x] `E054-CT04` Fuente caída → 503 `DEPENDENCY_UNAVAILABLE`, sin resultado parcial.
- [ ] `E054-CT05` Dos nodos Go reales: **NOT_RUN** sin AWS autorizado.
- [x] `E054-CT06` Fechas derivadas en UTC del movimiento persistido; sin inferencia de zona IANA del cliente; importes no postulados en la respuesta.
- [x] `E054-CT07` Sin patrón con 2 muestras, con 3 cargos el mismo día, con ingresos o con reversas; patrón con 4 fechas reales y `sampleSize=4`.
