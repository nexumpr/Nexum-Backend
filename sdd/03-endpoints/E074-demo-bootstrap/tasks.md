# Tareas planificadas — E074

> Estado Oleada 5: IMPLEMENTADO_LOCAL (200). AWS_DEPLOYED=NO; FUNCTIONAL_VERIFIED=PARCIAL.

- [x] `E074-T01` Revisar OpenAPI `operationId=nexum_e074` vs `GET /api/v1/demo/bootstrap` y contrato de dominio. — OpenAPI y ruta verificados en la rama liz-oleada5.
- [x] `E074-T02` Implementar request/response DTO estrictos `ninguno` / `DemoBootstrap`; rechazo de campos desconocidos. — DTO estricto y rechazo de campos desconocidos cubiertos por httptest.
- [x] `E074-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada. — JWT + sesión autoritativa; derivación de userId exclusivamente de sub.
- [x] `E074-T04` Implementar caso de uso y invariantes: Opcional: si se autoriza, aislar por entorno y sujeto; nunca exponer en producción a usuarios reales. — Caso de uso implementado con dependencias reales inyectadas.
- [x] `E074-T05` Implementar adaptador DynamoDB y patrón: Demo aislada de producción, sin datos reales y habilitación por environment flag de IaC. — N/A verificado: el caso de uso no persiste ni altera DynamoDB.
- [ ] `E074-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios. — Cuota Redis fail-open verificada; consistencia multi-nodo NO ejecutada.
- [x] `E074-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación. — Códigos y errores locales verificados; ETag/idempotencia donde aplica.
- [x] `E074-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales. — go test de aplicación y transporte PASS.
- [ ] `E074-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI. — PENDIENTE: HTTP real con JWT de AWS y DynamoDB.
- [ ] `E074-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique. — PARCIAL: auth/propiedad/errores locales; concurrencia y carga NO ejecutadas.
- [ ] `E074-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas. — PENDIENTE: CI Linux, go test -race y validadores finales sobre SHA final.
- [ ] `E074-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real. — PENDIENTE: Gate de release; no se desplegó.

## Casos concretos

- [ ] `E074-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema. — PENDIENTE: validación JSON Schema automatizada.
- [x] `E074-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs. — Cubierto por httptest con entradas inválidas.
- [x] `E074-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos. — Cubierto con usuario A vs B sin fuga entre particiones.
- [x] `E074-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial. — 503 fail-closed cubierto con dependencias nulas.
- [ ] `E074-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones. — PENDIENTE: dos nodos Go reales.

## Evidencia local Oleada 5

- `gofmt -l cmd internal pkg tests` → sin salida. `go vet ./...`, `go build ./...` → PASS.
- `go test ./... -count=1 -timeout 240s` → PASS, incluidos los paquetes nuevos de Oleada 5.
- `go test -race ./cmd/... ./tests/unit/... ./tests/security/...` → NOT_RUN_LOCAL: CGO/gcc no disponible en este Windows.
- `validar_h06.py` → H06_CONTRACT_PASS; `verificar_publicacion_i150.py paridad` → PARIDAD_OK; `verificar_iac_devsecops.py` → IAC_DEVSECOPS_PASS.
- `staticcheck` y `gosec`: sólo hallazgos heredados; ninguno en archivos nuevos de Oleada 5. CI Linux sobre el SHA final: NOT_RUN.
- `gitleaks`: 4 hallazgos heredados de Oleada 4 en 4 archivos sin diff en esta rama; 0 en Oleada 5. No se reporta secretoscan PASS.
- Sin AWS real, sin JWT de producción, sin dispositivo y sin E2E: `FCM_E2E_PASS=NO`.

**Estado:** código y pruebas locales completos; pendientes Gate de release, AWS, CI Linux y validación JSON Schema.
