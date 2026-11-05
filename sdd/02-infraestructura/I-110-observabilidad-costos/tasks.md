# I-110-observabilidad-costos — CloudWatch, métricas, tracing y monitoreo FinOps

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-090, I-000, I-020, I-030 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## Gestión de tareas H05 (solo especificación y aceptación de diseño)

Estado inicial de todos los trabajos: `TODO`, salvo evidencia local de integridad H04 documentada por el responsable. No marcar como completado por generar archivos.

### T-110-01 — Cerrar logs

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-090, I-000, I-020, I-030.
- **Trabajo:** JSON fields timestamp, severity, service, environment, traceId, requestId, route template, status, latencyMs, instanceId; redacción automática de password, tokens, raw voice, emails, montos no necesarios.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-110-02 — Cerrar métricas

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-090, I-000, I-020, I-030.
- **Trabajo:** latencia p50/p95/p99, RPS, 4xx/5xx, target healthy/unhealthy, Dynamo consumed capacity/throttle/latency, Redis latency/evictions/memory, Go GC/runtime; OpenTelemetry a sistema aprobado.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-110-03 — Cerrar alertas

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-090, I-000, I-020, I-030.
- **Trabajo:** ALB target count <2, 5xx, API ready fail, caché indisponible, errores de autenticación anómalos, budget/Free Tier 85/100%, métricas rotas. Severidad + runbook.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-110-04 — Cerrar retención

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-090, I-000, I-020, I-030.
- **Trabajo:** CloudWatch Logs con días de retención finitos (p.ej. 7/14 como propuesta), no infinito; revisar precio ingest/storage/extracción/alarms y evitar trace 100% por defecto.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-110-05 — Cerrar trazabilidad

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-090, I-000, I-020, I-030.
- **Trabajo:** TraceId propagado por header confiable, IDs anónimos; no registrar claims JWT completos, token push, secretos, cuentas ajenas.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-110-06 — Cerrar finops

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-090, I-000, I-020, I-030.
- **Trabajo:** Comparar gasto/usage y balance créditos diariamente durante ventanas, generar reporte antes/después de apagado; alarmas no equivalen a límite de gasto garantizado.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-110-90 — Revisión de riesgos y dependencias

- **Estado:** TODO.
- **Trabajo:** resolver o escalar: Altos costos de logs/trazas, PII inadvertida, Alarmas insuficientes para evitar cargos.
- **Criterio DoD:** cada riesgo clasificado RESOLVED/BLOCKED/ACCEPTED con firma/fecha; no usar ACCEPTED para incumplir requisito docente sin aclaración.

### T-110-99 — Ratificación de la especificación

- **Estado:** REVIEW pendiente.
- **Entrada:** `spec.md`, `plan.md`, `tasks.md`, `checklist.md` actualizados.
- **Salida:** decisión explícita del responsable y vínculo a evidencia en informe I-GO.
- **Bloqueo:** no crear Terraform/endpoints ni ejecutar AWS.
