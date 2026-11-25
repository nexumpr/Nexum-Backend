# Propuesta de observabilidad mínima — sin agentes, bajo coste

**Estado:** `PROPUESTA — NO IMPLEMENTADA — AWS_NOT_RUN`.
**Objetivo:** confirmar worker, scheduler, versión desplegada y 5xx con el menor
coste y sin instalar el agente CloudWatch (dependencia de I-020/I-060).

## 1. Situación actual

- `internal/transport/http` ya emite `RequestRecord` estructurado y `/metrics`
  local; `internal/telemetry` redacta claves sensibles.
- `modules/observabilidad` crea log group `/nexum/<entorno>/backend`, retención
  7 días, metric filters y alarmas, pero **no hay pipeline de envío de logs**: el
  agente no está instalado y el endpoint `logs` por sí solo no envía nada.
- `memoria.md` registra `LOG_PIPELINE_IMPLEMENTED_LOCAL` y
  `LOG_PIPELINE_AWS_VERIFIED = NOT_RUN`.

## 2. Diseño propuesto (sin agente)

1. **Exportador directo a CloudWatch Logs desde Go** con AWS SDK v2:
   `CreateLogStream` + `PutLogEvents` por lotes; cola en memoria acotada (256
   eventos); si se llena, descarta y suma `logs_dropped_total`. El envío ocurre
   fuera de la ruta crítica y **nunca** hace fallar una petición.
2. **Log group existente** de `modules/observabilidad`; no se crea otro. IAM
   `logs:CreateLogStream` y `logs:PutLogEvents` sobre el ARN exacto del grupo
   (el módulo IAM ya soporta `cloudwatch_log_group_arns`). Sin
   `logs:CreateLogGroup`, sin `logs:*`, sin `Resource: "*"`.
3. **Campos canónicos** en cada evento (sin PII, sin tokens):
   `event`, `component` (`http|worker|scheduler`), `build_version`,
   `worker_name`, `lease_held`, `processed`, `failed`, `last_success_at`,
   `schedule_lag_seconds`, `status`.
4. **Heartbeat de worker/scheduler** al arrancar y en cada tick:
   `event=worker_heartbeat`, `worker=password_reset|push_outbox|reminders`,
   `lease_held`, `processed`, `last_success_at`, `schedule_lag_seconds`.
   Si el proceso no puede tomar lease, lo declara; no se inventa éxito.
5. **Versión**: `build_version` se inyecta por `-ldflags` o variable de entorno
   y se registra al arrancar y en cada heartbeat. La versión efectiva se consulta
   con Logs Insights; no se crea una métrica por versión.
6. **5xx**: el metric filter existente de `RequestRecord` se conserva y se
   contrasta con la alarma. No se muestrea ni se descarta el 5xx.

## 3. Métricas, alarmas y consultas

| Señal | Mecanismo | Umbral propuesto | Coste incremental |
|---|---|---|---|
| Worker sin heartbeat | metric filter `worker_heartbeat` | < 1 evento/15 min | 0.30 USD/mes (métrica) + 0.10 USD/mes (alarma) |
| Scheduler retrasado | metric filter `schedule_lag_seconds` | > 300 s durante 3 periodos | 0.30 + 0.10 |
| 5xx | filtro existente `status >= 500` | > 0 en 5 min | ya presupuestado |
| Versión desplegada | Logs Insights por `build_version` | informativo / dashboard | 0.005 USD/GB escaneado |

Coste orientativo con 100 MB/mes: ingesta 0.05 USD + almacenamiento 0.003 USD +
3 métricas 0.90 USD + 3 alarmas 0.30 USD ≈ **1.25 USD/mes**. Con 1 GB/mes ≈
1.73 USD/mes. Precios de lista, `DOC_ONLY`; factura real `NOT_VERIFIED`.

## 4. Seguridad y límites

- Sólo se registran campos de la lista blanca; el redactor existente se aplica
  antes de serializar. Nunca tokens, contraseñas, cuerpos financieros ni
  identificadores sensibles completos.
- El log group es privado; no se añaden suscripciones ni exportaciones.
- Retención 7 días y límite de 100 MB/mes. Si se supera, se alerta y se recorta
  el nivel de detalle, no se desactiva la alarma.
- Sin agente, sin SSM, sin Docker, sin abrir puertos ni IPv4 públicas.
- Falla abierto en logging (nunca tumba la API) y falla cerrado en las alarmas
  (ausencia de heartbeat es señal de fallo, no de “sin datos”).

## 5. Dependencias y secuencia

1. Aprobar esta propuesta y el coste (`APRUEBO_OBSERVABILIDAD_MINIMA`).
2. Implementar exportador y heartbeats con pruebas unitarias y de redacción.
3. Terraform: sólo nuevos metric filters/alarmas sobre el log group existente,
   con plan custodiado y Gate.
4. Verificación AWS: `filter-log-events`/Logs Insights de lectura; no se declara
   `VERIFIED` sin eventos reales de worker y scheduler.
5. Mantener `/health/live` y `/health/ready` como están; no sustituyen la
   observabilidad de procesos internos.

## 6. Lo que no se propone

- CloudWatch Agent, Prometheus, Grafana, X-Ray, OpenTelemetry Collector o
  cualquier agente que exija instalación/SSM.
- Métricas por usuario o por transacción (coste y privacidad).
- Retención larga o exportación a S3.
- Alarmas de version mismatch automáticas: la versión se consulta, no se
  inventa una métrica dinámica.
