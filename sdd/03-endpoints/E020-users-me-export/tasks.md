# Tareas planificadas — E020

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E020-T01` Revisar OpenAPI `operationId=nexum_e020` vs `GET /api/v1/users/me/export` y contrato de dominio.
- [ ] `E020-T02` Implementar request/response DTO estrictos `ninguno` / `UserExport`; rechazo de campos desconocidos.
- [ ] `E020-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E020-T04` Implementar caso de uso y invariantes: Exportación sin datos de otros usuarios, paginación interna y límites; respuesta JSON redacted sin tokens.
- [ ] `E020-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / PROFILE, CONSENT#*, ONBOARDING, PREFERENCES; privacidad y revocación transversal.
- [ ] `E020-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E020-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E020-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E020-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E020-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E020-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E020-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E020-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E020-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E020-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E020-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E020-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.


## Cierre local E020 (2026-11-19, actualizado 2026-11-20)

- **Implementado:** `GET /api/v1/users/me/export` con paginacion `limit`/cursor HMAC, orden fijo movements/contributions/recurringPayments/goals/budgets, singletons profile/income/consents/onboarding/preferences y `nextCursor` solo al agotar.
- **OpenAPI:** `UserExport` ampliado con las seis propiedades opcionales aprobadas por RFC-022 v1.2.1; JSON/YAML en paridad y `PUBLICACION_VALIDA`.
- **Privacidad:** DTO publicos, sin hashes/tokens/secretos, sin Scan; errores 503 sin cursor.
- **Wiring:** `cmd/server/e020.go`; `montarOleada3` monta el handler con dependencias reales.
- **PR:** #137 (`feat/h08e-e020-export`), HEAD `669408eb083e2d59869d2ddf6ef5a10105c7604c` al registrar esta evidencia. Sin merge ni AWS.

## Evidencia de pruebas (2026-11-20)

| Capa | Prueba | Resultado | Limite |
|---|---|---|---|
| Aplicacion | `tests/unit/application/export_test.go` (paginacion, orden fijo, limite, cursor ligado a usuario/coleccion/vencimiento, fail-closed, perfil ausente, fallo de lectura, consentimientos completos, meta incoherente, `exportedAt`) | **PASS** | Doble de `Fuente`; no DynamoDB real |
| Adaptador | `tests/unit/dynamo/export_store_test.go` (Query por particion, `ConsistentRead`, orden, `Limit`, `ExclusiveStartKey`/`LastEvaluatedKey`, aislamiento, error DynamoDB, items incoherentes, presupuestos, familias vacias, reflexion sin `Scan`) | **PASS** | Cliente DynamoDB falso fiel; no AWS |
| Transporte | `tests/unit/transport/export_test.go` (200 autenticado, 11 propiedades, `profile` de 6 campos, `Cache-Control`, 401, 503, `limit`, cursor valido/manipulado/ajeno/vencido/otra coleccion, 400/404/500, sin secretos, sin `nextCursor` en error) | **PASS** | JWT/sesion de prueba; no HTTPS publico |
| Integracion del lote | `TestOleada3TreceRutasMontadas` y `cmd/server/registro_oleada3_rutas_test.go` | **PASS** | Acredita registro/routing y fail-closed; no negocio positivo |
| CI Linux | Run `44753828797` sobre `98d38571c6a8d0c5cf27b4e6eb977ab0b52fe031` (integracion temporal #138): gofmt, vet, staticcheck, build, test, `-race`, fuzzing, cobertura 87.1%, gosec, govulncheck, Semgrep, gitleaks, Trivy, contratos, integracion y Quality Gate | **PASS** | Combinacion de los 16 PR; el PR #137 apilado solo ejecuta `validar` |
| AWS / dos nodos / HTTPS | Exportacion real contra DynamoDB/Redis y CloudFront | **NOT_RUN** | Requiere release y Gate de despliegue |

**Defecto real corregido:** la reanudacion de `Presupuestos` usaba un rango abierto `SK >= BUDGETSET#<periodo>`, que no puede combinarse con `begins_with` y arrastraba prefijos ajenos de la particion. Se sustituyo por `begins_with` del periodo y continuacion por el prefijo completo desde la SK del periodo; prueba de regresion en `tests/unit/dynamo/export_store_test.go`.

**No se declara:** AWS desplegado, HTTPS verificado, dos nodos con E020 ni aceptacion Android. Esas verificaciones pertenecen a la certificacion posterior al despliegue.
