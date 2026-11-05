# I-110-observabilidad-costos — CloudWatch, métricas, tracing y monitoreo FinOps

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-090, I-000, I-020, I-030 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## 1. Objetivo y definición de terminado

Definir observabilidad operacional en AWS con correlación, alertas, retención, métricas y control de costos, manteniendo PII y secretos fuera de registros.

## Autoridad y restricciones

- `constitution.md` (GOV-01..18, R-001..060) y el Gate H04 aprobado explícitamente el **2026-11-05** tienen precedencia; H05 diseña infraestructura, no la ejecuta.
- Arquitectura ratificada: Go (Clean Architecture), AWS, DynamoDB, **Redis real**, **2 instancias simultáneas**, balanceador HTTPS y CI/CD/DevSecOps. Ninguno es demostración opcional.
- El PDF del docente v3 exige Backend cloud con costo cero; el catálogo V6.0.2 enumera 75 rutas propuestas E001..E075 y utiliza stack histórico GCP/MongoDB **descartado**.
- No crear código Go, archivos Terraform `.tf`, pipelines ni endpoints H06 durante H05; `terraform apply/destroy`, `git push`, cambios AWS y facturación están prohibidos hasta fases y aprobaciones autorizadas.
- **Regla de evidencia:** documentos pueden alcanzar DRAFT completo sin acceso cloud; Gate I-GO requiere demostrar **viabilidad operacional y económica**, no asumirla. Campos pendientes no se cambian a PASS ni se solventan con créditos.

## 2. Contrato de infraestructura propuesto

### INF-110-01 — Logs

JSON fields timestamp, severity, service, environment, traceId, requestId, route template, status, latencyMs, instanceId; redacción automática de password, tokens, raw voice, emails, montos no necesarios.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-110-02 — Métricas

latencia p50/p95/p99, RPS, 4xx/5xx, target healthy/unhealthy, Dynamo consumed capacity/throttle/latency, Redis latency/evictions/memory, Go GC/runtime; OpenTelemetry a sistema aprobado.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-110-03 — Alertas

ALB target count <2, 5xx, API ready fail, caché indisponible, errores de autenticación anómalos, budget/Free Tier 85/100%, métricas rotas. Severidad + runbook.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-110-04 — Retención

CloudWatch Logs con días de retención finitos (p.ej. 7/14 como propuesta), no infinito; revisar precio ingest/storage/extracción/alarms y evitar trace 100% por defecto.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-110-05 — Trazabilidad

TraceId propagado por header confiable, IDs anónimos; no registrar claims JWT completos, token push, secretos, cuentas ajenas.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-110-06 — FinOps

Comparar gasto/usage y balance créditos diariamente durante ventanas, generar reporte antes/después de apagado; alarmas no equivalen a límite de gasto garantizado.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

## 3. Dependencias y secuencia

I-000, I-020, I-030, I-050, I-120, I-140

## 4. Criterios de aceptación de diseño (Gate H05)

- Tablero y consultas de salud para dos nodos.
- Alarmas de caída y presupuesto diseñadas.
- PII nunca presente en logs en fixtures de prueba.
- Retención y precio CloudWatch explicitados.
- Runbooks de incidentes relacionados a metricas y cada alarma.

## 5. Riesgos que impiden una aprobación sin evidencia

- **Riesgo:** Altos costos de logs/trazas. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.
- **Riesgo:** PII inadvertida. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.
- **Riesgo:** Alarmas insuficientes para evitar cargos. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.

## 6. Entregables y referencias

Los siguientes nombres identifican evidencias/artefactos **futuros** de I-GO y de implementación; no son archivos creados en H05: `MATRIZ_METRICAS_ALERTAS.md`, `POLITICA_LOGS.md`, `DASHBOARD_OBSERVABILIDAD.md`.

## 7. Criterio de bloqueo y aprobación

**BLOCKED** si falta una obligación técnica, contradice alguna spec A aprobada, no puede comprobarse la viabilidad Free/SCP o no hay aceptación humana del riesgo. **DRAFT** significa que el diseño está documentado; nunca equivale a infraestructura aplicada.

## 8. Cierre tecnico-financiero H05 (propuesta v2; aun no aprobada)

- Tarifa de CloudWatch se contabiliza por logs, retención, metricas personalizadas y alarmas, con 3 GB ingesta, 3 GB-mes aproximados, 3 metricas y 2 alarmas como supuestos de uso, no como límites garantizados. Logs estructurados sin datos financieros ni secretos, 7 días de retención (validar cada log group), sampling de trazas / costo antes de habilitar X-Ray y ALB access logs; la ausencia de cargo de un concepto solo puede declararse si NO se habilita.
- Revisar consumo **diario** vs 45d; si A $103.41 sin impuestos ya rompe reserva $30, alertar antes de $90; B $84.11 sin 24/7 da reserva teórica $35.89. Alertas no son stop ni garantizan tiempo real. Separar precios brutos de neto y de IVA/obligaciones fiscales.
