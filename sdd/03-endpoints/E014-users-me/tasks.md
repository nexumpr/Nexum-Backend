# Tareas planificadas — E014

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E014-T01` Revisar OpenAPI `operationId=nexum_e014` vs `DELETE /api/v1/users/me` y contrato de dominio.
- [ ] `E014-T02` Implementar request/response DTO estrictos `DeleteAccountRequest` / `DeletionReceipt`; rechazo de campos desconocidos.
- [ ] `E014-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E014-T04` Implementar caso de uso y invariantes: Marcar tombstone y revocar sesiones de inmediato; trabajo idempotente auditable para purga por lotes; respetar retenciones legales declaradas.
- [ ] `E014-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / PROFILE, CONSENT#*, ONBOARDING, PREFERENCES; privacidad y revocación transversal.
- [ ] `E014-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E014-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E014-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E014-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E014-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E014-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E014-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E014-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E014-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E014-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E014-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E014-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E014-CT06` Revocación/rotación en nodo A persiste y es observable inmediatamente en nodo B; token reutilizado rechazado.

## Tareas de la enmienda (A8) — PENDIENTES DE EJECUCION

- [ ] Implementar `deletionRequestId` (uuid) en la respuesta 202.
- [ ] Anadir la validacion de entrada correspondiente y su mensaje 422.
- [ ] Cubrir con prueba positiva y negativa.
- [ ] Verificar por HTTP real tras el despliegue autorizado.

## Oleada 2 (2026-11-19) — E014 `DELETE /api/v1/users/me`

| Campo | Valor |
|---|---|
| Estado | IMPLEMENTED=YES · MERGED=YES · PR_OPEN=NO · CI_GREEN=YES (12/12) · MERGED=YES · AWS_DEPLOYED=NO · HTTPS_VERIFIED=NO · FUNCTIONAL_VERIFIED=NO |
| PR | #116 · HEAD `ea38d23` |
| Claves DynamoDB | USER#<id>/DELETION#<requestId> + QUEUE#DELETION/<requestId> + EMAIL# + REFRESH# |
| Decisiones | Bloqueo y revocacion inmediata (status DELETION_PENDING + sessionEpoch+1); worker con lease Redis, fence en DynamoDB, fases COLLECT/EXTERNAL/PARTITION/VERIFY/TERMINAL; PURGED solo tras verificacion; 503 si el worker no esta activo. |
| Errores y causa raiz | G101 (constante con Password), G115 (int32 sin rango), G602 (indice de slice) y U1000; colision de isConditionalCheckFailed con recurrentes resuelta renombrando en E014. |
| Pruebas | `go build`, `go vet` y suites de aplicacion/adaptador/transporte en verde; donde Windows Application Control bloqueo el binario se declara NOT_RUN_LOCAL y la autoridad es el CI Linux. |
| Contrato Android | Ruta y esquema segun `sdd/03-endpoints/openapi/nexum-api-v1.json`; errores con `ErrorEnvelope` y `traceId`. |
| Pendiente | Publicacion AWS, verificacion HTTP real y aceptacion posterior. |

> **Registro operativo vigente (Oleada 2):** el bloque anterior de esta seccion es la evidencia de la implementacion y el merge. Las casillas `[ ]` del plan historico H06 no se marcan retroactivamente sin evidencia de ejecucion.

## Hotfix E014 — invalidacion de cache sin SCAN (2026-11-19)

- **Defecto real en produccion:** el trabajo de purga llegaba a `phase=TERMINAL` y quedaba en `state=RETRY` con `attempts=3`. Causa: `UserCacheInvalidator.DeletePrefix` usa `SCAN`, comando denegado por el ACL Redis de A-080 (`-scan`). El error se trataba como fallo de dependencia y bloqueaba el cierre.
- **Correccion:** en `pasoTerminal` se distingue la denegacion por ACL (`NOPERM`) de una indisponibilidad real. Con `NOPERM` se registra `cache_invalidation_denied` y se continua con la verificacion durable; con cualquier otro error se mantiene `RETRY`. No se habilita SCAN, no se modifica el ACL, IAM ni Terraform.
- **Justificacion:** tras el tombstone (`DELETION_PENDING` + `sessionEpoch+1`) ninguna ruta autenticada puede servir datos del usuario (el middleware exige `PROFILE ACTIVE`); las entradas de cache son inalcanzables y su TTL esta acotado (30 s en agregados). El trabajo conserva la verificacion de particion, localizadores y cola antes de `PURGED`.
- **Pruebas:** `TestE014InvalidacionDenegadaPorACLCompletaPurga` (NOPERM -> PURGED) y `TestE014InvalidacionTransitoriaNoProducePurged` (Redis caido -> RETRY, nunca PURGED), ademas de las 11 pruebas previas del worker.
- **Estado:** corregido en rama `fix/h08e-e014-cache-invalidation`; merge, release y verificacion AWS pendientes de sus Gates.
