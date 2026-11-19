# Checklist de calidad — E028

**Diseño H06:** review. **Implementación y evidencia:** NOT_RUN. **Respuesta prevista:** 201.

- [ ] Firma del Gate H06 por el responsable; no confundir instalación con aprobación.
- [ ] Método/path conservan el ID del inventario H04.
- [ ] Payload válido/negativo, ejemplo y JSON Schema `MovementCreate` revisados.
- [ ] Respuesta `Movement` y errores de OpenAPI revisados.
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

## Oleada 2 (2026-11-19) — E028 `POST /api/v1/movements`

| Campo | Valor |
|---|---|
| Estado | IMPLEMENTED · PR_OPEN · CI_GREEN (12/12) · MERGED=YES · AWS_DEPLOYED=NO · HTTPS_VERIFIED=NO · FUNCTIONAL_VERIFIED=NO |
| PR | #102 · HEAD `089d7e7aac35908ac209bdc16488230ae61ac6d2` |
| Claves DynamoDB | USER#<id>/MOVEMENT#<occurredAt>#<id> + MOVEMENTPTR#<id> + IDEM# |
| Decisiones | Idempotency-Key obligatoria con UserIdempotencyKeys (PK por usuario); transaccion unica recurso+puntero+registro; importes int64 centimos. |
| Errores y causa raiz | La carrera de idempotencia se resolvia por indice de CancellationReasons; ahora el registro de idempotencia es la autoridad (commit 089d7e7). |
| Pruebas | `go build`, `go vet` y suites de aplicacion/adaptador/transporte en verde; donde Windows Application Control bloqueo el binario se declara NOT_RUN_LOCAL y la autoridad es el CI Linux. |
| Contrato Android | Ruta y esquema segun `sdd/03-endpoints/openapi/nexum-api-v1.json`; errores con `ErrorEnvelope` y `traceId`. |
| Pendiente | Merge autorizado, release agrupada y verificacion HTTP real. |
