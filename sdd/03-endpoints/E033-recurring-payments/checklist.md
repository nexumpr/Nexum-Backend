# Checklist de calidad — E033

**Diseño H06:** review. **Implementación y evidencia:** NOT_RUN. **Respuesta prevista:** 201.

- [ ] Firma del Gate H06 por el responsable; no confundir instalación con aprobación.
- [ ] Método/path conservan el ID del inventario H04.
- [ ] Payload válido/negativo, ejemplo y JSON Schema `RecurringCreate` revisados.
- [ ] Respuesta `RecurringPayment` y errores de OpenAPI revisados.
- [ ] Semántica de propiedad y JWT o excepción pública justificada.
- [ ] Idempotencia obligatoria y concurrencia resueltas.
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
