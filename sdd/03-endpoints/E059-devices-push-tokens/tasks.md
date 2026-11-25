# Tareas — E059 (implementación local H08, sin AWS)

> Estado: implementación local completada con contrato OpenAPI existente. La
> evidencia siguiente corresponde a ejecución real en la rama `liz-oleada4`;
> despliegue, dos nodos y IaC siguen **BLOQUEADOS sin AWS**.

- [x] `E059-T01` Revisar OpenAPI `operationId=nexum_e059` vs `POST /api/v1/devices/push-tokens` y contrato de dominio. Evidencia: `sdd/03-endpoints/openapi/nexum-api-v1.json`, operación `nexum_e059`; DTOs `PushTokenRegister`/`PushDevice`.
- [x] `E059-T02` DTOs estrictos y rechazo de campos desconocidos. Evidencia: `PushTokenRegisterRequest` y `BindJSONLimit` en `internal/transport/http/push_devices_handler.go`; caso `campo_extra` en `TestE059HTTPNegativas`.
- [x] `E059-T03` Autorización por sujeto JWT. Evidencia: `AuthenticateBearerWithSession` + `UserIDFromContext` en ambos handlers; caso `sin_token` → 401 en `TestE059HTTPNegativas`.
- [x] `E059-T04` Caso de uso: token cifrado en reposo, rotación y separación por usuario. Evidencia: `Service.Register` con `TokenSealer`, AAD `<userID>|<deviceID>` y SHA-256 del token; `TestE059ServicioRegistroNuevo`, `TestE059ServicioRotacionReutilizaID`.
- [x] `E059-T05` Adaptador DynamoDB sin Scan: `USER#<sub>/DEVICE#<uuid>` e idempotencia `USER#<sub>/IDEM#PUSH#<sha256>`. Evidencia: `internal/adapters/dynamo/push_devices_store.go`; `TestE059AdaptadorTransaccionCondicional`, `TestE059AdaptadorQueryPorEtiquetaSinScan` (Query con `begins_with(SK, :prefix)` y filtro en memoria).
- [x] `E059-T06` Consistencia multiinstancia e idempotencia con TTL. Evidencia: condición `attribute_not_exists(PK) OR expiresAt < :now` en `Register`; `TestE059ServicioReplayMismaClave`, `TestE059ServicioClaveRepetidaPayloadDistinto`, `TestE059AdaptadorCancelacionConcurrente`;Redis se usa sólo para cuota (`LimitarPorUsuarioContrato`).
- [x] `E059-T07` Códigos, ErrorEnvelope, Location y cache. Evidencia: `writePushDevicesError`, `Cache-Control: private, no-store`, `Location: /api/v1/devices/push-tokens/<id>`; `TestE059HTTPContratoValido`, `TestE059HTTPNegativas`, `TestE059HTTPFailClosed`.
- [x] `E059-T08` Unit tests de reglas y validaciones con fixtures sintéticos. Evidencia: `internal/application/pushdevices/service_test.go` (clave AES generada en memoria, nunca impresa).
- [x] `E059-T09` Integración Dynamo con fake client y prueba HTTP local conforme OpenAPI. Evidencia: `internal/adapters/dynamo/push_devices_store_test.go` y `tests/unit/transport/push_devices_test.go`. **AWS real: NOT_RUN.**
- [x] `E059-T10` Negativas de auth, propiedad, cuota y fallo cerrado. Evidencia: `TestE059HTTPNegativas`, `TestE059HTTPFailClosed`, `TestE059ServicioSinTitular`, `TestE059ServicioSinSealerOSinStore`.
- [x] `E059-T11` gofmt, revisión de contrato y trazas saneadas en local. Evidencia: comando de validación abajo; rollback/IaC real queda en `T12`.
- [ ] `E059-T12` **BLOQUEADO sin AWS**: solicitar autorización humana de release, desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real. No se ejecuta `terraform`, AWS ni despliegue en esta tarea.

## Casos concretos

- [x] `E059-CT01` 201 con esquema exacto, `Location` y sin token/cipher. Evidencia: `TestE059HTTPContratoValido`.
- [x] `E059-CT02` JSON malformado, campo extra, tipo/provider inválido, token corto y etiqueta >80 → 400 sin secretos. Evidencia: `TestE059HTTPNegativas`.
- [x] `E059-CT03` Aislamiento A/B y sin filtración entre titulares (aplicable a la propiedad compartida con E060). Evidencia: `TestE060HTTPAjenoInexistenteYAislamientoAB`.
- [x] `E059-CT04` Pérdida de dependencias → 503 sin escritura parcial. Evidencia: `TestE059HTTPFailClosed`, `TestE059AdaptadorFallaCerrada`.
- [ ] `E059-CT05` **BLOQUEADO sin AWS**: misma operación en dos nodos Go reales, consistencia de sesión/caché y ausencia de duplicaciones. No se afirma PASS sin despliegue real.
- [x] `E059-CT06` Replay estable con misma clave/payload y 409 con payload distinto. Evidencia: `TestE059HTTPReplayYConflicto`, `TestE059ServicioReplayMismaClave`, `TestE059ServicioClaveRepetidaPayloadDistinto`.
- [x] `E059-CT07` Rotación y baja sin reutilizar token revocado; sin dependencia del equipo Android. Evidencia: `TestE059ServicioRotacionReutilizaID`, `TestE060ServicioBajaIdempotenteYTokenRevocado`.

## Evidencia de ejecución real

```
gofmt -w <archivos propios de E059/E060>
go test ./internal/application/pushdevices ./internal/adapters/dynamo ./tests/unit/transport ./cmd/server -run 'E059|E060' -count=1 -timeout 300s
ok  github.com/nexumpr/Nexum-Backend/internal/application/pushdevices
ok  github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo
ok  github.com/nexumpr/Nexum-Backend/tests/unit/transport
ok  github.com/nexumpr/Nexum-Backend/cmd/server  [no tests to run]
```
