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


### Implementacion H08 (2026-11-08)

- T-110-01 Logs: IMPLEMENTADO_LOCAL. RequestRecord con campos canonicos y validacion de claves sensibles; POLITICA_LOGS.md; redaccion A-090.
- T-110-02 Metricas: IMPLEMENTADO_LOCAL. 3 metric filters (5xx, 401, readiness), metricas ALB y consultas p50/p95/p99 por Logs Insights.
- T-110-03 Alertas: IMPLEMENTADO_LOCAL. 5 alarmas (targets<2, 5xx, 401, readiness, logs sin eventos), SNS opcional y runbooks A1-A5.
- T-110-04 Retencion: IMPLEMENTADO_LOCAL. /nexum/<entorno>/backend con 7 dias por defecto y maximo 30 por precondition; coste I-000.
- T-110-05 Trazabilidad: IMPLEMENTADO_LOCAL. traceId/requestId de A-090; dashboard Logs Insights; sin claims/tokens/PII.
- T-110-06 FinOps: IMPLEMENTADO_LOCAL. Presupuesto 85/100, 3 filters + 5 alarmas (+0.2000 USD/mes estimado, no medido) y dashboard gratuito.
- Modulo observabilidad cableado en envs/h08i con ARNs de I-030; dashboard condicional. AWS_NOT_RUN.
- Pruebas: 10 de contrato Terraform, 6 de alineacion FinOps y 6 de RequestRecord.

- Bloqueo SNS (2026-11-08): con alias/aws/sns Trivy marca AWS-0136 (HIGH, exige CMK). No se crea CMK por coste no presupuestado; se retiran SNS, alarm_actions y notificaciones del presupuesto. ALERT_DELIVERY = BLOCKED_RFC007_SNS_KMS; alarmas visibles con runbook; RFC-007 pendiente.

- Ampliacion (2026-11-08): 4 metric filters (incluye 429), 8 alarmas (A6 rate limit, A7 Redis, A8 DynamoDB condicionales), cableadas desde I-040/I-050; RequestLogger integrado en HTTPServer con ruta saneada y metricas; /metrics loopback con counters/latencia/runtime; 3 pruebas Go nuevas y trazabilidad W3C probada.
- Pipeline de logs: LOG_PIPELINE_IMPLEMENTED_LOCAL y AWS_VERIFIED NOT_RUN. I-010 tiene endpoint logs e I-060 permisos acotados; falta agente de CloudWatch en I-020.
- D-110-01 (A-090): headers de tracing solo validan longitud, no hex/flags, y X-Request-ID sin limite; registrado sin corregir.
- Coste: +0.5000 USD/mes estimado por 5 alarmas extra (no medido). Notificacion SNS BLOCKED_RFC007_SNS_KMS.

- Correccion final (2026-11-08): dimensiones ALB/TG corregidas (app/... sin loadbalancer/) con dimensiones.tftest.hcl ejecutado por terraform test en CI; consultas Logs Insights pasan a pct(); rutas desconocidas -> /unknown y metricas limitadas a 500 series con contador de descartes; AWS Budgets admite emails directos sin SNS (variable vacia por defecto); ALERT_DELIVERY sigue BLOCKED_RFC007_SNS_KMS.
- Pruebas: contrato Terraform 12, adversariales de ruta (UUID/email/nombre/token/encoded/larga/miles) y cardinalidad; Go build/vet PASS.
