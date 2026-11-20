# Checklist de calidad — E045

**Diseño H06:** review. **Implementación y evidencia:** NOT_RUN. **Respuesta prevista:** 200.

- [ ] Firma del Gate H06 por el responsable; no confundir instalación con aprobación.
- [ ] Método/path conservan el ID del inventario H04.
- [ ] Payload válido/negativo, ejemplo y JSON Schema `N/A` revisados.
- [ ] Respuesta `PageOfContribution` y errores de OpenAPI revisados.
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

## Evidencia de implementación local — 2026-11-19

- [x] Método y ruta: `GET /api/v1/goal-contributions`, registrado por `registrarOleada3` (E045).
- [x] Respuesta `PageOfContribution` exacta (`items` + `nextCursor` opcional) verificada por prueba HTTP.
- [x] Propiedad y JWT: sujeto autenticado, sesión autoritativa y aislamiento entre dos usuarios.
- [x] `limit` 1..100, `status` `active|reversed`, `goalId` UUID, `from`/`to` RFC3339 y `from<=to`.
- [x] Sin `Scan`: el adaptador resuelve la consulta con `Query` sobre `USER#<sub>`.
- [x] Cursor opaco firmado con HMAC, ligado al usuario y a la firma de filtros; manipulado → 400.
- [x] Sin clave de cursor o sin persistencia → 503 (nunca una lista vacía silenciosa).
- [x] Importes decimal string y marcas de tiempo UTC (`2006-01-02T15:04:05.000Z`).
- [x] Errores `400/401/429/500/503` con envoltorio común; sin PII ni secretos en la respuesta.
- [x] Pruebas unitarias de transporte ejecutadas: 5 funciones, incluida paginación de dos páginas.
- [ ] Contrato OpenAPI validado en CI: PENDIENTE_CI.
- [ ] HTTP real en dos instancias con TLS: NOT_RUN (requiere Gate de despliegue).
- [ ] Gate individual de release H08: NOT_RUN.
