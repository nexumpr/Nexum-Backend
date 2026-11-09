# I-110 — Política de logs

| Campo | Valor |
|---|---|
| Spec | `sdd/02-infraestructura/I-110-observabilidad-costos` (INF-110-01, INF-110-04) |
| Estado | `IMPLEMENTADO_LOCAL`; ingestión real `NOT_RUN` (requiere agente y AWS) |
| Grupo de logs | `/nexum/<entorno>/backend` |
| Retención | 7 días por defecto (máximo 30 sin revisión FinOps) |

## 1. Campos canónicos

| Campo | Tipo | Obligatorio | Notas |
|---|---|---|---|
| `timestamp` | RFC3339 UTC | Sí | Sin hora local |
| `severity` | `DEBUG/INFO/WARN/ERROR` | Sí | No usar niveles inventados |
| `service` | string | Sí | `nexum-backend` |
| `environment` | string | Sí | `h08i`, `prod`… |
| `traceId` | hex 32 | Sí | W3C `traceparent`; nunca claims |
| `requestId` | string | Sí | Correlación por petición |
| `route` | template | Sí | `/api/v1/movimientos/{id}`, no la URL con IDs |
| `status` | int | Sí | 100–599 |
| `latencyMs` | int | Sí | ≥ 0 |
| `instanceId` | string | Sí | Para demostrar 2 nodos |
| `error` | string | No | Redactado |
| `attributes` | mapa | No | Claves sensibles prohibidas |

El registro tipado vive en `internal/infrastructure/telemetry/request_record.go`
(`RequestRecord.Validate`/`Sanitized`).

## 2. Redacción y datos prohibidos

Nunca se registran: contraseñas, hashes, tokens, JWT completos, refresh tokens,
cabeceras `Authorization`/`Cookie`, correos completos, importes/saldos innecesarios,
transcripciones de voz, datos de tarjeta, IBAN, claims de identidad ni cuerpos
financieros completos.

- Claves sensibles se **rechazan** en `Validate` y se enmascaran en `Sanitized`.
- Valores con JWT, Bearer, tarjetas o emails se redactan con `RedactSensitiveString`.
- El logger JSON de A-090 aplica además el filtro de atributos sensibles.

## 3. Retención y coste

- `retention_in_days = 7` por defecto; valores admitidos por CloudWatch Logs.
- `terraform_data.precondiciones` bloquea retención > 30 días sin revisión.
- I-000 presupuesta 3 GB de ingesta y 4.5 GB-mes de retención (~1.5000 + 0.1350 USD).
  El consumo real es `NOT_VERIFIED`; ingestión superior aumenta coste.
- **No** se habilita X-Ray, ALB access logs ni tracing 100 %: se evita coste no
  presupuestado. `AWS_NOT_RUN`.

## 4. Dependencia de agente

El grupo de logs y los metric filters existen, pero **las instancias no envían logs**
hasta que I-020 configure el agente de CloudWatch (o un colector equivalente) en la
AMI/user-data. Esta pieza no se modifica en I-110; queda registrada como dependencia
bloqueante para la observabilidad real. Sin agente, la alarma A5 (sin eventos) se
disparará: es el comportamiento esperado de una configuración incompleta, no un PASS.

## 5. Correlación

`traceId` y `requestId` provienen de A-090 (W3C `traceparent`/`x-request-id`). La
consulta de Logs Insights del dashboard filtra por esos campos. No se registran
claims JWT ni identificadores de terceros.
