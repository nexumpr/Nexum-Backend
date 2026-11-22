# Tareas — E058

> Estado: **CODE_IMPLEMENTADO_EN_LOCAL** en `liz-oleada4`. Evidencia real ejecutada; `AWS_DEPLOYED=NO`, `HTTPS_VERIFIED=NO`, `FUNCTIONAL_VERIFIED=NO`.

- [x] `E058-T01` Revisado `operationId=nexum_e058` vs `PUT /api/v1/notification-preferences`, OpenAPI 11657+ y esquema `NotificationPreferencesUpdate` 17674; guía V2 §6.7. Sin cambios al OpenAPI.
- [x] `E058-T02` DTO estricto implementado: `NotificationPreferencesUpdate` con `additionalProperties: false` mediante `BindJSONLimit`; tres booleanos requeridos, seis opcionales con defaults de esquema; `quietHours` objeto o `null`; `timeZone` opcional. `version` NO se acepta en el request (OpenAPI no lo declara; discrepancia RFC-016 registrada, no se introduce campo sin Gate).
- [x] `E058-T03` Autorización por `sub` verificado + sesión autoritativa; propiedad por `USER#<sub>`. Evidencia: `TestE058HTTPNegativasYAislamiento`.
- [x] `E058-T04` Caso de uso implementado: PUT reemplaza el conjunto declarado; la desactivación prevalece (`pushEnabled=false` se persiste y se devuelve); CAS interno por versión sin que el cliente envíe `version`. Item ausente →defaults materializados antes de escribir.
- [x] `E058-T05` Adaptador real extendido: `dynamo.NotificationPreferencesStore.Update` con `UpdateItem` condicional `#version = :expected`, `version = esperada+1`, campos opcionales ausentes retirados con `REMOVE`, relectura en conflicto y tope de reintentos. Sin `Scan`, sin recursos ni índices nuevos.
- [x] `E058-T06` Redis integrado como limitador por usuario (`analytics`, 60/min) con `LimitarPorUsuarioContrato`, aplicado tras autenticar. Evidencia: `TestE058HTTPNegativasYAislamiento/sin_cuota` (429 + `RATE_LIMITED`).
- [x] `E058-T07` Códigos y errores del contrato: 200/400/401/405/409/422/429/500/503 con `ErrorEnvelope` y `Cache-Control: private, no-store`. 403/404 no se sintetizan en preferencias del propio `sub`.
- [x] `E058-T08` Unit tests ejecutados: CAS con versión observada, materialización previa, sin store, store sin CAS, conflicto, expresión `SET`/`REMOVE`, versión esperada y conflicto de versión. Evidencia: `update_test.go` (3) y `notification_preferences_store_test.go` (3 nuevos).
- [x] `E058-T09` Integración HTTP local conforme OpenAPI: actualización válida + relectura por GET, defaults opcionales, 422 (`quietHours` sin `enabled`, formato, inicio=fin, zona inválida), 400 (requerido ausente, campo desconocido, JSON roto, tipo incorrecto), 409, 401, 429 y aislamiento A/B. Evidencia: `notification_preferences_test.go` (5 pruebas nuevas).
- [x] `E058-T10` Negativas ejecutadas: auth, cuota, conflicto de escritura, dependencia caída y usuario B sin afectar a A.
- [x] `E058-T11` Calidad local: `gofmt`, `go vet ./...`, `go build ./...`, `go test ./... -count=1`, validadores H06/SDD y DevSecOps. Race `NOT_RUN_LOCAL` sin cgo; CI Linux autoridad. Evidencia: `memoria.md`.
- [ ] `E058-T12` Release/IaC/dos nodos/HTTP real: **BLOQUEADO** hasta autorización humana; no se hizo merge, AWS ni despliegue.

## Casos concretos

- [x] `E058-CT01` 200 con `NotificationPreferences` exacto; la relectura por E057 devuelve el mismo estado sin incrementar versión.
- [x] `E058-CT02` Body incompleto, campo extra, JSON malformado y tipo incorrecto → 400 sin secretos en logs.
- [x] `E058-CT03` `quietHours` inválido, `enabled` ausente, `start=end` y zona IANA inválida → 422 `BUSINESS_RULE_VIOLATION` con `field`.
- [x] `E058-CT04` Store caído → 503; conflicto de versión → 409; ninguna escritura parcial.
- [ ] `E058-CT05` Dos nodos Go reales: **NOT_RUN** sin AWS autorizado.
- [x] `E058-CT06` Opt-out `pushEnabled=false` se persiste y se devuelve; dos sujetos probados sin contaminación cruzada.
- [x] `E058-CT07` Actualizaciones simultáneas: CAS rechaza versión observada desactualizada; el adaptador relee y agota reintentos con 409.
