# Tareas — E052

> Estado: **CODE_IMPLEMENTADO_EN_LOCAL** en `liz-oleada4` (commit de implementación pendiente de registro final). Evidencia real adjunta; `AWS_DEPLOYED=NO`, `HTTPS_VERIFIED=NO`, `FUNCTIONAL_VERIFIED=NO` hasta Gate y prueba HTTP real.

- [x] `E052-T01` Revisado `operationId=nexum_e052` vs `GET /api/v1/analytics/insights` y contrato de dominio. Evidencia: OpenAPI 10567–10754; guía V2 §6.1; `MATCH_V602_OLEADA4.md`.
- [x] `E052-T02` DTO estricto implementado: sin cuerpo de request; `WirePage`/`WireInsight` con `additionalProperties=false` equivalente en transporte. Evidencia: `internal/application/analytics/insights.go`, `internal/transport/http/analytics_insights_handler.go`.
- [x] `E052-T03` Autorización por `sub` verificado y sesión autoritativa implementada; Propiedad por `USER#<sub>`. Evidencia: middleware `AuthenticateBearerWithSession`; tests `TestE052HTTPNegativas`, `TestE052HTTPFailClosedYAislamiento`.
- [x] `E052-T04` Caso de uso implementado con métricas verificables: umbral 12 movimientos, categoría ganadora por centavos reales, `evidenceCount`, `explanation` con importe exacto y `modelVersion=analytics-v1`. No se presenta como IA generativa. Evidencia: `internal/application/analytics/service.go`, `TestE052UmbralDatosSuficientes`.
- [x] `E052-T05` Adaptador real reutilizado: `dynamo.MovementsStore` con `Query` acotada a `USER#<sub>` y rango del mes; sin `Scan`, sin índice nuevo. Evidencia: `cmd/server/e052.go`; `internal/adapters/dynamo/movements_store.go` (`ListMovements`).
- [x] `E052-T06` Redis integrado como limitador compartido por usuario (`analytics`, 60/min) con fail-open de cuota, sin caché de datos financieros. Evidencia: `cmd/server/e052.go`, `cacheinfra.NewAuthRateLimiter`.
- [x] `E052-T07` Códigos y errores del contrato implementados: 200/400/401/405/503/500 vía `ErrorEnvelope`; `Cache-Control: private, no-store`; sin `nextCursor` huérfano. Evidencia: handler + tests de transporte.
- [x] `E052-T08` Tests unitarios reales de reglas y validación con fixtures sin PII. Evidencia: `internal/application/analytics/insights_test.go` (5 pruebas).
- [x] `E052-T09` Prueba HTTP de integración local conforme OpenAPI con JWT/sesión simulados y fuente con estado. Evidencia: `tests/unit/transport/analytics_insights_test.go` (3 pruebas).
- [x] `E052-T10` Negativas ejecutadas: auth ausente, periodo inválido, método no permitido, dependencia caída y usuario B sin filtración. Evidencia: mismos tests de transporte y aplicación.
- [x] `E052-T11` Calidad local ejecutada: `gofmt`, `go vet ./...`, `go build ./...`, `go test ./... -count=1`, validadores H06/SDD. Race `NOT_RUN_LOCAL`: `-race` requiere cgo y no hay toolchain C en Windows; CI Linux es autoridad. Evidencia: registro en `memoria.md`.
- [ ] `E052-T12` Autorización de release, despliegue desde IaC/CI, prueba en dos nodos y evidencia HTTP real: **BLOQUEADO** hasta Gate humano; no se ejecutó merge, AWS ni despliegue.

## Casos concretos

- [x] `E052-CT01` 200 con esquema exacto `PageOfInsight`: verificado por HTTP local y JSON decode.
- [x] `E052-CT02` Forma inválida (`period=2026-13`) → 400 `INVALID_REQUEST` sin secretos en logs. No hay cuerpo JSON en este endpoint.
- [x] `E052-CT03` Dos sujetos: B recibe `enoughData=false` y ningún dato de A.
- [x] `E052-CT04` Fuente caída → 503 `DEPENDENCY_UNAVAILABLE`; sin escritura parcial (endpoint de solo lectura).
- [ ] `E052-CT05` Misma operación en dos nodos Go reales: **NOT_RUN**, requiere AWS autorizado.
- [x] `E052-CT06` Importes operados en `int64` centavos y formateados con `FormatCentsToDecimal`; rango de mes UTC verificado por `PeriodBounds`. Decimal inválido no aplica (sin body).
- [x] `E052-CT07` No se usa IA generativa: E052 es analytics determinista con `modelVersion` explícito; ML real corresponde a E055 y sigue supeditado a su Gate.

## Tareas de la enmienda (A5) — RFC-016

- [x] `enoughData` (boolean) y `minimumSamples` (integer=12) implementados en la raíz de la respuesta.
- [x] Validación negativa de `period` con error 400 y detalle de campo.
- [x] Cobertura positiva (12 movimientos) y negativa (11 movimientos) en tests unitarios y de transporte.
- [ ] Verificación por HTTP real tras despliegue autorizado: **BLOQUEADO** (`AWS_DEPLOYED=NO`).
