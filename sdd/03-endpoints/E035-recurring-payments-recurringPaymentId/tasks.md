# Tareas planificadas — E035

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E035-T01` Revisar OpenAPI `operationId=nexum_e035` vs `PATCH /api/v1/recurring-payments/{recurringPaymentId}` y contrato de dominio.
- [ ] `E035-T02` Implementar request/response DTO estrictos `RecurringPatch` / `RecurringPayment`; rechazo de campos desconocidos.
- [ ] `E035-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E035-T04` Implementar caso de uso y invariantes: If-Match; cambios no alteran historial de pagos ejecutados.
- [ ] `E035-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / RECURRING#<id>; transacción de pago con MOVEMENT#<UTC>#<id> e idempotencia.
- [ ] `E035-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E035-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E035-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E035-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E035-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E035-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E035-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E035-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E035-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E035-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E035-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E035-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E035-CT06` Montos exactos en centavos: 0, mínimo, máximo, decimal inválido, cambio de mes y zona IANA.
- [ ] `E035-CT07` If-Match omitido -> 428; ETag viejo -> 412; carrera de versiones no pierde actualizaciones.

## Tareas de la enmienda (A8) — PENDIENTES DE EJECUCION

- [ ] Implementar `type`, `startDate` y `dueDay` editables en request y respuesta.
- [ ] Anadir la validacion de entrada correspondiente y su mensaje 422.
- [ ] Cubrir con prueba positiva y negativa.
- [ ] Verificar por HTTP real tras el despliegue autorizado.

## Oleada 2 (2026-11-19) — E035 `PATCH /api/v1/recurring-payments/{recurringPaymentId}`

| Campo | Valor |
|---|---|
| Estado | IMPLEMENTED=YES · MERGED=YES · PR_OPEN=NO · CI_GREEN=YES (12/12) · MERGED=YES · AWS_DEPLOYED=NO · HTTPS_VERIFIED=NO · FUNCTIONAL_VERIFIED=NO |
| PR | #110 · HEAD `2c76fa80abcea43a4fe5e864a3ef7044b47a77f3` |
| Claves DynamoDB | USER#<id>/RECURRING#<id> |
| Decisiones | If-Match 428/412; frequency no editable (campo desconocido 400); dueDay tri-estado; type inmutable 422; no-op sin subir version. |
| Errores y causa raiz | NextDueDate se atribuia a dueDay no enviado; corregido en el PR. |
| Pruebas | `go build`, `go vet` y suites de aplicacion/adaptador/transporte en verde; donde Windows Application Control bloqueo el binario se declara NOT_RUN_LOCAL y la autoridad es el CI Linux. |
| Contrato Android | Ruta y esquema segun `sdd/03-endpoints/openapi/nexum-api-v1.json`; errores con `ErrorEnvelope` y `traceId`. |
| Pendiente | Publicacion AWS, verificacion HTTP real y aceptacion posterior. |

> **Registro operativo vigente (Oleada 2):** el bloque anterior de esta seccion es la evidencia de la implementacion y el merge. Las casillas `[ ]` del plan historico H06 no se marcan retroactivamente sin evidencia de ejecucion.

### Cierre Oleada 2 - release v0.18.0+9722224c15c5

| Campo | Valor |
|---|---|
| Implementacion | CONCLUIDA |
| PR de codigo | #110 (fusionado) |
| CI | PASS (13/13 checks del PR) |
| Contrato | OpenAPI 3.1 vigente, paridad JSON/YAML |
| Publicacion AWS | DEPLOYED: release `v0.18.0+9722224c15c5`, LT v14 |
| Verificacion HTTPS | PASS (CloudFront `d2lpmqy5w6x9es`) |
| Persistencia DynamoDB | PASS (relecturas y consultas consistentes) |
| Seguridad | PASS (ownership, aislamiento, revocacion) |
| Aceptacion funcional | PASS - 200 v2; 412 stale; persistencia |
| Rollback | LT v13 / release `v0.17.0+87aac167fe28` |
| Limitaciones | multi-AZ limitada (ambas EC2 en us-east-2a); E015 pendiente legal; SES pendiente |

<!-- NOTA_VIGENCIA_AUDITORIA_SEMANTICA -->
### Nota de vigencia - auditoria semantica (2026-11-21)

- Estado: `AUDITADA_SEMANTICA_CON_OBSERVACIONES`.
- Hallazgo: El cuerpo `{}` se trata como no-op 200; asimetria con E041 registrada, sin cambio de codigo.
- Los bloques anteriores de este archivo conservan el estado historico del endpoint y no se reescriben.
- Estado vigente segun la auditoria semantica del PR #142 (2026-11-21):
- Evidencia funcional agregada: `sdd/_control/H08/CIERRE_OLEADA_2.md` y `memoria.md`, secciones "Oleada 2 - registro por endpoint" y "Cierre documental Oleada 2".
- Release inicial desplegada: `v0.17.0+87aac167fe28`. Release correctiva del hotfix E014: `v0.18.0+9722224c15c5`.
- Evidencia funcional individual por endpoint: `EVIDENCIA_FUNCIONAL_INDIVIDUAL_NO_LOCALIZADA`.
- Cualquier contradiccion entre bloques anteriores y esta nota se resuelve a favor de esta nota.
