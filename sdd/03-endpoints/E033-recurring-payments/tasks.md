# Tareas planificadas — E033

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E033-T01` Revisar OpenAPI `operationId=nexum_e033` vs `POST /api/v1/recurring-payments` y contrato de dominio.
- [ ] `E033-T02` Implementar request/response DTO estrictos `RecurringCreate` / `RecurringPayment`; rechazo de campos desconocidos.
- [ ] `E033-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E033-T04` Implementar caso de uso y invariantes: Monto positivo, frecuencia válida y fecha siguiente consistente; crear regla sin generar pago.
- [ ] `E033-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / RECURRING#<id>; transacción de pago con MOVEMENT#<UTC>#<id> e idempotencia.
- [ ] `E033-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E033-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E033-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E033-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E033-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E033-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E033-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E033-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E033-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E033-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E033-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E033-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E033-CT06` Montos exactos en centavos: 0, mínimo, máximo, decimal inválido, cambio de mes y zona IANA.
- [ ] `E033-CT07` Repetición idéntica de Idempotency-Key -> respuesta estable; payload diferente con misma key -> 409; carrera multiinstancia.

## Tareas de la enmienda (A8) — PENDIENTES DE EJECUCION

- [ ] Implementar `type`, `startDate` y `dueDay` en request y respuesta.
- [ ] Anadir la validacion de entrada correspondiente y su mensaje 422.
- [ ] Cubrir con prueba positiva y negativa.
- [ ] Verificar por HTTP real tras el despliegue autorizado.

## Oleada 2 (2026-11-19) — E033 `POST /api/v1/recurring-payments`

| Campo | Valor |
|---|---|
| Estado | IMPLEMENTED=YES · MERGED=YES · PR_OPEN=NO · CI_GREEN=YES (12/12) · MERGED=YES · AWS_DEPLOYED=NO · HTTPS_VERIFIED=NO · FUNCTIONAL_VERIFIED=NO |
| PR | #107 · HEAD `ed70d395df456c5af40822b17e43423478d22364` |
| Claves DynamoDB | USER#<id>/RECURRING#<id> + IDEM# |
| Decisiones | RFC-021: acepta 9 frecuencias, persiste y emite 7 canonicas (biweekly->fortnightly, yearly->annual); Idempotency-Key por usuario; no genera pagos (E037 fuera de alcance). |
| Errores y causa raiz | U1000 por helper de prueba sin uso, corregido en d6294b5. |
| Pruebas | `go build`, `go vet` y suites de aplicacion/adaptador/transporte en verde; donde Windows Application Control bloqueo el binario se declara NOT_RUN_LOCAL y la autoridad es el CI Linux. |
| Contrato Android | Ruta y esquema segun `sdd/03-endpoints/openapi/nexum-api-v1.json`; errores con `ErrorEnvelope` y `traceId`. |
| Pendiente | Publicacion AWS, verificacion HTTP real y aceptacion posterior. |

> **Registro operativo vigente (Oleada 2):** el bloque anterior de esta seccion es la evidencia de la implementacion y el merge. Las casillas `[ ]` del plan historico H06 no se marcan retroactivamente sin evidencia de ejecucion.

### Cierre Oleada 2 - release v0.18.0+9722224c15c5

| Campo | Valor |
|---|---|
| Implementacion | CONCLUIDA |
| PR de codigo | #107 (fusionado) |
| CI | PASS (13/13 checks del PR) |
| Contrato | OpenAPI 3.1 vigente, paridad JSON/YAML |
| Publicacion AWS | DEPLOYED: release `v0.18.0+9722224c15c5`, LT v14 |
| Verificacion HTTPS | PASS (CloudFront `d2lpmqy5w6x9es`) |
| Persistencia DynamoDB | PASS (relecturas y consultas consistentes) |
| Seguridad | PASS (ownership, aislamiento, revocacion) |
| Aceptacion funcional | PASS - 201 x9; alias normalizados; idempotencia |
| Rollback | LT v13 / release `v0.17.0+87aac167fe28` |
| Limitaciones | multi-AZ limitada (ambas EC2 en us-east-2a); E015 pendiente legal; SES pendiente |

<!-- NOTA_VIGENCIA_AUDITORIA_SEMANTICA -->
### Nota de vigencia - auditoria semantica (2026-11-21)

- Estado: `AUDITADA_SEMANTICA_CON_OBSERVACIONES`.
- Hallazgo: Sin hallazgos materiales; idempotencia por usuario y frecuencias RFC-021 verificadas en pruebas.
- Los bloques anteriores de este archivo conservan el estado historico del endpoint y no se reescriben.
- Estado vigente segun la auditoria semantica del PR #142 (2026-11-21):
- Evidencia funcional agregada: `sdd/_control/H08/CIERRE_OLEADA_2.md` y `memoria.md`, secciones "Oleada 2 - registro por endpoint" y "Cierre documental Oleada 2".
- Release inicial desplegada: `v0.17.0+87aac167fe28`. Release correctiva del hotfix E014: `v0.18.0+9722224c15c5`.
- Evidencia funcional individual por endpoint: `EVIDENCIA_FUNCIONAL_INDIVIDUAL_NO_LOCALIZADA`.
- Cualquier contradiccion entre bloques anteriores y esta nota se resuelve a favor de esta nota.
