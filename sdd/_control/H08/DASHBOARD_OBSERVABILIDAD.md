# I-110 — Dashboard de observabilidad

| Campo | Valor |
|---|---|
| Spec | `sdd/02-infraestructura/I-110-observabilidad-costos` (INF-110-02, INF-110-05) |
| Estado | `IMPLEMENTADO_LOCAL`; dashboard declarado, **no creado** (`AWS_NOT_RUN`) |
| Nombre | `nexum-<entorno>-observabilidad` |
| Coste | CloudWatch permite 3 dashboards gratuitos; se crea 1 |

## 1. Widgets

| Widget | Contenido | Fuente |
|---|---|---|
| Salud de targets | `HealthyHostCount` y `UnHealthyHostCount`, línea de referencia en 2 | AWS/ApplicationELB (ALB y TG de I-030) |
| Errores y autenticación | `Errores5xx`, `Auth401`, `ReadinessNo200` (Sum) | metric filters de I-110 |
| Logs backend | Últimos 50 eventos con `timestamp, severity, route, status, latencyMs, traceId` | Logs Insights sobre `/nexum/<entorno>/backend` |

## 2. Consultas de salud para dos nodos

```sql
-- Conteo de instancias que emiten logs en la última hora
SOURCE '/nexum/h08i/backend'
| fields instanceId
| stats count_distinct(instanceId) as nodos by bin(1h)

-- Latencia p50/p95/p99 a partir de los logs (no es métrica nativa)
SOURCE '/nexum/h08i/backend'
| fields latencyMs, route
| stats percentile(latencyMs, 50) as p50, percentile(latencyMs, 95) as p95, percentile(latencyMs, 99) as p99 by route

-- Errores por ruta y traza
SOURCE '/nexum/h08i/backend'
| filter status >= 500
| stats count() as errores by route, traceId
| sort errores desc
```

Estas consultas requieren que las instancias envíen logs (agente de CloudWatch,
dependencia I-020). Sin agente, `NOT_RUN`.

## 3. Correlación

- `traceId` y `requestId` vienen de A-090; el dashboard filtra por ambos.
- No se muestran claims JWT, tokens, correos completos ni importes.
- La consulta usa `route` (template), no la URL con identificadores reales.

## 4. Lo que el dashboard no demuestra

- No prueba que las dos instancias AWS existan: eso exige `AWS_NOT_RUN → verificación real`.
- No sustituye las alarmas ni los runbooks.
- No incluye métricas DynamoDB/Redis hasta cablear sus identificadores (I-040/I-050),
  ni runtime Go hasta exportar OTel.
- Un dashboard sin datos no es `PASS`: es una configuración incompleta.
