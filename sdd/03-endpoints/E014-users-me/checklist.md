# Checklist de calidad — E014

**Diseño H06:** review. **Implementación y evidencia:** NOT_RUN. **Respuesta prevista:** 202.

- [ ] Firma del Gate H06 por el responsable; no confundir instalación con aprobación.
- [ ] Método/path conservan el ID del inventario H04.
- [ ] Payload válido/negativo, ejemplo y JSON Schema `DeleteAccountRequest` revisados.
- [ ] Respuesta `DeletionReceipt` y errores de OpenAPI revisados.
- [ ] Semántica de propiedad y JWT o excepción pública justificada.
- [ ] Idempotencia no exigida específicamente y concurrencia resueltas.
- [ ] DynamoDB tiene Get/Query/Transact, localizadores e índices, **nunca Scan**.
- [ ] Redis invalidación/TTL/servicio real previsto; fallo seguro.
- [ ] Montos decimales/cents y timestamps UTC/zonas IANA correctos donde aplican.
- [ ] Pruebas de error 400/401/403/404/409/412/422/429/500/503 pertinentes al caso.
- [ ] PII y secretos ausentes en logs, capturas y fixture.
- [ ] Tests unitarios reales, contrato OpenAPI, integración Dynamo y HTTP: NOT_RUN ahora.
- [ ] API en dos instancias, TLS público, CI/CD e IaC: NOT_RUN ahora.
- [ ] Gate individual de release H08 y rollback aprobados antes de `terraform apply`.
- [ ] Dependencias del cliente Mobile nunca constituyen Gate del Backend.

**Criterio de cierre:** ninguna casilla operativa se marca por generar este documento; la documentación contractual se valida con el auditor del ZIP y queda REVIEW hasta aprobación.

## Criterios de aceptacion de la enmienda (A8)

- [ ] El campo aparece en el contrato OpenAPI (YAML y JSON en paridad).
- [ ] Se persiste y se devuelve en la respuesta de creacion.
- [ ] Se consulta en el detalle y en el listado con el mismo tipo y formato.
- [ ] Se puede editar (si aplica) y la edicion se refleja al releer.
- [ ] Validacion negativa devuelve 422 sin filtrar datos internos.

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
