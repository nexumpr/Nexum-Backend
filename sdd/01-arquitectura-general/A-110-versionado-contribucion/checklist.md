# A-110 — Checklist de arquitectura

**Estado:** IMPLEMENTADO LOCAL H08-A; pruebas unitarias, gobernanza y gates humanos PASS (`AC-A-110-01`, `AC-A-110-02`, `AC-A-110-03`).

## Integridad conceptual

- [x] `constitution.md` prevalece sobre decisiones de esta spec.
- [x] No se asume que el prototipo Android realiza HTTP ni que el catálogo anterior describe implementación existente.
- [x] La arquitectura AWS+DynamoDB+Redis reemplaza exclusivamente las decisiones de plataforma históricas; conserva funcionalidad identificada.
- [x] Solo Backend; ninguna dependencia de aprobación, integración o pantalla del equipo Mobile.
- [x] No genera specs de H05/H06 ni código ni operaciones cloud.

## Aceptación específica

- [x] **AC-A-110-01:** Historial de decisiones y razones accesible (`tests/unit/system/version_governance_test.go:TestSemVerFormatAndDecisionLogAccessibility`).
- [x] **AC-A-110-02:** Cada PR con CI y autorizar deploy separado (`tests/unit/system/version_governance_test.go:TestPRGatePolicyAndDeployDecoupling`).
- [x] **AC-A-110-03:** Cambios a spec aprobada tienen RFC y revisión (`tests/unit/system/version_governance_test.go:TestRFCChangeControlProcess`).

## Revisiones de construcción

- [x] Se revisó **Versionamiento** (§2.1) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Ramas** (§2.2) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Orden SDD** (§2.3) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Cambios** (§2.4) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Agentes y permisos** (§2.5) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Criterio de DONE** (§2.6) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Repositorio** (§2.7) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Transferencia** (§2.8) y su mecanismo de aceptación con casos positivos, negativos y límites.

## Estado de las evidencias

| Evidencia | Estado actual | Regla |
|---|---|---|
| Documentos `spec/plan/tasks/checklist` en paquete | GENERADOS; validación en test de ZIP | Hash/estructura comprobables |
| Revisión humana del contenido de A-110 | REVISADO Y APROBADO | Operador aprueba avance |
| Pruebas Go / HTTP reales | PASS (100% unit tests) | Validado con `go test ./...` |
| AWS/Redis/2 instancias/despliegue real | SIMULADO LOCAL / NOT_RUN CLOUD | Cloud real reservado para Gate correspondiente |
| Costo cero y permisos de cuenta AWS | PENDIENTE H05 | Bloquea H05 si no viable |
