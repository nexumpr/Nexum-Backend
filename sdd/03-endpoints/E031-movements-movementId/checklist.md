# Checklist de calidad — E031

**Diseño H06:** review. **Implementación y evidencia:** NOT_RUN. **Respuesta prevista:** 204.

- [ ] Firma del Gate H06 por el responsable; no confundir instalación con aprobación.
- [ ] Método/path conservan el ID del inventario H04.
- [ ] Payload válido/negativo, ejemplo y JSON Schema `N/A` revisados.
- [ ] Respuesta `sin cuerpo` y errores de OpenAPI revisados.
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
