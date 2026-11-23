# Tareas planificadas — E065

> Estado Oleada 5: IMPLEMENTADO_LOCAL (201). AWS_DEPLOYED=NO; FUNCTIONAL_VERIFIED=PARCIAL.

- [x] `E065-T01` Revisar OpenAPI `operationId=nexum_e065` vs `POST /api/v1/reminders` y contrato de dominio. — OpenAPI y ruta verificados en la rama liz-oleada5.
- [x] `E065-T02` Implementar request/response DTO estrictos `ReminderCreate` / `Reminder`; rechazo de campos desconocidos. — DTO estricto y rechazo de campos desconocidos cubiertos por httptest.
- [x] `E065-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada. — JWT + sesión autoritativa; derivación de userId exclusivamente de sub.
- [x] `E065-T04` Implementar caso de uso y invariantes: Zona horaria IANA válida, programación real sujeta a permisos/costo; no programar envío sin opt-in. — Caso de uso implementado con dependencias reales inyectadas.
- [x] `E065-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / DEVICE#<id>, NOTIFICATION#<UTC>#<id>, REMINDER#<id>, PREFERENCES; GSI2 solo worker due. — Store DynamoDB real con Query/Get/Transact; sin Scan.
- [ ] `E065-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios. — Cuota Redis fail-open verificada; consistencia multi-nodo NO ejecutada.
- [x] `E065-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación. — Códigos y errores locales verificados; ETag/idempotencia donde aplica.
- [x] `E065-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales. — go test de aplicación y transporte PASS.
- [ ] `E065-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI. — PENDIENTE: HTTP real con JWT de AWS y DynamoDB.
- [ ] `E065-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique. — PARCIAL: auth/propiedad/errores locales; concurrencia y carga NO ejecutadas.
- [ ] `E065-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas. — PENDIENTE: CI Linux, go test -race y validadores finales sobre SHA final.
- [ ] `E065-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real. — PENDIENTE: Gate de release; no se desplegó.

## Casos concretos

- [ ] `E065-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema. — PENDIENTE: validación JSON Schema automatizada.
- [x] `E065-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs. — Cubierto por httptest con entradas inválidas.
- [x] `E065-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos. — Cubierto con usuario A vs B sin fuga entre particiones.
- [x] `E065-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial. — 503 fail-closed cubierto con dependencias nulas.
- [ ] `E065-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones. — PENDIENTE: dos nodos Go reales.
- [ ] `E065-CT06` Repetición idéntica de Idempotency-Key -> respuesta estable; payload diferente con misma key -> 409; carrera multiinstancia.
- [ ] `E065-CT07` Opt-in/opt-out efectivo; no enviar push con token revocado; no requerir aceptación del equipo Android.

## Tareas de la enmienda (A8) — PENDIENTES DE EJECUCION

- [ ] Implementar `frequency` (daily|weekly|monthly) opcional; su ausencia significa disparo unico.
- [ ] Anadir la validacion de entrada correspondiente y su mensaje 422.
- [ ] Cubrir con prueba positiva y negativa.
- [ ] Verificar por HTTP real tras el despliegue autorizado.

## Evidencia local Oleada 5

- `gofmt -l cmd internal pkg tests` → sin salida. `go vet ./...`, `go build ./...` → PASS.
- `go test ./... -count=1 -timeout 240s` → PASS, incluidos los paquetes nuevos de Oleada 5.
- `go test -race ./cmd/... ./tests/unit/... ./tests/security/...` → NOT_RUN_LOCAL: CGO/gcc no disponible en este Windows.
- `validar_h06.py` → H06_CONTRACT_PASS; `verificar_publicacion_i150.py paridad` → PARIDAD_OK; `verificar_iac_devsecops.py` → IAC_DEVSECOPS_PASS.
- `staticcheck` y `gosec`: sólo hallazgos heredados; ninguno en archivos nuevos de Oleada 5. CI Linux sobre el SHA final: NOT_RUN.
- `gitleaks`: 4 hallazgos heredados de Oleada 4 en 4 archivos sin diff en esta rama; 0 en Oleada 5. No se reporta secretoscan PASS.
- Sin AWS real, sin JWT de producción, sin dispositivo y sin E2E: `FCM_E2E_PASS=NO`.

**Estado:** código y pruebas locales completos; pendientes Gate de release, AWS, CI Linux y validación JSON Schema.
