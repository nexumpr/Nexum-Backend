# Tareas planificadas — E028

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E028-T01` Revisar OpenAPI `operationId=nexum_e028` vs `POST /api/v1/movements` y contrato de dominio.
- [ ] `E028-T02` Implementar request/response DTO estrictos `MovementCreate` / `Movement`; rechazo de campos desconocidos.
- [ ] `E028-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E028-T04` Implementar caso de uso y invariantes: Una única contabilización por Idempotency-Key; categorías válidas y monto positivo; transacción con proyecciones si aplica.
- [ ] `E028-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / MOVEMENT#<UTC>#<id>; MOVEMENT_ID#<id> locator; GSI1 para filtros por tipo/fecha.
- [ ] `E028-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E028-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E028-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E028-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E028-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E028-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E028-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E028-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E028-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E028-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E028-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E028-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E028-CT06` Montos exactos en centavos: 0, mínimo, máximo, decimal inválido, cambio de mes y zona IANA.
- [ ] `E028-CT07` Repetición idéntica de Idempotency-Key -> respuesta estable; payload diferente con misma key -> 409; carrera multiinstancia.

## Oleada 2 (2026-11-19) — E028 `POST /api/v1/movements`

| Campo | Valor |
|---|---|
| Estado | IMPLEMENTED=YES · MERGED=YES · PR_OPEN=NO · CI_GREEN=YES (12/12) · MERGED=YES · AWS_DEPLOYED=NO · HTTPS_VERIFIED=NO · FUNCTIONAL_VERIFIED=NO |
| PR | #102 · HEAD `089d7e7aac35908ac209bdc16488230ae61ac6d2` |
| Claves DynamoDB | USER#<id>/MOVEMENT#<occurredAt>#<id> + MOVEMENTPTR#<id> + IDEM# |
| Decisiones | Idempotency-Key obligatoria con UserIdempotencyKeys (PK por usuario); transaccion unica recurso+puntero+registro; importes int64 centimos. |
| Errores y causa raiz | La carrera de idempotencia se resolvia por indice de CancellationReasons; ahora el registro de idempotencia es la autoridad (commit 089d7e7). |
| Pruebas | `go build`, `go vet` y suites de aplicacion/adaptador/transporte en verde; donde Windows Application Control bloqueo el binario se declara NOT_RUN_LOCAL y la autoridad es el CI Linux. |
| Contrato Android | Ruta y esquema segun `sdd/03-endpoints/openapi/nexum-api-v1.json`; errores con `ErrorEnvelope` y `traceId`. |
| Pendiente | Publicacion AWS, verificacion HTTP real y aceptacion posterior. |

> **Registro operativo vigente (Oleada 2):** el bloque anterior de esta seccion es la evidencia de la implementacion y el merge. Las casillas `[ ]` del plan historico H06 no se marcan retroactivamente sin evidencia de ejecucion.

### Cierre Oleada 2 - release v0.18.0+9722224c15c5

| Campo | Valor |
|---|---|
| Implementacion | CONCLUIDA |
| PR de codigo | #102 (fusionado) |
| CI | PASS (13/13 checks del PR) |
| Contrato | OpenAPI 3.1 vigente, paridad JSON/YAML |
| Publicacion AWS | DEPLOYED: release `v0.18.0+9722224c15c5`, LT v14 |
| Verificacion HTTPS | PASS (CloudFront `d2lpmqy5w6x9es`) |
| Persistencia DynamoDB | PASS (relecturas y consultas consistentes) |
| Seguridad | PASS (ownership, aislamiento, revocacion) |
| Aceptacion funcional | PASS - 201; replay identico; 409 con payload distinto; importes exactos |
| Rollback | LT v13 / release `v0.17.0+87aac167fe28` |
| Limitaciones | multi-AZ limitada (ambas EC2 en us-east-2a); E015 pendiente legal; SES pendiente |

<!-- NOTA_VIGENCIA_AUDITORIA_SEMANTICA -->
### Nota de vigencia - auditoria semantica (2026-11-21)

- Estado: `AUDITADA_SEMANTICA_CON_OBSERVACIONES`.
- Hallazgo: La clave fisica implementada es `MOVEMENTPTR#<id>` (decision D-W2-02), no `MOVEMENT_ID#`; los campos requeridos ausentes responden 422 y no 400.
- Los bloques anteriores de este archivo conservan el estado historico del endpoint y no se reescriben.
- Estado vigente segun la auditoria semantica del PR #142 (2026-11-21):
- Evidencia funcional agregada: `sdd/_control/H08/CIERRE_OLEADA_2.md` y `memoria.md`, secciones "Oleada 2 - registro por endpoint" y "Cierre documental Oleada 2".
- Release inicial desplegada: `v0.17.0+87aac167fe28`. Release correctiva del hotfix E014: `v0.18.0+9722224c15c5`.
- Evidencia funcional individual por endpoint: `EVIDENCIA_FUNCIONAL_INDIVIDUAL_NO_LOCALIZADA`.
- Cualquier contradiccion entre bloques anteriores y esta nota se resuelve a favor de esta nota.
