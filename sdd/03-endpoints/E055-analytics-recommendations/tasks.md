# Tareas — E055

> Estado: **CODE_IMPLEMENTADO_EN_LOCAL** en `liz-oleada4`. Evidencia real ejecutada; `ML_GATE_APPROVED=NO`, `AWS_DEPLOYED=NO`, `HTTPS_VERIFIED=NO`, `FUNCTIONAL_VERIFIED=NO`.

- [x] `E055-T01` Revisado `operationId=nexum_e055` vs `GET /api/v1/analytics/recommendations`, OpenAPI 11128+ y esquemas `Recommendation` 17538 / `PageOfRecommendation` 18505; guía V2 §6.4. Sin cambios al OpenAPI.
- [x] `E055-T02` DTO estricto implementado: sin cuerpo; `WireRecommendation` con los cinco campos obligatorios y `WireRecommendationPage` con `items` + `hasEnoughData`; `nextCursor` se omite porque no hay parámetro de entrada para usarlo.
- [x] `E055-T03` Autorización por `sub` verificado + sesión autoritativa; propiedad por `USER#<sub>`. Evidencia: `TestE055HTTPNegativas`, `TestE055HTTPFailClosedYAislamiento`.
- [x] `E055-T04` Caso de uso implementado sobre K-Means real: `hasEnoughData=false` con menos de 12 movimientos, menos de 2 categorías o silueta < 0,35; `items=[]` en ese estado; `reason` cita conteos, mediana y franja horaria calculados; `disclaimer` obligatorio; sin ahorros prometidos.
- [x] `E055-T05` DynamoDB real reutilizado: `Query` acotada a usuario/ventana (90 días por defecto o `period` explícito), sin `Scan`, compartida con E052–E054. No se crearon índices ni recursos AWS.
- [x] `E055-T06` Redis integrado como limitador por usuario (`analytics`, 60/min) con `LimitarPorUsuarioContrato`, aplicado tras autenticar. Evidencia: `TestE055HTTPNegativas/cuota` (429 + `RATE_LIMITED` + `Retry-After`).
- [x] `E055-T07` Códigos y errores del contrato: 200/400/401/405/429/500/503 con `ErrorEnvelope` y `Cache-Control: private, no-store` también en error. No se inventó 422: OpenAPI declara 400 y manda.
- [x] `E055-T08` Unit tests ejecutados: 0/5/11/12 movimientos, una sola categoría, silueta baja, ventana 90 días, muestreo de 500, dos usuarios, determinismo y latencia. Evidencia: `recommendations_test.go`.
- [x] `E055-T09` Integración HTTP local conforme OpenAPI con JWT/sesión simulados, esquema exacto, UUID v5, `modelVersion=kmeans-v1`, ausencia de PII y `hasEnoughData`. Evidencia: `analytics_recommendations_test.go` (3 pruebas).
- [x] `E055-T10` Negativas ejecutadas: auth, periodo inválido, método, cuota, dependencia caída y usuario B sin filtración.
- [x] `E055-T11` Calidad local: `gofmt`, `go vet ./...`, `go build ./...`, `go test ./... -count=1`, validadores H06/SDD, baseline y Semgrep. Race `NOT_RUN_LOCAL` sin cgo; CI Linux autoridad. Evidencia: `memoria.md`.
- [ ] `E055-T12` Release/IaC/dos nodos/HTTP real: **BLOQUEADO** hasta autorización humana y Gate ML; no se hizo merge, AWS ni despliegue.

## Casos concretos

- [x] `E055-CT01` 200 con esquema exacto `PageOfRecommendation` y `hasEnoughData` verificado por JSON decode.
- [x] `E055-CT02` Forma inválida (`period=2026-13`) → 400 `INVALID_REQUEST`; sin cuerpo JSON en este endpoint, sin secretos en logs.
- [x] `E055-CT03` Dos sujetos: B recibe `hasEnoughData=false`, `items=[]` sin datos de A. No se sintetiza 404: la lectura es agregada y no expone existencia de recursos por ID.
- [x] `E055-CT04` Fuente caída → 503 `DEPENDENCY_UNAVAILABLE`, sin resultado parcial.
- [ ] `E055-CT05` Dos nodos Go reales: **NOT_RUN** sin AWS autorizado.
- [x] `E055-CT06` Importes en centavos exactos, fechas UTC convertidas a convención Python para la feature día, sin zonas IANA del cliente; la latencia local con 500 movimientos dio p95 `1.724 ms`.
- [x] `E055-CT07` ML real K-Means, no reglas fijas ni proveedor de pago: artefacto `kmeans-v1` con SHA verificado, inferencia Go y 12 asignaciones reproducidas contra referencia Python. **Gate ML-155 sigue PENDIENTE**.

## Tareas de la enmienda (A2)

- [x] `hasEnoughData` implementado como booleano requerido en `PageOfRecommendation`; `false` implica `items=[]`.
- [x] Validación negativa de `period` → 400 `INVALID_REQUEST` con detalle `period: invalid`. El 422 solicitado en la enmienda **no se inventó** porque el contrato H06/OpenAPI declara 400 y ningún 422; discrepancia registrada conforme Anexo J.
- [x] Cobertura positiva y negativa en unitarias y HTTP (`TestE055HistorialInsuficiente`, `TestE055RecomendacionesKMeansReales`, `TestE055HTTPContratoValido`, `TestE055HTTPNegativas`).
- [ ] Verificación HTTP real tras despliegue autorizado: **BLOQUEADO**.
