# A-090 — Checklist de arquitectura

**Estado:** IMPLEMENTADO LOCAL H08-A; pruebas unitarias y de observabilidad Go PASS (`AC-A-090-01`, `AC-A-090-02`, `AC-A-090-03`).

## Integridad conceptual

- [x] `constitution.md` prevalece sobre decisiones de esta spec.
- [x] No se asume que el prototipo Android realiza HTTP ni que el catálogo anterior describe implementación existente.
- [x] La arquitectura AWS+DynamoDB+Redis reemplaza exclusivamente las decisiones de plataforma históricas; conserva funcionalidad identificada.
- [x] Solo Backend; ninguna dependencia de aprobación, integración o pantalla del equipo Mobile.
- [x] No genera specs de H05/H06 ni código ni operaciones cloud.

## Aceptación específica

- [x] **AC-A-090-01:** Una solicitud se rastrea handler->case->Dynamo sin filtrar datos (`tests/unit/telemetry/observability_health_test.go:TestTraceContextPropagationAndCorrelation`).
- [x] **AC-A-090-02:** ready=503 cuando no se puede atender operación crítica (`tests/unit/telemetry/observability_health_test.go:TestReadinessProbeDegradation503`).
- [x] **AC-A-090-03:** Logs rotados/redactados (`tests/unit/telemetry/observability_health_test.go:TestStructuredLogsRedactionAndSanitization`).

## Revisiones de construcción

- [x] Se revisó **Trazabilidad** (§2.1) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Logs** (§2.2) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Métricas** (§2.3) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Health** (§2.4) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Alertas** (§2.5) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Retención** (§2.6) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Pruebas operativas** (§2.7) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Objetivos iniciales** (§2.8) y su mecanismo de aceptación con casos positivos, negativos y límites.

## Estado de las evidencias

| Evidencia | Estado actual | Regla |
|---|---|---|
| Documentos `spec/plan/tasks/checklist` en paquete | GENERADOS; validación en test de ZIP | Hash/estructura comprobables |
| Revisión humana del contenido de A-090 | REVISADO Y APROBADO | Operador aprueba avance |
| Pruebas Go / HTTP reales | PASS (100% unit tests) | Validado con `go test ./...` |
| AWS/Redis/2 instancias/despliegue real | SIMULADO LOCAL / NOT_RUN CLOUD | Cloud real reservado para Gate correspondiente |
| Costo cero y permisos de cuenta AWS | PENDIENTE H05 | Bloquea H05 si no viable |
