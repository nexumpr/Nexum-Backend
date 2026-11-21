# Checklist de calidad — E009

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

## Criterios de aceptacion de la enmienda (A1)

- [ ] El campo aparece en el contrato OpenAPI (YAML y JSON en paridad).
- [ ] Se persiste y se devuelve en la respuesta de creacion.
- [ ] Se consulta en el detalle y en el listado con el mismo tipo y formato.
- [ ] Se puede editar (si aplica) y la edicion se refleja al releer.
- [ ] Validacion negativa devuelve 422 sin filtrar datos internos.

<!-- NOTA_VIGENCIA_AUDITORIA_SEMANTICA -->
### Nota de vigencia - auditoria semantica (2026-11-21)

- Estado: `AUDITADA_SEMANTICA_CON_OBSERVACIONES`.
- Hallazgo: 429 declarado y no emitido; 409/422 individuales en AWS no localizados.
- Los bloques anteriores de este archivo conservan el estado historico del endpoint y no se reescriben.
- Evidencia: `sdd/_control/H08/GATE_H08E_E007_E011_DESPLIEGUE_AWS.md` §4 y los Gates especificos de E001-E006; `memoria.md` cierre E001/E002 y v0.15.0.
- Release inicial: ver la matriz de auditoria del PR #142. Ultima verificacion funcional agregada: `v0.15.0+8f13c86d2ca0`.
- Cualquier contradiccion entre bloques anteriores y esta nota se resuelve a favor de esta nota.
