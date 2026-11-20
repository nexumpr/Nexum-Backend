# Checklist de calidad — E050

**Diseño H06:** review. **Implementación y evidencia:** NOT_RUN. **Respuesta prevista:** 200.

- [ ] Firma del Gate H06 por el responsable; no confundir instalación con aprobación.
- [ ] Método/path conservan el ID del inventario H04.
- [ ] Payload válido/negativo, ejemplo y JSON Schema `N/A` revisados.
- [ ] Respuesta `PageOfBudgetUsage` y errores de OpenAPI revisados.
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


## Cierre local E050 (2026-11-19)

- [x] Implementacion local, wiring y pruebas.
- [ ] PR y CI Linux.
- [ ] Certificacion HTTPS y despliegue AWS.


## Hotfix 2026-11-20 — consulta DynamoDB

- Causa raiz confirmada en AWS: `KeyConditionExpression` con `#sk >= :from AND #sk < :to`; DynamoDB solo admite una condicion de rango por clave (`ValidationException`), lo que producia 503.
- Correccion: `BETWEEN :from AND :to` con descarte explicito del borde superior para conservar el intervalo `[from, to)` y paginacion por `LastEvaluatedKey`.
- Prueba de regresion: `tests/unit/dynamo/query_condition_regression_test.go` captura la `QueryInput` efectiva, exige `BETWEEN` y comprueba que el borde superior no se contabiliza.
- Estado: corregido en rama de hotfix; AWS pendiente de nueva release y certificacion.
