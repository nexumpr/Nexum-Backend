# Tareas — E056

> Estado: **CODE_IMPLEMENTADO_EN_LOCAL** en `liz-oleada4`. Evidencia real ejecutada; `AWS_DEPLOYED=NO`, `HTTPS_VERIFIED=NO`, `FUNCTIONAL_VERIFIED=NO`.

- [x] `E056-T01` Revisado `operationId=nexum_e056` vs `GET /api/v1/analytics/savings-streak`, OpenAPI 11315+ y esquema `SavingsStreak` 17573; guía V2 §6.5. Sin cambios al OpenAPI.
- [x] `E056-T02` DTO estricto implementado: sin parámetros ni cuerpo; `WireSavingsStreak` con exactamente `days`, `timezone` y `asOfDate`.
- [x] `E056-T03` Autorización por `sub` verificado + sesión autoritativa; propiedad por `USER#<sub>`. Evidencia: `TestE056HTTPNegativas`, `TestE056HTTPFailClosedYAislamiento`.
- [x] `E056-T04` Caso de uso implementado: racha de días civiles IANA consecutivos hacia atrás desde `asOfDate`; regla local explícita «día sin aportación activa rompe racha», pendiente de confirmación de Gate. Se cuentan sólo aportaciones activas, positivas, no revertidas y deduplicadas por id; no se suman movimientos vinculados. Ventana máxima 365 días.
- [x] `E056-T05` Adaptador real reutilizado: `dynamo.ContributionsStore` con `Query` acotada a `USER#<sub>` y al rango de fechas, `Status=active`, sin `Scan`. No se crearon recursos ni índices AWS.
- [x] `E056-T06` Redis integrado como limitador por usuario (`analytics`, 60/min) con `LimitarPorUsuarioContrato`, aplicado tras autenticar. Evidencia: `TestE056HTTPNegativas/cuota` (429 + `RATE_LIMITED`).
- [x] `E056-T07` Códigos y errores del contrato: 200/400/401/405/429/500/503 con `ErrorEnvelope` y `Cache-Control: private, no-store`. 403/404 no se sintetizan en lectura agregada sin ID de recurso.
- [x] `E056-T08` Unit tests ejecutados: rachas 0/1/7, día hueco, revertidas/importe 0/negativas, deduplicación entre páginas, cruce medianoche, DST `America/New_York`, fallback IANA y fallo de dependencia. Evidencia: `savings_test.go` (8 pruebas).
- [x] `E056-T09` Integración HTTP local conforme OpenAPI con JWT/sesión simulados y esquema exacto. Evidencia: `analytics_savings_test.go` (3 pruebas).
- [x] `E056-T10` Negativas ejecutadas: auth, método, cuota, dependencia caída y usuario B sin racha ajena.
- [x] `E056-T11` Calidad local: `gofmt`, `go vet ./...`, `go build ./...`, `go test ./... -count=1`, validadores H06/SDD y DevSecOps. Race `NOT_RUN_LOCAL` sin cgo; CI Linux autoridad. Evidencia: `memoria.md`.
- [ ] `E056-T12` Release/IaC/dos nodos/HTTP real: **BLOQUEADO** hasta autorización humana; no se hizo merge, AWS ni despliegue.

## Casos concretos

- [x] `E056-CT01` 200 con esquema exacto `SavingsStreak` verificado por JSON decode.
- [x] `E056-CT02` Sin cuerpo ni parámetros: no hay campos desconocidos que validar; sin secretos en logs.
- [x] `E056-CT03` Dos sujetos: B recibe `days=0` sin datos de A.
- [x] `E056-CT04` Store caído → 503 `DEPENDENCY_UNAVAILABLE`, sin racha parcial.
- [ ] `E056-CT05` Dos nodos Go reales: **NOT_RUN** sin AWS autorizado.
- [x] `E056-CT06` Importes en centavos y marcas UTC convertidas al día civil de la zona IANA resuelta; probado cruce medianoche y cambio DST sin duplicar días.
- [x] `E056-CT07` Aportaciones revertidas, de importe no positivo y duplicadas quedan excluidas; el movimiento vinculado no se cuenta por separado.

**Pendiente de decisión humana:** confirmar o sustituir la regla de día sin aportación. El código documenta la regla usada y no afirma que fue aprobada.
