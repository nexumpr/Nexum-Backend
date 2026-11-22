# Tareas — E060 (implementación local H08, sin AWS)

> Estado: implementación local completada con contrato OpenAPI existente. La
> evidencia siguiente corresponde a ejecución real en la rama `liz-oleada4`;
> despliegue, dos nodos y IaC siguen **BLOQUEADOS sin AWS**.

- [x] `E060-T01` Revisar OpenAPI `operationId=nexum_e060` vs `DELETE /api/v1/devices/push-tokens/{deviceRegistrationId}` y contrato de dominio. Evidencia: `sdd/03-endpoints/openapi/nexum-api-v1.yaml` líneas 8113-8245; sin cuerpo y 204 sin contenido.
- [x] `E060-T02` Sin DTO de request/response: el handler ignora cuerpo y valida `deviceRegistrationId` con formato UUID canónico. Evidencia: `pushDeviceIDFromPath`, `TestE060HTTPNegativas` (`id_invalido` → 400).
- [x] `E060-T03` Autorización por sujeto JWT y propiedad. Evidencia: `AuthenticateBearerWithSession`/`UserIDFromContext`; `GetDevice` y `Revoke` usan siempre `USER#<sub>/DEVICE#<id>`; `TestE060HTTPAjenoInexistenteYAislamientoAB`.
- [x] `E060-T04` Caso de uso: revocar sólo el dispositivo propio, borrar token cifrado y huella, impedir envíos posteriores. Evidencia: `Service.Revoke`, `TestE060ServicioBajaIdempotenteYTokenRevocado`.
- [x] `E060-T05` Adaptador DynamoDB sin Scan: `UpdateItem` condicional por `active`, `SET active=false/revokedAt`, `REMOVE tokenCipher/tokenFingerprint`. Evidencia: `TestE060AdaptadorBajaUpdateItem`, `TestE060AdaptadorGetDevice`.
- [x] `E060-T06` Consistencia multiinstancia: condición `active = true`; si otra instancia gana, relectura y 204/404 homogéneo. Cuota por usuario con `LimitarPorUsuarioContrato(..., "analytics", ...)`; Redis real NOT_RUN.
- [x] `E060-T07` Códigos y ErrorEnvelope: 204 sin cuerpo; 400/401/404/429/503 probados; 409/422 mapeados en el handler. Evidencia: `writePushDevicesError`, `TestE060HTTPBajaExitosaEIdempotente`, `TestE060HTTPNegativas`, `TestE060HTTPFailClosed`.
- [x] `E060-T08` Unit tests de reglas con fixtures sintéticos. Evidencia: `internal/application/pushdevices/service_test.go`.
- [x] `E060-T09` Integración Dynamo con fake client y prueba HTTP local conforme OpenAPI. Evidencia: `internal/adapters/dynamo/push_devices_store_test.go`, `tests/unit/transport/push_devices_test.go`. **AWS real: NOT_RUN.**
- [x] `E060-T10` Negativas de auth, propiedad/ajeno, inexistente, cuota y fallo cerrado. Evidencia: `TestE060HTTPNegativas`, `TestE060HTTPAjenoInexistenteYAislamientoAB`, `TestE060ServicioAjenoInexistente`, `TestE060AdaptadorFallaCerrada`.
- [x] `E060-T11` gofmt, revisión de contrato y trazas saneadas en local. Evidencia: comando de validación abajo; rollback/IaC real queda en `T12`.
- [ ] `E060-T12` **BLOQUEADO sin AWS**: solicitar autorización humana de release, desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real. No se ejecuta `terraform`, AWS ni despliegue en esta tarea.

## Casos concretos

- [x] `E060-CT01` 204 sin cuerpo y sin `Location` en baja propia y repetida. Evidencia: `TestE060HTTPBajaExitosaEIdempotente`.
- [x] `E060-CT02` Parámetro de ruta inválido → 400 `INVALID_REQUEST`; sin secretos en logs. Evidencia: `TestE060HTTPNegativas`.
- [x] `E060-CT03` Dispositivo ajeno o inexistente → 404 `NOT_FOUND` homogéneo, verificado con dos sujetos. Evidencia: `TestE060HTTPAjenoInexistenteYAislamientoAB`, `TestE060ServicioAjenoInexistente`.
- [x] `E060-CT04` Sin servicio/sealer/store → 503 sin confirmar escritura. Evidencia: `TestE060HTTPFailClosed`, `TestE059AdaptadorFallaCerrada`.
- [ ] `E060-CT05` **BLOQUEADO sin AWS**: misma operación en dos nodos Go reales, consistencia de sesión/caché y ausencia de duplicaciones. No se afirma PASS sin despliegue real.
- [x] `E060-CT06` Token revocado nunca reutilizable y sin dependencia del equipo Android. Evidencia: `TestE060ServicioBajaIdempotenteYTokenRevocado` (baja retira cipher y huella; alta posterior crea nuevo ID).

## Evidencia de ejecución real

```
gofmt -w <archivos propios de E059/E060>
go test ./internal/application/pushdevices ./internal/adapters/dynamo ./tests/unit/transport ./cmd/server -run 'E059|E060' -count=1 -timeout 300s
ok  github.com/nexumpr/Nexum-Backend/internal/application/pushdevices
ok  github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo
ok  github.com/nexumpr/Nexum-Backend/tests/unit/transport
ok  github.com/nexumpr/Nexum-Backend/cmd/server  [no tests to run]
```
