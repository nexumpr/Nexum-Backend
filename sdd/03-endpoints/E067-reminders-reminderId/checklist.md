# Checklist de calidad — E067

**Diseño H06:** review. **Implementación y evidencia:** IMPLEMENTADO_LOCAL (204 — borrado lógico idempotente); AWS_DEPLOYED=NO; FUNCTIONAL_VERIFIED=PARCIAL.

- [ ] Firma del Gate H06 por el responsable; no confundir instalación con aprobación. — PENDIENTE: no sustituye aprobación humana.
- [x] Método/path conservan el ID del inventario H04. — Registro cerrado de Oleada 5 verificado.
- [ ] Payload válido/negativo, ejemplo y JSON Schema `N/A` revisados. — Revisión documental OK; JSON Schema NO ejecutado.
- [x] Respuesta `sin cuerpo` y errores de OpenAPI revisados. — Esquema y códigos revisados contra OpenAPI.
- [x] Semántica de propiedad y JWT o excepción pública justificada. —  userId derivado de sub; A/B aislados en pruebas locales.
- [x] Idempotencia no exigida específicamente y concurrencia resueltas. — Idempotencia/ETag verificados donde aplica; N/A en el resto.
- [x] DynamoDB tiene Get/Query/Transact, localizadores e índices, **nunca Scan**. — Accesos Query/Get/Transact; sin Scan.
- [ ] Redis invalidación/TTL/servicio real previsto; fallo seguro. — Cuota fail-open verificada; invalidación/TTL NO implementados.
- [x] Montos decimales/cents y timestamps UTC/zonas IANA correctos donde aplican. — Cents/decimales y timestamps UTC/IANA verificados donde aplica.
- [ ] Pruebas de error 400/401/403/404/409/412/422/429/500/503 pertinentes al caso. — SUBCONJUNTO LOCAL verificado; 403/429/500 dependen de middleware/CI.
- [x] PII y secretos ausentes en logs, capturas y fixture. — Sin secretos en respuestas; correos sintéticos en fixtures; logs sin cuerpos de usuario.
- [ ] Tests unitarios reales, contrato OpenAPI, integración Dynamo y HTTP: NOT_RUN ahora. — Unitarios y HTTP locales PASS; integración Dynamo/AWS NO ejecutada.
- [ ] API en dos instancias, TLS público, CI/CD e IaC: NOT_RUN ahora. — NO ejecutado: TLS público, CI/CD e IaC quedan fuera de esta rama.
- [ ] Gate individual de release H08 y rollback aprobados antes de `terraform apply`. — PENDIENTE: sin merge, PR ni despliegue.
- [x] Dependencias del cliente Mobile nunca constituyen Gate del Backend. — No constituyen Gate del Backend.

**Criterio de cierre:** ninguna casilla operativa se marca por generar este documento; la documentación contractual se valida con el auditor del ZIP y queda REVIEW hasta aprobación.

## Evidencia local Oleada 5

- `gofmt -l cmd internal pkg tests` → sin salida. `go vet ./...`, `go build ./...` → PASS.
- `go test ./... -count=1 -timeout 240s` → PASS, incluidos los paquetes nuevos de Oleada 5.
- `go test -race ./cmd/... ./tests/unit/... ./tests/security/...` → NOT_RUN_LOCAL: CGO/gcc no disponible en este Windows.
- `validar_h06.py` → H06_CONTRACT_PASS (75/300); `verificar_publicacion_i150.py paridad` → PARIDAD_OK; `verificar_iac_devsecops.py` → IAC_DEVSECOPS_PASS.
- `staticcheck` y `gosec`: los hallazgos restantes son heredados de Oleada 4 o anterior; ninguno está en archivos nuevos de Oleada 5. CI Linux sobre el SHA final: NOT_RUN.
- `gitleaks`: 4 hallazgos `generic-api-key` en commits de Oleada 4, en 4 archivos sin diff en esta rama; 0 hallazgos en archivos de Oleada 5. No se reporta secretoscan PASS.
- Pruebas HTTP locales: httptest 204/401/404/503 y borrado lógico idempotente.
- Sin AWS real, sin JWT de producción, sin dispositivo y sin E2E: `FCM_E2E_PASS=NO`.

**Cierre:** no se marca Gate, AWS, CI Linux, dos nodos ni JSON Schema; ver `tasks.md` para el detalle.
