# Tareas planificadas - E031

> **Nota de vigencia (2026-11-24).** El borrado se implementó en local como **tombstone lógico con
> `204` idempotente y puntero `MOVEMENTPTR#` conservado**, conforme a
> `sdd/_control/H08/DECISION_E031_BORRADO.md` v1.1.0 y a las autorizaciones
> `APRUEBO_E031_IDEMPOTENTE_204_CON_PUNTERO` / `APRUEBO_CIERRE_IMPLEMENTACION_E031_TOMBSTONE`.
> Las casillas de esta ficha describen el plan histórico y **no** se reescriben; la evidencia real
> vive en `memoria.md` y en las pruebas de `tests/unit/{dynamo,application,transport}`. El
> despliegue AWS y la aceptación por HTTP siguen pendientes de su Gate.

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E031-T01` Revisar OpenAPI `operationId=nexum_e031` vs `DELETE /api/v1/movements/{movementId}` y contrato de dominio.
- [ ] `E031-T02` Implementar request/response DTO estrictos `ninguno` / `ninguno`; rechazo de campos desconocidos.
- [ ] `E031-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E031-T04` Implementar caso de uso y invariantes: Soft delete o reverso auditable; no permitir borrar registro ligado a aportaciones confirmadas sin reversa.
- [ ] `E031-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / MOVEMENT#<UTC>#<id>; MOVEMENT_ID#<id> locator; GSI1 para filtros por tipo/fecha.
- [ ] `E031-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E031-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E031-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E031-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E031-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E031-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E031-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E031-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E031-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E031-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E031-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E031-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E031-CT06` Montos exactos en centavos: 0, mínimo, máximo, decimal inválido, cambio de mes y zona IANA.

## Oleada 2 (2026-11-19) — E031 `DELETE /api/v1/movements/{movementId}`

| Campo | Valor |
|---|---|
| Estado | IMPLEMENTED=YES · MERGED=YES · PR_OPEN=NO · CI_GREEN=YES (12/12) · MERGED=YES · AWS_DEPLOYED=NO · HTTPS_VERIFIED=NO · FUNCTIONAL_VERIFIED=NO |
| PR | #106 · HEAD `14d11527750bd4459f193fffedda2e8f811e1c4d` |
| Claves DynamoDB | MOVEMENTPTR# + MOVEMENT# |
| Decisiones | 204 sin cuerpo; 409 si el movimiento tiene aportacion confirmada; borrado transaccional con reintento si el item se movio. |
| Errores y causa raiz | Ninguno propio. |
| Pruebas | `go build`, `go vet` y suites de aplicacion/adaptador/transporte en verde; donde Windows Application Control bloqueo el binario se declara NOT_RUN_LOCAL y la autoridad es el CI Linux. |
| Contrato Android | Ruta y esquema segun `sdd/03-endpoints/openapi/nexum-api-v1.json`; errores con `ErrorEnvelope` y `traceId`. |
| Pendiente | Publicacion AWS, verificacion HTTP real y aceptacion posterior. |

> **Registro operativo vigente (Oleada 2):** el bloque anterior de esta seccion es la evidencia de la implementacion y el merge. Las casillas `[ ]` del plan historico H06 no se marcan retroactivamente sin evidencia de ejecucion.

### Cierre Oleada 2 - release v0.18.0+9722224c15c5

| Campo | Valor |
|---|---|
| Implementacion | CONCLUIDA |
| PR de codigo | #106 (fusionado) |
| CI | PASS (13/13 checks del PR) |
| Contrato | OpenAPI 3.1 vigente, paridad JSON/YAML |
| Publicacion AWS | DEPLOYED: release `v0.18.0+9722224c15c5`, LT v14 |
| Verificacion HTTPS | PASS (CloudFront `d2lpmqy5w6x9es`) |
| Persistencia DynamoDB | PASS (relecturas y consultas consistentes) |
| Seguridad | PASS (ownership, aislamiento, revocacion) |
| Aceptacion funcional | PASS - 204; 404 posterior; 404 ajeno |
| Rollback | LT v13 / release `v0.17.0+87aac167fe28` |
| Limitaciones | multi-AZ limitada (ambas EC2 en us-east-2a); E015 pendiente legal; SES pendiente |

<!-- NOTA_VIGENCIA_AUDITORIA_SEMANTICA -->
### Nota de vigencia - auditoria semantica (2026-11-21)

- Estado: `DISCREPANCIA_DOCUMENTAL_REGISTRADA`.
- Hallazgo: `spec.md` exige soft delete o reverso auditable y la implementacion borra fisicamente el item y su puntero. Requiere RFC/decision humana; no se corrige en este PR documental.
- Los bloques anteriores de este archivo conservan el estado historico del endpoint y no se reescriben.
- Estado vigente segun la auditoria semantica del PR #142 (2026-11-21):
- Evidencia funcional agregada: `sdd/_control/H08/CIERRE_OLEADA_2.md` y `memoria.md`, secciones "Oleada 2 - registro por endpoint" y "Cierre documental Oleada 2".
- Release inicial desplegada: `v0.17.0+87aac167fe28`. Release correctiva del hotfix E014: `v0.18.0+9722224c15c5`.
- Evidencia funcional individual por endpoint: `EVIDENCIA_FUNCIONAL_INDIVIDUAL_NO_LOCALIZADA`.
- Cualquier contradiccion entre bloques anteriores y esta nota se resuelve a favor de esta nota.
