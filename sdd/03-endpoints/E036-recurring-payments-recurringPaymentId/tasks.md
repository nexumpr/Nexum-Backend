# Tareas planificadas — E036

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E036-T01` Revisar OpenAPI `operationId=nexum_e036` vs `DELETE /api/v1/recurring-payments/{recurringPaymentId}` y contrato de dominio.
- [ ] `E036-T02` Implementar request/response DTO estrictos `ninguno` / `ninguno`; rechazo de campos desconocidos.
- [ ] `E036-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E036-T04` Implementar caso de uso y invariantes: Desactivar regla sin borrar movimientos de pagos anteriores; idempotente.
- [ ] `E036-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / RECURRING#<id>; transacción de pago con MOVEMENT#<UTC>#<id> e idempotencia.
- [ ] `E036-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E036-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E036-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E036-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E036-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E036-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E036-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E036-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E036-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E036-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E036-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E036-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E036-CT06` Montos exactos en centavos: 0, mínimo, máximo, decimal inválido, cambio de mes y zona IANA.

## Oleada 2 (2026-11-19) — E036 `DELETE /api/v1/recurring-payments/{recurringPaymentId}`

| Campo | Valor |
|---|---|
| Estado | IMPLEMENTED=YES · MERGED=YES · PR_OPEN=NO · CI_GREEN=YES (12/12) · MERGED=YES · AWS_DEPLOYED=NO · HTTPS_VERIFIED=NO · FUNCTIONAL_VERIFIED=NO |
| PR | #111 · HEAD `39d100782e4a42938a4622eec1be6616978339e4` |
| Claves DynamoDB | USER#<id>/RECURRING#<id> |
| Decisiones | 204 idempotente (segundo DELETE no sube version); condicion que alcanza items heredados sin atributo active. |
| Errores y causa raiz | U1000/SA4010 heredados de la linea, corregidos en 004c69d. |
| Pruebas | `go build`, `go vet` y suites de aplicacion/adaptador/transporte en verde; donde Windows Application Control bloqueo el binario se declara NOT_RUN_LOCAL y la autoridad es el CI Linux. |
| Contrato Android | Ruta y esquema segun `sdd/03-endpoints/openapi/nexum-api-v1.json`; errores con `ErrorEnvelope` y `traceId`. |
| Pendiente | Publicacion AWS, verificacion HTTP real y aceptacion posterior. |

> **Registro operativo vigente (Oleada 2):** el bloque anterior de esta seccion es la evidencia de la implementacion y el merge. Las casillas `[ ]` del plan historico H06 no se marcan retroactivamente sin evidencia de ejecucion.
