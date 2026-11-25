# Tareas planificadas — E073

> Estado Oleada 5: CONTRACT_BLOCKED. No se modifica HealthRegistry ni router.RegisterHealth sin Gate del propietario.

- [ ] `E073-T01` Revisar OpenAPI `operationId=nexum_e073` vs `GET /api/v1/health/ready` y contrato de dominio. — PENDIENTE: requiere Gate para cambiar la respuesta observable.
- [ ] `E073-T02` Implementar request/response DTO estrictos `ninguno` / `Health`; rechazo de campos desconocidos. — PENDIENTE: requiere Gate para cambiar la respuesta observable.
- [ ] `E073-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada. — PENDIENTE: requiere Gate para cambiar la respuesta observable.
- [ ] `E073-T04` Implementar caso de uso y invariantes: 503 cuando incapaz de servir peticiones; comprobar dependencias según H05 sin filtrar credenciales. — PENDIENTE: requiere Gate para cambiar la respuesta observable.
- [ ] `E073-T05` Implementar adaptador DynamoDB y patrón: Readiness consulta dependencias sin exponer tablas; liveness solo proceso (sin escritura). — PENDIENTE: requiere Gate para cambiar la respuesta observable.
- [ ] `E073-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios. — PENDIENTE: requiere Gate para cambiar la respuesta observable.
- [ ] `E073-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación. — PENDIENTE: requiere Gate para cambiar la respuesta observable.
- [ ] `E073-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales. — PENDIENTE: requiere Gate para cambiar la respuesta observable.
- [ ] `E073-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI. — PENDIENTE: requiere Gate para cambiar la respuesta observable.
- [ ] `E073-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique. — PENDIENTE: requiere Gate para cambiar la respuesta observable.
- [ ] `E073-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas. — PENDIENTE: requiere Gate para cambiar la respuesta observable.
- [ ] `E073-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real. — PENDIENTE: requiere Gate para cambiar la respuesta observable.

## Casos concretos

- [ ] `E073-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema. — PENDIENTE: requiere Gate para cambiar la respuesta observable.
- [ ] `E073-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs. — PENDIENTE: requiere Gate para cambiar la respuesta observable.
- [ ] `E073-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos. — PENDIENTE: requiere Gate para cambiar la respuesta observable.
- [ ] `E073-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial. — PENDIENTE: requiere Gate para cambiar la respuesta observable.
- [ ] `E073-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones. — PENDIENTE: requiere Gate para cambiar la respuesta observable.
- [ ] `E073-CT06` Nodo individual: /live no depende de DB, /ready devuelve 503 al fallar dependencia; el ALB saca solo el nodo no listo. — PENDIENTE: requiere Gate para cambiar la respuesta observable.

## Evidencia local Oleada 5

- `gofmt -l cmd internal pkg tests` → sin salida. `go vet ./...`, `go build ./...` → PASS.
- `go test ./... -count=1 -timeout 240s` → PASS, incluidos los paquetes nuevos de Oleada 5.
- `go test -race ./cmd/... ./tests/unit/... ./tests/security/...` → NOT_RUN_LOCAL: CGO/gcc no disponible en este Windows.
- `validar_h06.py` → H06_CONTRACT_PASS; `verificar_publicacion_i150.py paridad` → PARIDAD_OK; `verificar_iac_devsecops.py` → IAC_DEVSECOPS_PASS.
- `staticcheck` y `gosec`: sólo hallazgos heredados; ninguno en archivos nuevos de Oleada 5. CI Linux sobre el SHA final: NOT_RUN.
- `gitleaks`: 4 hallazgos heredados de Oleada 4 en 4 archivos sin diff en esta rama; 0 en Oleada 5. No se reporta secretoscan PASS.
- Sin cambio observable: no se duplicaron rutas ni se tocó HealthRegistry; se mantiene el runtime `UP` vigente.

**Estado:** `CONTRACT_BLOCKED`; no se declara conformidad con `Health` sin Gate del propietario.
