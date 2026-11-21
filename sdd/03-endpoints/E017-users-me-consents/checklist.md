# Checklist de calidad — E017

**Diseño H06:** review. **Implementación y evidencia:** NOT_RUN. **Respuesta prevista:** 201.

- [ ] Firma del Gate H06 por el responsable; no confundir instalación con aprobación.
- [ ] Método/path conservan el ID del inventario H04.
- [ ] Payload válido/negativo, ejemplo y JSON Schema `ConsentRequest` revisados.
- [ ] Respuesta `Consent` y errores de OpenAPI revisados.
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

<!-- NOTA_VIGENCIA_AUDITORIA_SEMANTICA -->
### Nota de vigencia - auditoria semantica (2026-11-21)

- Estado: `AUDITADA_SEMANTICA_CON_OBSERVACIONES`.
- Hallazgo: 429 no emitido; `Location` declarado 'si aplica' y no emitido.
- Los bloques anteriores de este archivo conservan el estado historico del endpoint y no se reescriben.
- Evidencia: `memoria.md:1956-1958` (release v0.16.0) y `memoria.md:1968` (29/29 post-hotfix); Gate `GATE_H08E_OLEADA_1.md`.
- Releases: `v0.16.0+7ad1f4a9d6a2` y hotfix `v0.16.1+615ac1c734f2`. El codigo de estos 15 endpoints no cambio desde `7ad1f4a9`.
- Cualquier contradiccion entre bloques anteriores y esta nota se resuelve a favor de esta nota.
