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
| `RateLimit429` | metric filter `{ $.status = 429 }` | rate limiting / abuso | Alarma A6 |
| `IncomingLogEvents` | AWS/Logs | agente/métricas rotas | Alarma A5 |
| `TargetResponseTime` | AWS/ApplicationELB | p50/p95/p99 en dashboard | Dashboard (no alarma) |
| `RequestCount` | AWS/ApplicationELB | RPS | Dashboard |
| `HTTPCode_Target_4XX/5XX` | AWS/ApplicationELB | errores por target | Dashboard |
| `ConsumedReadCapacityUnits`, `ThrottledRequests`, `SuccessfulRequestLatency` | AWS/DynamoDB | capacidad Dynamo | Alarma A8 (throttles) + dashboard |
| `CurrConnections`, `Evictions`, `DatabaseMemoryUsagePercentage` | AWS/ElastiCache | Redis | Alarma A7 (conexiones) + dashboard |
| `nexum_http_requests_total`, `nexum_http_request_duration_seconds`, `nexum_go_*` | `/metrics` local (proceso Go) | RPS/latencia/runtime | Exportador local; agente/OTel **NOT_RUN** |

**p50/p95/p99**: se obtienen de `latencyMs` en Logs Insights (`stats pct(latencyMs, 50)`),
no de una métrica nativa. Sin agente de logs, `NOT_RUN`.

## 2. Alarmas declaradas

| ID | Alarma | Métrica / condición | Umbral | Evaluación | Severidad | Runbook |
|---|---|---|---|---|---|---|
| A1 | `targets-sanos-bajos` | `HealthyHostCount` (Minimum) | < 2 | 60 s × 2, missing=breaching | Crítica | §3.1 |
| A2 | `errores-5xx` | `Errores5xx` (Sum) | ≥ 5 / 5 min | 300 s × 1 | Alta | §3.2 |
| A3 | `auth-401` | `Auth401` (Sum) | ≥ 20 / 5 min | 300 s × 1 | Alta | §3.3 |
| A4 | `readiness-no-200` | `ReadinessNo200` (Sum) | ≥ 1 / 5 min | 300 s × 1 | Alta | §3.4 |
| A5 | `logs-sin-eventos` | `IncomingLogEvents` (Sum) | < 1 / hora | 3600 s × 1, missing=breaching | Media | §3.5 |
| A6 | `rate-limit-429` | `RateLimit429` (Sum) | ≥ 50 / 5 min | 300 s × 1 | Alta | §3.6 |
| A7 | `redis-sin-conexiones` | `CurrConnections` (Maximum, `ReplicationGroupId`) | < 1 | 300 s × 2, missing=breaching | Alta | §3.7 |
| A8 | `dynamodb-throttles` | `ThrottledRequests` (Sum, `TableName`) | ≥ 5 / 5 min | 300 s × 2 | Alta | §3.8 |

A7/A8 sólo existen si se aportan `redis_replication_group_id`/`dynamodb_table_name`; en `envs/h08i`
se cablean desde I-050/I-040. Son 8 alarmas declaradas (2 condicionales).

El presupuesto (`aws_budgets_budget`, 1 USD) admite **notificación directa por email sin SNS**
(`subscriber_email_addresses`) mediante la variable externa `emails_presupuesto`, vacía por defecto y sin
direcciones reales en Git. Si no hay destinatario configurado, no se declara notificación funcional.
**No es un límite de gasto** y no detiene recursos.

## 2.1 Bloqueo de notificación de alarmas (RFC-007)

`aws_sns_topic` con `alias/aws/sns` dispara Trivy AWS-0136 (HIGH: exige CMK de cliente); una CMK tiene
coste mensual no presupuestado y no se crea unilateralmente. Se retiraron SNS y `alarm_actions`.
`ALERT_DELIVERY = BLOCKED_RFC007_SNS_KMS`. Las notificaciones de AWS Budgets por email son una capacidad
distinta y no dependen de SNS; su estado es `IMPLEMENTADO_LOCAL` sin destinatario verificado. Las alarmas
siguen visibles y con runbook.

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

### 3.6 A6 — rate limiting 429
1. Confirmar origen y patrón (IP, ruta, ventana).
2. Verificar que no es un cliente legítimo con reintentos agresivos.
3. Revisar límites locales y coordinación (D-100-01/D-100-02) antes de ajustar umbrales.
4. No desactivar el rate limiting para "bajar" la alarma.

### 3.7 A7 — Redis sin conexiones
1. Revisar `describe-cache-clusters`/`describe-replication-groups` y estado del nodo.
2. Comprobar SG/TLS/credenciales RBAC (I-050) y readiness del backend.
3. Si el nodo está caído, escalar; no declarar caché disponible sin conexión real.

### 3.8 A8 — DynamoDB throttling
1. Revisar `ConsumedRead/WriteCapacityUnits` y patrones de acceso.
2. Confirmar que no hay `Scan` ni consultas no acotadas.
3. Evaluar capacidad on-demand y claves calientes; no cambiar la tabla sin Gate.

## 4. Límites

- Alarmas y logs no aplicados: `AWS_NOT_RUN`.
- 8 alarmas declaradas (6 base + 2 condicionales); I-000 presupuesta 3 → **+0.5000 USD/mes**
  estimados, no medidos. Notificación automática `BLOCKED_RFC007_SNS_KMS`.
- El dashboard no cablea DynamoDB/Redis widgets aunque sus alarmas existan; se documentan como
  dependencia, no como verificados.
- Go runtime/OTel no está exportado a CloudWatch; `/metrics` local (loopback) está implementado y
  probado, pero el agente/OTel de recolección es `NOT_RUN`.
- Las alarmas no limitan gasto; el presupuesto es informativo y su notificación por email requiere
  destinatario autorizado.
- **Rutas y cardinalidad**: las rutas no registradas se registran como `/unknown`; `/metrics` limita a 500
  series y descarta el resto (`nexum_metrics_series_dropped_total`). No se usan IDs, correos ni tokens
  como etiquetas.
