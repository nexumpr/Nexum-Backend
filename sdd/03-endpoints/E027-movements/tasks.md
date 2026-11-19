# Tareas planificadas — E027

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E027-T01` Revisar OpenAPI `operationId=nexum_e027` vs `GET /api/v1/movements` y contrato de dominio.
- [ ] `E027-T02` Implementar request/response DTO estrictos `ninguno` / `PageOfMovement`; rechazo de campos desconocidos.
- [ ] `E027-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E027-T04` Implementar caso de uso y invariantes: Query por usuario, fecha, tipo y cursor firmado; orden determinista timestamp+ID; nunca Scan.
- [ ] `E027-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / MOVEMENT#<UTC>#<id>; MOVEMENT_ID#<id> locator; GSI1 para filtros por tipo/fecha.
- [ ] `E027-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E027-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E027-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E027-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E027-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E027-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E027-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E027-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E027-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E027-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E027-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E027-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E027-CT06` Montos exactos en centavos: 0, mínimo, máximo, decimal inválido, cambio de mes y zona IANA.
- [ ] `E027-CT07` Paginación 25/100, filtro modificado con cursor antiguo -> 400, lectura sin Scan, sin duplicados en páginas.

## Oleada 2 (2026-11-19) — E027 `GET /api/v1/movements`

| Campo | Valor |
|---|---|
| Estado | IMPLEMENTED=YES · MERGED=YES · PR_OPEN=NO · CI_GREEN=YES (12/12) · MERGED=YES · AWS_DEPLOYED=NO · HTTPS_VERIFIED=NO · FUNCTIONAL_VERIFIED=NO |
| PR | #103 · HEAD `3947ad82ffffd78795419d6db1bcc384c3ab3b22` |
| Claves DynamoDB | USER#<id> con begins_with(MOVEMENT#) |
| Decisiones | Cursor opaco firmado (httputil) ligado a filtros y usuario; limit 1..100; filtros type/from/to en servidor; sin Scan. |
| Errores y causa raiz | Ninguno propio; hereda la correccion de idempotencia de E028. |
| Pruebas | `go build`, `go vet` y suites de aplicacion/adaptador/transporte en verde; donde Windows Application Control bloqueo el binario se declara NOT_RUN_LOCAL y la autoridad es el CI Linux. |
| Contrato Android | Ruta y esquema segun `sdd/03-endpoints/openapi/nexum-api-v1.json`; errores con `ErrorEnvelope` y `traceId`. |
| Pendiente | Publicacion AWS, verificacion HTTP real y aceptacion posterior. |

> **Registro operativo vigente (Oleada 2):** el bloque anterior de esta seccion es la evidencia de la implementacion y el merge. Las casillas `[ ]` del plan historico H06 no se marcan retroactivamente sin evidencia de ejecucion.
