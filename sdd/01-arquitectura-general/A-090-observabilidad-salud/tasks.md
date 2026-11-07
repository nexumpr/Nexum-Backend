# A-090 — Tareas auditables del diseño

**Etapa:** H08-A, implementación Go local y documentación de arquitectura. **Estado:** IMPLEMENTADO LOCAL H08-A; pruebas unitarias y contratos verificados al 100% PASS.

## A. Revisión técnica secuencial

- [x] **A-090-T01 — Trazabilidad.** Revisar con usuario la regla de `spec.md` §2.1; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** `internal/infrastructure/telemetry/tracer.go` implementa W3C traceparent y correlación `AC-A-090-01`.
- [x] **A-090-T02 — Logs.** Revisar con usuario la regla de `spec.md` §2.2; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** `internal/infrastructure/telemetry/redactor.go` sanitiza contraseñas, JWT, Bearer tokens y PII (`AC-A-090-03`).
- [x] **A-090-T03 — Métricas.** Revisar con usuario la regla de `spec.md` §2.3; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** integración de métricas de latencia y estado de dependencias.
- [x] **A-090-T04 — Health.** Revisar con usuario la regla de `spec.md` §2.4; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** `internal/transport/http/health_handler.go` provee `/live` (200 UP) y `/ready` (503 si falla dependencia) cumpliendo `AC-A-090-02`.
- [x] **A-090-T05 — Alertas.** Revisar con usuario la regla de `spec.md` §2.5; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** modelo de alertas y estados de salud desacoplados.
- [x] **A-090-T06 — Retención.** Revisar con usuario la regla de `spec.md` §2.6; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** logs JSON estructurados sin persistencia de PII en texto claro.
- [x] **A-090-T07 — Pruebas operativas.** Revisar con usuario la regla de `spec.md` §2.7; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** degradación controlada de Redis/DynamoDB probada en `observability_health_test.go`.
- [x] **A-090-T08 — Objetivos iniciales.** Revisar con usuario la regla de `spec.md` §2.8; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** contratos de probes y trazabilidad listos para métricas de latencia.

## B. Tareas transversales de aceptación

- [x] **A-090-T09:** Validar todas las referencias de fuente académica y contradicciones con catálogo histórico.
- [x] **A-090-T10:** Resolver riesgos de seguridad/datos de la especificación o registrar bloqueo con responsable y Gate de resolución.
- [x] **A-090-T11:** Revisar el contrato con los otros 11 documentos de H04; comprobar no introducir requisitos Mobile ni fases H05/H06.
- [x] **A-090-T12:** Recoger aprobación humana expresa del Gate H04. Hasta entonces ninguna task documental se da por `APPROVED` por instalar el ZIP.

## C. Ejecución futura después de H07

Las tareas de implementación, pruebas HTTP, CI/CD y despliegue pertenecen a H08+, nunca se marcan completadas por el documento. Cada cambio requiere release autorizado.

## Hotfix H08-A/A-090 — D-02 y D-06 (2026-11-06)

- Gate `sdd/_control/H08/GATE_H08A_HOTFIX_A090_HEALTH.md`.
- Rutas normativas `GET /api/v1/health/live` y `GET /api/v1/health/ready` registradas en el router real mediante `RegisterHealth`.
- Liveness no consulta dependencias; readiness devuelve 503 durante apagado, sin dependencias registradas, con dependencia caida o por timeout (pendientes `UNKNOWN`, sin bloqueo).
- `run()` conecta el registro al runtime y marca `BeginShutdown()` antes del drenado.
- Probes heredados `/health` y `/ready` se conservan como compatibilidad delegando en el mismo registro.
- Pruebas nuevas: `tests/unit/transport/health_normative_test.go` (8 casos, HTTP real en puerto efimero).
- Evidencia: go build/vet/test PASS, gosec 0, govulncheck PASS, SDD PASS, OpenAPI 75. race: NOT_RUN en Windows (CGO), se ejecuta en CI.
