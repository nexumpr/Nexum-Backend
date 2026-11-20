# Checklist de calidad — E020

**Diseño H06:** review. **Implementación y evidencia:** IMPLEMENTED_LOCAL; pruebas de aplicacion, adaptador y transporte PASS; CI del conjunto PASS; AWS/HTTPS/dos nodos NOT_RUN. **Respuesta prevista:** 200.

- [ ] Firma del Gate H06 por el responsable; no confundir instalación con aprobación.
- [ ] Método/path conservan el ID del inventario H04.
- [ ] Payload válido/negativo, ejemplo y JSON Schema `N/A` revisados.
- [ ] Respuesta `UserExport` y errores de OpenAPI revisados.
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


## Cierre local E020 (2026-11-19, actualizado 2026-11-20)

- [x] OpenAPI ampliado, paginacion, DTO, adaptador y wiring; pruebas de aplicacion, adaptador y transporte PASS.
- [x] Nunca `Scan`: comprobado por reflexion sobre las interfaces del adaptador y por las consultas emitidas.
- [x] Aislamiento por usuario, cursor HMAC (manipulado/ajeno/vencido/otra coleccion), errores sin `nextCursor` y sin secretos.
- [x] CI Linux del conjunto: run `44753828797` sobre `98d38571c6a8d0c5cf27b4e6eb977ab0b52fe031` (12 jobs + SDD) PASS; cobertura de dominio 87.1%.
- [ ] CI Go del PR #137 individual: el PR apilado solo ejecuta `validar`; se ejecutara al retargetear a `main`.
- [ ] Certificacion HTTPS, DynamoDB/Redis reales, dos nodos y despliegue AWS: NOT_RUN; requieren release y Gate posterior.
- [ ] Aceptacion Android V6.0.2: NOT_RUN; fuente no disponible en el entorno.
