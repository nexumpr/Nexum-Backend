# Tareas — E053

> Estado: **CODE_IMPLEMENTADO_EN_LOCAL** en `liz-oleada4`. Evidencia real ejecutada; `AWS_DEPLOYED=NO`, `HTTPS_VERIFIED=NO`, `FUNCTIONAL_VERIFIED=NO` hasta autorización de release y prueba HTTP real.

- [x] `E053-T01` Revisado `operationId=nexum_e053` vs `GET /api/v1/analytics/categories`; OpenAPI 10755–10939, esquemas `CategoryAnalytic` 17465 y `PageOfCategoryAnalytic` 18463; guía V2 §6.2.
- [x] `E053-T02` DTO estricto implementado: sin cuerpo; `WireCategoryAnalytic`/`WireCategoryPage` con los cinco campos obligatorios y `additionalProperties=false` equivalente en transporte.
- [x] `E053-T03` Autorización por `sub` verificado + sesión autoritativa; propiedad por `USER#<sub>`. Evidencia: middleware compartido, `TestE053HTTPNegativas`, `TestE053HTTPFailClosedYAislamiento`.
- [x] `E053-T04` Caso de uso con centavos exactos y porcentajes reproducibles: agregación de gastos persistidos, `int64`, reparto por mayor resto con desempate por `categoryId`, suma exacta 100 y orden determinista. Evidencia: `internal/application/analytics/categories.go`, `TestE053PorcentajesExactosYSuma100`, `TestE053MayorRestoDeterminista`.
- [x] `E053-T05` Adaptador real reutilizado: `dynamo.MovementsStore.ListMovements` con `Query` acotada a `USER#<sub>` y rango de mes, sin `Scan` ni índice nuevo; agregación de 2 páginas verificada. Evidencia: `cmd/server/e053.go`, `loadMovements`, `TestE053PorcentajesExactosYSuma100`.
- [x] `E053-T06` Redis integrado como limitador por usuario compartido (`analytics`, 60/min). Se corrigió el orden del wrapper: la cuota se aplica **después** de autenticar para que el `sub` esté en contexto. Evidencia: `limitador_contrato.go`, handler, `TestE053HTTPNegativas/cuota`.
- [x] `E053-T07` Códigos y errores del contrato: 200/400/401/405/429/500/503 con `ErrorEnvelope`, `Retry-After` en 429 y `Cache-Control: private, no-store` también en error. 403/404 no se sintetizan: no hay recurso por ID que revelar. Evidencia: handlers y tests.
- [x] `E053-T08` Unit tests de reglas: datos cero, sólo ingresos, periodos, aislamiento A/B, fallos de dependencia, cursor sin progreso y límite máximo del pattern. Evidencia: `internal/application/analytics/categories_test.go` (6 pruebas).
- [x] `E053-T09` Integración HTTP local conforme OpenAPI con JWT/sesión simulados, validación de `amount` por regex, rango 0–100, suma 100 y ausencia de PII. Evidencia: `tests/unit/transport/analytics_categories_test.go` (3 pruebas).
- [x] `E053-T10` Negativas ejecutadas: periodo inválido, sin token, método, cuota agotada, dependencia caída y usuario B sin filtración. Evidencia: tests de transporte y aplicación.
- [x] `E053-T11` Calidad local: `gofmt`, `go vet ./...`, `go build ./...`, `go test ./... -count=1`, validadores H06/SDD y baseline recalculado. Race `NOT_RUN_LOCAL`: sin cgo/gcc en Windows; CI Linux es autoridad. Evidencia: `memoria.md`.
- [ ] `E053-T12` Autorización de release, despliegue IaC/CI, prueba en dos nodos y evidencia HTTP real: **BLOQUEADO** hasta Gate humano; no se ejecutó merge, AWS ni despliegue.

## Casos concretos

- [x] `E053-CT01` 200 con esquema exacto `PageOfCategoryAnalytic` validado por JSON decode y regex de `amount`.
- [x] `E053-CT02` Forma inválida (`period=2026-00`) → 400 `INVALID_REQUEST` con detalle y sin secretos en logs. Sin cuerpo JSON en este endpoint.
- [x] `E053-CT03` Dos sujetos: B recibe `items=[]` y ningún dato de A.
- [x] `E053-CT04` Fuente caída → 503 `DEPENDENCY_UNAVAILABLE`; sin resultado parcial (endpoint de solo lectura).
- [ ] `E053-CT05` Misma operación en dos nodos Go reales: **NOT_RUN**, requiere AWS autorizado.
- [x] `E053-CT06` Centavos exactos: 0, mínimo, máximo del pattern (`999999999999.99`), agrupación, cambio de mes y zona UTC delimitada en servidor.
- [x] `E053-CT07` Suma de porcentajes consistente con el reparto determinista (70/20/10 y 34/33/33) y ninguna categoría de ingreso.

## Nota de corrección transversal

- Se detectó que el wrapper de cuota de E052 se aplicaba fuera de la autenticación, por lo que nunca veía el `sub` y no limitaba. Se corrigió en E052 y E053 con `LimitarPorUsuarioContrato` (código `RATE_LIMITED` exigido por H06/OpenAPI), sin modificar el helper heredado ni los tests de endpoints anteriores.
