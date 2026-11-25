# Checklist de calidad — E072

**Diseño H06:** review. **Implementación y evidencia:** CONTRACT_BLOCKED; runtime de Oleada 4 conservado. **Respuesta prevista:** 200/503 (runtime actual).

- [ ] Firma del Gate H06 por el responsable; no confundir instalación con aprobación. — CONTRACT_BLOCKED sin Gate.
- [ ] Método/path conservan el ID del inventario H04. — CONTRACT_BLOCKED sin Gate.
- [ ] Payload válido/negativo, ejemplo y JSON Schema `N/A` revisados. — CONTRACT_BLOCKED sin Gate.
- [ ] Respuesta `Health` y errores de OpenAPI revisados. — CONTRACT_BLOCKED sin Gate.
- [ ] Semántica de propiedad y JWT o excepción pública justificada. — CONTRACT_BLOCKED sin Gate.
- [ ] Idempotencia no exigida específicamente y concurrencia resueltas. — CONTRACT_BLOCKED sin Gate.
- [ ] DynamoDB tiene Get/Query/Transact, localizadores e índices, **nunca Scan**. — CONTRACT_BLOCKED sin Gate.
- [ ] Redis invalidación/TTL/servicio real previsto; fallo seguro. — CONTRACT_BLOCKED sin Gate.
- [ ] Montos decimales/cents y timestamps UTC/zonas IANA correctos donde aplican. — CONTRACT_BLOCKED sin Gate.
- [ ] Pruebas de error 400/401/403/404/409/412/422/429/500/503 pertinentes al caso. — CONTRACT_BLOCKED sin Gate.
PII y secretos ausentes en logs, capturas y fixture. — CONTRACT_BLOCKED sin Gate.
- [ ] Tests unitarios reales, contrato OpenAPI, integración Dynamo y HTTP: NOT_RUN ahora. — CONTRACT_BLOCKED sin Gate.
- [ ] API en dos instancias, TLS público, CI/CD e IaC: NOT_RUN ahora. — CONTRACT_BLOCKED sin Gate.
- [ ] Gate individual de release H08 y rollback aprobados antes de `terraform apply`. — CONTRACT_BLOCKED sin Gate.
- [x] Dependencias del cliente Mobile nunca constituyen Gate del Backend.

**Criterio de cierre:** ninguna casilla operativa se marca por generar este documento; la documentación contractual se valida con el auditor del ZIP y queda REVIEW hasta aprobación.

## Evidencia local Oleada 5

- `gofmt -l cmd internal pkg tests` → sin salida. `go vet ./...`, `go build ./...` → PASS.
- `go test ./... -count=1 -timeout 240s` → PASS, incluidos los paquetes nuevos de Oleada 5.
- `go test -race ./cmd/... ./tests/unit/... ./tests/security/...` → NOT_RUN_LOCAL: CGO/gcc no disponible en este Windows.
- `validar_h06.py` → H06_CONTRACT_PASS (75/300); `verificar_publicacion_i150.py paridad` → PARIDAD_OK; `verificar_iac_devsecops.py` → IAC_DEVSECOPS_PASS.
- `staticcheck` y `gosec`: los hallazgos restantes son heredados de Oleada 4 o anterior; ninguno está en archivos nuevos de Oleada 5. CI Linux sobre el SHA final: NOT_RUN.
- `gitleaks`: 4 hallazgos `generic-api-key` en commits de Oleada 4, en 4 archivos sin diff en esta rama; 0 hallazgos en archivos de Oleada 5. No se reporta secretoscan PASS.
- Sin cambio de código ni pruebas HTTP nuevas: se conserva el runtime `UP` de Oleada 4 y se bloquea la conformidad con el esquema `Health`.
- Sin AWS real, sin JWT de producción, sin dispositivo y sin E2E: `FCM_E2E_PASS=NO`.

**Cierre:** `CONTRACT_BLOCKED`; no se declara conformidad con `Health` sin Gate del propietario.
