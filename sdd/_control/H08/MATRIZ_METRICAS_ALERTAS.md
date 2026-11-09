# I-110 — Matriz de métricas, alarmas y runbooks

| Campo | Valor |
|---|---|
| Spec | `sdd/02-infraestructura/I-110-observabilidad-costos` (INF-110-02, INF-110-03) |
| Estado | `IMPLEMENTADO_LOCAL`; alarmas declaradas, **ninguna aplicada** (`AWS_NOT_RUN`) |
| Acciones | **Sin SNS**: notificación `BLOCKED_RFC007_SNS_KMS` por coste de CMK; alarmas visibles en consola/dashboard |

## 1. Métricas

| Métrica | Origen | Uso | Estado |
|---|---|---|---|
| `HealthyHostCount` / `UnHealthyHostCount` | AWS/ApplicationELB | 2 nodos sanos | Alarma A1 + dashboard |
| `Errores5xx` | metric filter `{ $.status >= 500 }` | tasa de error servidor | Alarma A2 |
| `Auth401` | metric filter `{ $.status = 401 }` | picos de autenticación fallida | Alarma A3 |
| `ReadinessNo200` | metric filter `{ $.route = "/api/v1/health/ready" && $.status != 200 }` | readiness degradada | Alarma A4 |
| `IncomingLogEvents` | AWS/Logs | agente/métricas rotas | Alarma A5 |
| `TargetResponseTime` | AWS/ApplicationELB | p50/p95/p99 en dashboard | Dashboard (no alarma) |
| `RequestCount` | AWS/ApplicationELB | RPS | Dashboard |
| `HTTPCode_Target_4XX/5XX` | AWS/ApplicationELB | errores por target | Dashboard |
| `ConsumedReadCapacityUnits`, `ThrottledRequests`, `SuccessfulRequestLatency` | AWS/DynamoDB | capacidad Dynamo | Dependencia: dashboard no cableado |
| `CurrConnections`, `Evictions`, `DatabaseMemoryUsagePercentage` | AWS/ElastiCache | Redis | Dependencia: dashboard no cableado |
| Go runtime (`go_gc_duration_seconds`…) | expuesto por el proceso | GC/runtime | Requiere exporter/OTel; **NOT_RUN** |

**p50/p95/p99**: se obtienen de `latencyMs` en Logs Insights (`stats percentile(latencyMs, 50)`),
no de una métrica nativa. Sin agente de logs, `NOT_RUN`.

## 2. Alarmas declaradas

| ID | Alarma | Métrica / condición | Umbral | Evaluación | Severidad | Runbook |
|---|---|---|---|---|---|---|
| A1 | `targets-sanos-bajos` | `HealthyHostCount` (Minimum) | < 2 | 60 s × 2, missing=breaching | Crítica | §3.1 |
| A2 | `errores-5xx` | `Errores5xx` (Sum) | ≥ 5 / 5 min | 300 s × 1 | Alta | §3.2 |
| A3 | `auth-401` | `Auth401` (Sum) | ≥ 20 / 5 min | 300 s × 1 | Alta | §3.3 |
| A4 | `readiness-no-200` | `ReadinessNo200` (Sum) | ≥ 1 / 5 min | 300 s × 1 | Alta | §3.4 |
| A5 | `logs-sin-eventos` | `IncomingLogEvents` (Sum) | < 1 / hora | 3600 s × 1, missing=breaching | Media | §3.5 |

El presupuesto (`aws_budgets_budget`, 1 USD) queda declarado **sin notificación** hasta resolver
`RFC-007` (SNS con CMK frente a coste no presupuestado). **No es un límite de gasto** y no detiene
recursos. Las alarmas existen y se ven en consola/dashboard, pero no envían notificación automática.

## 2.1 Bloqueo de notificación (RFC-007)

`aws_sns_topic` con `alias/aws/sns` dispara Trivy AWS-0136 (HIGH: exige CMK de cliente); una CMK tiene
coste mensual no presupuestado y no se crea unilateralmente. Se retiraron SNS, `alarm_actions` y las
notificaciones del presupuesto. `ALERT_DELIVERY = BLOCKED_RFC007_SNS_KMS`; las alarmas siguen visibles
y con runbook. No se declara entrega de alertas verificada.

## 3. Runbooks

### 3.1 A1 — menos de 2 targets sanos
1. `aws elbv2 describe-target-health` por target group.
2. Revisar health check `/api/v1/health/live`, estado del ASG y eventos de scaling.
3. No reducir el mínimo de 2; escalar o revertir el release (I-090).
4. Registrar evidencia y causa.

### 3.2 A2 — 5xx
1. Correlacionar `traceId` en Logs Insights.
2. Distinguir error de aplicación, dependencia (Dynamo/Redis) o despliegue.
3. Rollback si el release es la causa.
4. Registrar alcance e impacto.

### 3.3 A3 — 401 anómalos
1. Verificar origen (IP/ASN) y patrón temporal.
2. Confirmar rate limiting (D-100-01: límite local y XFF sin proxy validado).
3. No bloquear cuentas legítimas sin evidencia.

### 3.4 A4 — readiness ≠ 200
1. Consultar `/api/v1/health/ready` y dependencias registradas.
2. Si no hay dependencias reales, es el 503 por diseño: no declarar producción lista.
3. Escalar según I-040/I-050.

### 3.5 A5 — sin eventos de log
1. Confirmar agente de CloudWatch en instancias (dependencia I-020).
2. Revisar permisos de escritura a CloudWatch Logs en el rol de instancia.
3. No desactivar la alarma: la ausencia de logs es un fallo de observabilidad.

## 4. Límites

- Alarmas y logs no aplicados: `AWS_NOT_RUN`.
- El dashboard no cablea DynamoDB/Redis hasta recibir sus identificadores; se
  documentan como dependencia, no como verificados.
- Go runtime/OTel no está exportado; `NOT_RUN`.
- Las alarmas no limitan gasto; el presupuesto es informativo.
