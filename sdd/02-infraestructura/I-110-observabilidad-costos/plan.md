# I-110-observabilidad-costos — CloudWatch, métricas, tracing y monitoreo FinOps

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-090, I-000, I-020, I-030 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## 1. Estrategia de diseño documental de H05

Definir observabilidad operacional en AWS con correlación, alertas, retención, métricas y control de costos, manteniendo PII y secretos fuera de registros.

## Autoridad y restricciones

- `constitution.md` (GOV-01..18, R-001..060) y el Gate H04 aprobado explícitamente el **2026-11-05** tienen precedencia; H05 diseña infraestructura, no la ejecuta.
- Arquitectura ratificada: Go (Clean Architecture), AWS, DynamoDB, **Redis real**, **2 instancias simultáneas**, balanceador HTTPS y CI/CD/DevSecOps. Ninguno es demostración opcional.
- El PDF del docente v3 exige Backend cloud con costo cero; el catálogo V6.0.2 enumera 75 rutas propuestas E001..E075 y utiliza stack histórico GCP/MongoDB **descartado**.
- No crear código Go, archivos Terraform `.tf`, pipelines ni endpoints H06 durante H05; `terraform apply/destroy`, `git push`, cambios AWS y facturación están prohibidos hasta fases y aprobaciones autorizadas.
- **Regla de evidencia:** documentos pueden alcanzar DRAFT completo sin acceso cloud; Gate I-GO requiere demostrar **viabilidad operacional y económica**, no asumirla. Campos pendientes no se cambian a PASS ni se solventan con créditos.

## 2. Plan secuencial y verificaciones

### P-110-01 — Logs

**Entrada:** H04 aprobado, requisitos `A-090, I-000, I-020, I-030`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** JSON fields timestamp, severity, service, environment, traceId, requestId, route template, status, latencyMs, instanceId; redacción automática de password, tokens, raw voice, emails, montos no necesarios.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-110-02 — Métricas

**Entrada:** H04 aprobado, requisitos `A-090, I-000, I-020, I-030`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** latencia p50/p95/p99, RPS, 4xx/5xx, target healthy/unhealthy, Dynamo consumed capacity/throttle/latency, Redis latency/evictions/memory, Go GC/runtime; OpenTelemetry a sistema aprobado.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-110-03 — Alertas

**Entrada:** H04 aprobado, requisitos `A-090, I-000, I-020, I-030`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** ALB target count <2, 5xx, API ready fail, caché indisponible, errores de autenticación anómalos, budget/Free Tier 85/100%, métricas rotas. Severidad + runbook.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-110-04 — Retención

**Entrada:** H04 aprobado, requisitos `A-090, I-000, I-020, I-030`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** CloudWatch Logs con días de retención finitos (p.ej. 7/14 como propuesta), no infinito; revisar precio ingest/storage/extracción/alarms y evitar trace 100% por defecto.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-110-05 — Trazabilidad

**Entrada:** H04 aprobado, requisitos `A-090, I-000, I-020, I-030`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** TraceId propagado por header confiable, IDs anónimos; no registrar claims JWT completos, token push, secretos, cuentas ajenas.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-110-06 — FinOps

**Entrada:** H04 aprobado, requisitos `A-090, I-000, I-020, I-030`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Comparar gasto/usage y balance créditos diariamente durante ventanas, generar reporte antes/después de apagado; alarmas no equivalen a límite de gasto garantizado.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

## 3. Orden de ejecución futura H08 (no ejecutar ahora)

1. Auditar plan AWS/credenciales/precios y límites SCP sin cambios.
2. Preparar revisión de cambios, artefactos IaC y análisis económico en PR autorizado.
3. Ejecutar validaciones (fmt/validate/plan/pruebas), someter al gate humano.
4. Aprovisionar o modificar únicamente después de autorización de release y protección de datos.
5. Ejecutar pruebas reales, registrar métricas, costos y rollback, y no declarar DONE antes de las evidencias.

## 4. Dependencias cruzadas

I-000, I-020, I-030, I-050, I-120, I-140

## 5. Riesgos y tratamiento

1. `I-110-observabilidad-costos-R01`: Altos costos de logs/trazas. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.
2. `I-110-observabilidad-costos-R02`: PII inadvertida. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.
3. `I-110-observabilidad-costos-R03`: Alarmas insuficientes para evitar cargos. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.

## 6. Puerta de entrada a H06

Gate H05/I-GO NO se logra con instalación de Markdown: requiere pruebas de viabilidad económica/cuenta, trazabilidad física Dynamo/Redis, controles IAM y aceptación expresa del responsable. Los contratos detallados E001..E075 se formalizan exclusivamente en H06.
