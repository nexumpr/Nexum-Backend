# Tareas — E057

> Estado: **CODE_IMPLEMENTADO_EN_LOCAL** en `liz-oleada4`. Evidencia real ejecutada; `AWS_DEPLOYED=NO`, `HTTPS_VERIFIED=NO`, `FUNCTIONAL_VERIFIED=NO`.

- [x] `E057-T01` Revisado `operationId=nexum_e057` vs `GET /api/v1/notification-preferences`, OpenAPI 11486+ y esquema `NotificationPreferences` 17601; guía V2 §6.6. Sin cambios al OpenAPI.
- [x] `E057-T02` DTO estricto implementado: sin parámetros ni cuerpo; `WirePreferences` con los cuatro campos requeridos y los seis opcionales de la enmienda A3 (`budgetAlerts`, `goalUpdates`, `recurringReminders`, `streakReminders`, `quietHours`, `timeZone`). `additionalProperties: false` verificado por test.
- [x] `E057-T03` Autorización por `sub` verificado + sesión autoritativa; propiedad por `USER#<sub>`. Evidencia: `TestE057HTTPNegativas`, `TestE057HTTPDefaultsYAislamiento`.
- [x] `E057-T04` Caso de uso implementado: preferencias por canal, opt-in privado y estado real. Item ausente → defaults RFC-016 materializados de forma idempotente (creación condicional), sin inventar 404. `timeZone` no tiene default contractual y se omite hasta que E058 lo fije.
- [x] `E057-T05` Adaptador real `dynamo.NotificationPreferencesStore`: `GetItem` consistente y `PutItem` condicional `attribute_not_exists(PK)` sobre `USER#<sub>/NOTIFICATION_PREFERENCES`; sin `Scan`, sin recursos ni índices nuevos. SK dedicada para no colisionar con el `PREFERENCES` de perfil (E068/E069).
- [x] `E057-T06` Redis integrado como limitador por usuario (`analytics`, 60/min) con `LimitarPorUsuarioContrato`, aplicado tras autenticar. Evidencia: `TestE057HTTPNegativas/cuota` (429 + `RATE_LIMITED`).
- [x] `E057-T07` Códigos y errores del contrato: 200/400/401/405/429/500/503 con `ErrorEnvelope` y `Cache-Control: private, no-store`. 403/404 no se sintetizan en lectura agregada sin ID de recurso; 409 se reserva para conflicto de escritura (E058); 422 no existe en esta operación GET y no se inventó.
- [x] `E057-T08` Unit tests ejecutados: defaults sin registro, idempotencia, preferencias modificadas granulares, `quietHours`/`timeZone`, dos usuarios, sin titular, sin store y store caído. Evidencia: `preferences_test.go` (3 pruebas).
- [x] `E057-T09` Integración HTTP local conforme OpenAPI con JWT/sesión simulados, esquema exacto, `additionalProperties: false`, ausencia de PII y defaults. Evidencia: `notification_preferences_test.go` (4 pruebas).
- [x] `E057-T10` Negativas ejecutadas: auth, método, cuota, dependencia caída y usuario B sin filtración.
- [x] `E057-T11` Pruebas del adaptador Dynamo ejecutadas: proyección, item ausente/ajeno, creación condicional, ganador concurrente y falla cerrada. Evidencia: `notification_preferences_store_test.go` (5 pruebas).
- [x] `E057-T12` Calidad local: `gofmt`, `go vet ./...`, `go build ./...`, `go test ./... -count=1`, validadores H06/SDD y DevSecOps. Race `NOT_RUN_LOCAL` sin cgo; CI Linux autoridad. Evidencia: `memoria.md`.
- [ ] `E057-T13` Release/IaC/dos nodos/HTTP real: **BLOQUEADO** hasta autorización humana; no se hizo merge, AWS ni despliegue.

## Casos concretos

- [x] `E057-CT01` 200 con esquema exacto `NotificationPreferences` verificado por JSON decode y whitelist de campos.
- [x] `E057-CT02` Sin cuerpo ni parámetros: no hay campos desconocidos que rechazar; sin secretos en logs.
- [x] `E057-CT03` Dos sujetos: B recibe defaults propios y `pushEnabled=false` sin datos de A.
- [x] `E057-CT04` Store caído → 503 `DEPENDENCY_UNAVAILABLE`, sin defaults inventados en la respuesta.
- [ ] `E057-CT05` Dos nodos Go reales: **NOT_RUN** sin AWS autorizado.
- [x] `E057-CT06` Preferencias granulares, `quietHours` con `enabled=false` conservado y `timeZone` IANA devueltos sin normalización inventada.
