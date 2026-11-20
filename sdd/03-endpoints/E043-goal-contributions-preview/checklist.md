# Checklist de calidad — E043

**Diseño H06:** review. **Implementación y evidencia:** NOT_RUN. **Respuesta prevista:** 200.

- [ ] Firma del Gate H06 por el responsable; no confundir instalación con aprobación.
- [ ] Método/path conservan el ID del inventario H04.
- [ ] Payload válido/negativo, ejemplo y JSON Schema `ContributionPreviewRequest` revisados.
- [ ] Respuesta `ContributionPreview` y errores de OpenAPI revisados.
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

## Oleada 3 (2026-11-19) — E043 `POST /api/v1/goal-contributions/preview`

- [x] DTO cerrado `ContributionPreviewRequest`; campos desconocidos y JSON malformado -> 400.
- [x] Bearer RS256 + sesion autoritativa: sin token, token invalido o sesion revocada -> 401.
- [x] Suma exacta: `sum allocations == amount`; incumplimiento -> 422 `allocations`/`sum_mismatch`.
- [x] Metas repetidas -> 422; meta inexistente -> 404 sin revelar datos internos.
- [x] Sin componentes (servicio, tokens o autorizador) -> 503 `DEPENDENCY_UNAVAILABLE`.
- [x] Cuota por usuario 60/min aplicada dentro de la cadena autenticada; agotada -> 429 con `Retry-After`; Redis caido -> fail-open.
- [x] E043 no escribe: el doble de almacen no recibe ninguna escritura y las metas no cambian.
- [x] Respuesta `ContributionPreview` con `amount`, `currency`, `allocations`, `remaining` y `canCommit`; `Cache-Control: private, no-store`.
- [x] `gofmt`, `go build ./...`, `go vet ./...` y pruebas de transporte E043 PASS en local.
- [ ] CI del PR en verde y Gate de merge por SHA40.
- [ ] Verificacion HTTPS real en AWS (NO ejecutada).

> Las casillas historicas del checklist H06 permanecen sin marcar: no hay evidencia de ejecucion para ellas.
