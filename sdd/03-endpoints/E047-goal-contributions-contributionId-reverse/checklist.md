# Checklist de calidad — E047

**Diseño H06:** review. **Implementación y evidencia:** NOT_RUN. **Respuesta prevista:** 200.

- [ ] Firma del Gate H06 por el responsable; no confundir instalación con aprobación.
- [ ] Método/path conservan el ID del inventario H04.
- [ ] Payload válido/negativo, ejemplo y JSON Schema `ReverseContributionRequest` revisados.
- [ ] Respuesta `Contribution` y errores de OpenAPI revisados.
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


## Cierre local E047 (2026-11-19)

- [x] Handler, wiring y pruebas locales.
- [ ] PR y CI Linux.
- [ ] Certificacion HTTPS y despliegue AWS.


## Hotfix 2026-11-20 — reversion E047

- Causa raiz confirmada con una transaccion de diagnostico no destructiva contra DynamoDB: la `UpdateExpression` de la aportacion usa `:one` en `ADD #version :one`, pero `ExpressionAttributeValues` no declaraba `:one`; AWS devolvia `ValidationException` y el adaptador lo traducia a 503.
- Correccion: declarar `:one` en la actualizacion de la aportacion. La transaccion sigue siendo atomica y condicionada.
- Prueba de regresion: `TestE047StoreExpresionDeclaraTodosLosValores` captura `TransactWriteItemsInput` y exige que todo marcador usado en las expresiones tenga valor declarado.
- Estado: corregido en rama de hotfix; AWS pendiente de nueva release y certificacion.


## Verificación AWS v0.19.2 (2026-11-21)

- Corregido y desplegado en `v0.19.2+3098f379d7ab`; HTTPS real y DynamoDB consistente.
- Evidencia: `/health/live` 200, `/health/ready` 200; ASG 2/2/2; dos targets healthy.

<!-- NOTA_VIGENCIA_AUDITORIA_SEMANTICA -->
### Nota de vigencia - auditoria semantica (2026-11-21)

- Estado: `AUDITADA_SEMANTICA_CON_OBSERVACIONES`.
- Hallazgo: PR #136 cerrado sin merge; codigo por #139; hotfix #140 corrigio el `:one`.
- Los bloques anteriores de este archivo conservan el estado historico del endpoint y no se reescriben.
- Evidencia: `memoria.md:2238` (13/13 en v0.19.2) y `GATE_H08E_OLEADA_3.md` §12.1; CI de main run 44838845962.
- Norma aplicable: RFC-022 v1.2.1 (APPROVED). Release inicial `v0.19.0+3baa15840a2e`; release vigente `v0.19.2+3098f379d7ab`.
- Artefacto HTTP individual por endpoint: `EVIDENCIA_FUNCIONAL_INDIVIDUAL_NO_LOCALIZADA`; la certificacion consta de forma agregada en memoria y Gate.
- Cualquier contradiccion entre bloques anteriores y esta nota se resuelve a favor de esta nota.
