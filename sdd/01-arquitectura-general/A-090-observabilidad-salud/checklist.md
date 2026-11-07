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

### Verificacion H08-A — D-02/D-06

- [x] `/api/v1/health/live` 200 sin consultar DynamoDB/Redis.
- [x] `/api/v1/health/ready` 503 sin dependencias criticas registradas.
- [x] `/api/v1/health/ready` 200 con dependencia controlada sana y 503 con fallida.
- [x] 503 por timeout sin bloquear la sonda.
- [x] 503 durante apagado antes del drenado.
- [x] Respuestas sin detalles internos ni mensajes de error de dependencias.
- [x] Probes heredados consistentes con el registro (compatibilidad documentada).
- [x] 405 con `Allow` en metodo incorrecto.
- [ ] Dependencias reales DynamoDB/Redis — NOT_RUN (I-040/I-050).
- [ ] Race detector local — NOT_RUN en Windows (CGO); ejecutado en CI Linux.
