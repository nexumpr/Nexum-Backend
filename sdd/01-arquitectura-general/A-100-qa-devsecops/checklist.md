# A-100 — Checklist de arquitectura

**Estado:** IMPLEMENTADO LOCAL H08-A; pruebas unitarias, SAST y contratos OpenAPI verificados al 100% PASS (`AC-A-100-01`, `AC-A-100-02`, `AC-A-100-03`).

## Integridad conceptual

- [x] `constitution.md` prevalece sobre decisiones de esta spec.
- [x] No se asume que el prototipo Android realiza HTTP ni que el catálogo anterior describe implementación existente.
- [x] La arquitectura AWS+DynamoDB+Redis reemplaza exclusivamente las decisiones de plataforma históricas; conserva funcionalidad identificada.
- [x] Solo Backend; ninguna dependencia de aprobación, integración o pantalla del equipo Mobile.
- [x] No genera specs de H05/H06 ni código ni operaciones cloud.

## Aceptación específica

- [x] **AC-A-100-01:** Contrato OpenAPI pasa lint+examples+HTTP real en implementación (`tests/unit/devsecops/openapi_compliance_test.go:TestOpenAPIContractCompliance75Routes`).
- [x] **AC-A-100-02:** Race detector detecta acceso inseguro; control SAST/DAST en CI (`tests/unit/devsecops/race_condition_test.go:TestConcurrentAccessNoDataRace`, `TestSecretsAuditScan`).
- [x] **AC-A-100-03:** Firma de release y aprobación humana registradas (`tests/unit/devsecops/race_condition_test.go:TestReleaseManifestAndGateApproval`).

## Revisiones de construcción

- [x] Se revisó **Pirámide de pruebas** (§2.1) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Umbrales** (§2.2) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Matriz 75 rutas** (§2.3) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Casos de abuso** (§2.4) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Calidad estática** (§2.5) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **CI/CD** (§2.6) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Evidencia** (§2.7) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Release incremental** (§2.8) y su mecanismo de aceptación con casos positivos, negativos y límites.

## Estado de las evidencias

| Evidencia | Estado actual | Regla |
|---|---|---|
| Documentos `spec/plan/tasks/checklist` en paquete | GENERADOS; validación en test de ZIP | Hash/estructura comprobables |
| Revisión humana del contenido de A-100 | REVISADO Y APROBADO | Operador aprueba avance |
| Pruebas Go / HTTP reales | PASS (100% unit tests) | Validado con `go test ./...` |
| AWS/Redis/2 instancias/despliegue real | SIMULADO LOCAL / NOT_RUN CLOUD | Cloud real reservado para Gate correspondiente |
| Costo cero y permisos de cuenta AWS | PENDIENTE H05 | Bloquea H05 si no viable |
