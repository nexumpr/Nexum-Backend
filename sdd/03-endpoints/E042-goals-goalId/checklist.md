# Checklist de calidad — E042

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


## Cierre local E042 (2026-11-19)

- [x] Alternativa B, tombstone, wiring y pruebas locales.
- [ ] PR y CI Linux.
- [ ] Certificación HTTPS y despliegue AWS.


## Hotfix 2026-11-20 — E042 `:one` en tombstone

- Causa raiz confirmada en AWS: la `UpdateExpression` de `DeleteGoal` usa `ADD #version :one`, pero `ExpressionAttributeValues` no declaraba `:one`; DynamoDB devolvia `ValidationException` y el adaptador lo traducia a 503 tras la reversion de la aportacion.
- Correccion: declarar `:one` como Number. La condicion de version y `activeContributions` se conserva.
- Prueba de regresion: `TestE042UpdateItemDeclaraTodosLosValores` captura el `UpdateItemInput` real y exige que todos los marcadores tengan valor.
- Estado: corregido en rama de hotfix; AWS pendiente de release y certificacion.
