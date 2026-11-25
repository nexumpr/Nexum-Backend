# Observabilidad FCM y scheduler — diseño I-110

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/OBSERVABILIDAD_FCM_I110.md` |
| Versión | 1.0.0 |
| Fecha | 2026-11-24 |
| Módulo | `infra/terraform/modules/observabilidad` |
| Namespace | `Nexum/Observabilidad` |
| Estado | **`DISENO_LOCAL · METRICAS_Y_ALARMAS_DESHABILITADAS · GO_NOT_IMPLEMENTED · AWS_NOT_RUN`** |
| Coste | **3.00 USD/mes `DOC_ONLY`** si se habilitan todos los filtros y alarmas |
| Dependencia | Pipeline de logs a CloudWatch (`LOG_PIPELINE_AWS_VERIFIED = NOT_RUN`) |

> Los filtros y alarmas **existen en Terraform con `count = 0`**. No se crea
> ninguna métrica ni alarma mientras `habilitar_metricas_fcm` y
> `habilitar_alarmas_fcm` estén en `false`. El proceso Go que debe emitir los
> eventos **no existe** todavía (Oleada 4/5). Por tanto hoy no hay ninguna señal
> FCM observable: este documento define el contrato, no un resultado.

## 1. Situación actual y límite

- El log group `/nexum/<entorno>/backend` existe (I-110), retención 7 días
  (máximo 30) y ya tiene filtros de `5xx`, `401`, `readiness` y `429`.
- **No hay agente CloudWatch** ni exportador de logs desde Go. El interface
  endpoint `logs` no envía nada por sí solo: sin emisor, los eventos no llegan a
  CloudWatch y las alarmas no tendrían datos.
- `PROPUESTA_OBSERVABILIDAD_MINIMA.md` propone un exportador directo desde Go
  (CreateLogStream + PutLogEvents, cola acotada, sin bloquear la petición). Ese
  exportador **no está implementado**.
- Mientras no exista pipeline + eventos, el estado correcto de cualquier
  afirmación de observabilidad FCM es **`NOT_RUN`**, nunca `OK`.

## 2. Esquema de eventos (contrato)

Todos los eventos son **una línea JSON** en el log del backend. Sin PII, sin
tokens, sin cuerpos de push, sin credenciales.

### 2.1 `startup`

```json
{"event":"startup","component":"fcm","build_version":"vX.Y.Z+<sha12>","instance_id":"i-...","az":"us-east-2a","fcm_project_present":true,"credentials_source":"file","push_key_id":"k1","cursor_key_id":"k1","workers":["fcm_outbox","scheduler"],"worker_started":true,"scheduler_started":true}
```

| Campo | Tipo | Obligatorio | Regla |
|---|---|---|---|
| `event` | string | Sí | Literal `startup` |
| `component` | string | Sí | `fcm` |
| `build_version` | string | Sí | `vX.Y.Z+<sha12>`; debe coincidir con el release desplegado |
| `instance_id` | string | Sí | ID de EC2 obtenido por IMDSv2 (el módulo de cómputo exige IMDSv2) |
| `az` | string | Recomendado | Zona de la instancia |
| `fcm_project_present` | bool | Sí | No se registra el project id completo si se considera sensible |
| `credentials_source` | string | Sí | `file` o `inline` (nunca el contenido) |
| `push_key_id`, `cursor_key_id` | string | Sí | `k1`, `k2`; **nunca** la clave |
| `workers` | array | Sí | Workers arrancados |
| `worker_started`, `scheduler_started` | bool | Sí | Falla cerrado: si no arranca, no se declara sano |

### 2.2 `worker_heartbeat`

```json
{"event":"worker_heartbeat","worker":"fcm_outbox","instance_id":"i-...","lease_held":true,"processed":12,"failed":0,"queue_lag_seconds":3,"last_success_at":"2026-11-24T18:00:00Z"}
```

- `worker`: `fcm_outbox` (variable `worker_fcm`) o `scheduler`
  (`worker_scheduler`). Los filtros usan exactamente esos literales; cambiarlos
  exige cambiar Terraform y este documento.
- **Todos los procesos emiten heartbeat**, tengan o no el lease. `lease_held`
  distingue líder de seguidor. Así la ausencia de heartbeat significa «ningún
  proceso vivo», no «este nodo no es líder».
- `processed`, `failed`: contadores del intervalo, enteros no negativos.
- `queue_lag_seconds`: antigüedad del trabajo más viejo pendiente.
- Sin token de dispositivo, sin `userId`, sin correo.

### 2.3 `fcm_send`

```json
{"event":"fcm_send","status":"error","error_code":"UNREGISTERED","attempt":2,"message_id_present":false,"device_key_id":"k2"}
```

- `status`: `ok` | `error`.
- `error_code`: código de Google normalizado (`UNREGISTERED`, `INVALID_ARGUMENT`,
  `UNAVAILABLE`, `QUOTA_EXCEEDED`, …), nunca el cuerpo completo.
- `message_id_present`: bool. El `message_id` completo no se registra.
- `device_key_id`: identificador de clave usado para localizar el dispositivo,
  no el token.

### 2.4 `aws_error`

```json
{"event":"aws_error","service":"dynamodb","operation":"UpdateItem","error_code":"AccessDenied","request_id":"..."}
```

- Se emite para errores de AWS que no son de negocio. `AccessDenied` es la señal
  de IAM mal configurado (por ejemplo, `OUTBOX#*` o `SCHEDULE#*` ausentes).
- `request_id` de AWS es aceptable (no es dato personal).

### 2.5 `scheduler_tick`

```json
{"event":"scheduler_tick","instance_id":"i-...","schedule_lag_seconds":42,"due_count":3,"emitted_count":3,"failed_count":0}
```

- `schedule_lag_seconds`: diferencia entre `now` y el `dueAt` más antiguo que se
  está procesando. Es la métrica que alimenta la alarma de lag.
- No se registra el título del recordatorio ni el `userId`.

### 2.6 5xx

No se crea un evento nuevo: el filtro existente `{ $.status >= 500 }` de
`RequestRecord` sigue siendo la señal HTTP. La observabilidad FCM **no** lo
sustituye ni lo muestrea.

## 3. Filtros y métricas (Terraform)

| # | Recurso Terraform | Patrón | Métrica | Condición de creación |
|---:|---|---|---|---|
| 1 | `arranques_proceso` | `{ $.event = "startup" }` | `ArranquesProceso` | `habilitar_metricas_fcm` |
| 2 | `arranque_version_esperada` | `{ $.event = "startup" && $.build_version = "<version_esperada>" }` | `ArranqueVersionEsperada` | métricas + `version_esperada != ""` |
| 3 | `arranque_version_inesperada` | `{ $.event = "startup" && $.build_version != "<version_esperada>" }` | `ArranqueVersionInesperada` | métricas + `version_esperada != ""` |
| 4 | `fcm_worker_heartbeat` | `{ $.event = "worker_heartbeat" && $.worker = "fcm_outbox" }` | `FcmWorkerHeartbeat` | `habilitar_metricas_fcm` |
| 5 | `scheduler_heartbeat` | `{ $.event = "worker_heartbeat" && $.worker = "scheduler" }` | `SchedulerHeartbeat` | métricas |
| 6 | `fcm_errores_envio` | `{ $.event = "fcm_send" && $.status = "error" }` | `FcmErroresEnvio` | métricas |
| 7 | `access_denied` | `{ $.event = "aws_error" && $.error_code = "AccessDenied" }` | `AccessDenied` | métricas |
| 8 | `scheduler_lag` | `{ $.event = "scheduler_tick" }` | `SchedulerLagSegundos` (valor `$.schedule_lag_seconds`) | métricas |

Total máximo: **8 filtros** (6 base + 2 de versión). Namespace
`Nexum/Observabilidad`.

> **Discrepancia registrada `D-OBS-FCM-01`:** el texto del output
> `observabilidad_fcm.coste_estimado_mensual_usd` dice «8 filtros y 6 alarmas»,
> pero el módulo declara **8** filtros cuando `version_esperada` no está vacío
> (los 6 base más los 2 de versión). El coste de la sección 5 se calcula con 8.
> No se edita el `.tf` en esta unidad (propiedad de otra unidad de trabajo); se
> deja constancia para que el propietario del módulo lo corrija o justifique.

## 4. Alarmas (todas condicionales)

| # | Alarma | Métrica | Estadístico | Periodo | Evaluaciones | Umbral | Missing data |
|---:|---|---|---|---:|---:|---|---|
| A-FCM-1 | `fcm-worker-sin-latido` | `FcmWorkerHeartbeat` | Sum | `minutos_sin_heartbeat` × 60 (900 s) | 1 | `< 1` | `breaching` |
| A-FCM-2 | `scheduler-sin-latido` | `SchedulerHeartbeat` | Sum | 900 s | 1 | `< 1` | `breaching` |
| A-FCM-3 | `fcm-errores-envio` | `FcmErroresEnvio` | Sum | 300 s | 1 | `>= 5` (`umbral_fcm_errores`) | `notBreaching` |
| A-FCM-4 | `access-denied` | `AccessDenied` | Sum | 300 s | 1 | `>= 1` (`umbral_access_denied`) | `notBreaching` |
| A-FCM-5 | `scheduler-lag` | `SchedulerLagSegundos` | Maximum | 300 s | 3 | `> 300` s (`umbral_lag_scheduler_segundos`) | `notBreaching` |
| A-FCM-6 | `arranque-version-inesperada` | `ArranqueVersionInesperada` | Sum | 300 s | 1 | `>= 1` | `notBreaching` |

Notas de interpretación:

- **A-FCM-1/2 usan `breaching` en ausencia de datos**: si no hay eventos, la
  alarma se dispara. Es deliberado: la ausencia de heartbeat es fallo, no
  «sin datos». Exige que el pipeline de logs funcione; si CloudWatch no recibe
  logs por un problema del exportador, la alarma también se dispara (falso
  positivo) — preferible a silencio.
- **A-FCM-5** sólo se dispara si hay `scheduler_tick` con lag > 300 s durante
  15 min; si el scheduler no emite nada, dispara A-FCM-2.
- **A-FCM-6** exige `version_esperada` no vacío. Es la única forma de detectar
  un nodo que quedó en una versión anterior tras un refresh parcial.
- **No hay acciones de notificación**: `ALERT_DELIVERY = BLOCKED_RFC007_SNS_KMS`
  (SNS con CMK no presupuestada). Las alarmas quedan en estado visible en
  CloudWatch, sin email. No se declara entrega de alertas.

### 4.1 Variables del módulo

| Variable | Default | Efecto |
|---|---|---|
| `habilitar_metricas_fcm` | `false` | Crea los filtros |
| `habilitar_alarmas_fcm` | `false` | Crea las alarmas; exige métricas (precondition) |
| `version_esperada` | `""` | Forma `vX.Y.Z+<sha12>`; vacío = no crea filtros/alarma de versión |
| `worker_fcm` | `"fcm_outbox"` | Literal del campo `worker` |
| `worker_scheduler` | `"scheduler"` | Literal del campo `worker` |
| `umbral_fcm_errores` | `5` | Errores por 5 min |
| `umbral_access_denied` | `1` | AccessDenied por 5 min |
| `umbral_lag_scheduler_segundos` | `300` | Lag máximo |
| `minutos_sin_heartbeat` | `15` | 5, 10, 15, 30 o 60 |

## 5. Coste estimado (`DOC_ONLY`)

| Concepto | Cantidad | Precio de lista | Subtotal |
|---|---:|---:|---:|
| Métricas personalizadas | 8 | 0.30 USD/métrica-mes | 2.40 |
| Alarmas estándar | 6 | 0.10 USD/alarma-mes | 0.60 |
| Consultas Logs Insights / API | — | 0.005 USD/GB escaneado | ≈ 0.10 |
| **Total** | | | **3.00 USD/mes** |

- **`DOC_ONLY`**: precios de lista, no factura. `AWS_COST_REAL = NOT_VERIFIED`.
- Sin `habilitar_metricas_fcm`, el coste es **0**.
- No incluye ingesta/almacenamiento de logs (ya presupuestado en I-110; el
  volumen real no se mide).
- No incluye SNS (bloqueado) ni dashboards nuevos.
- El saldo de créditos **no** autoriza habilitarlo: requiere Gate de coste.

## 6. Cómo confirma el próximo despliegue qué proceso corre en cada nodo

Este es el objetivo operativo concreto: saber, sin adivinar, **qué versión y qué
procesos** ejecuta cada EC2.

### 6.1 Mecanismo

1. Cada proceso emite **un** `startup` al arrancar, con `instance_id`, `az`,
   `build_version`, `workers`, `worker_started`, `scheduler_started`,
   `push_key_id`/`cursor_key_id` (ids, no valores).
2. Cada worker emite `worker_heartbeat` periódico con `lease_held`.
3. Los eventos llegan al log group (requiere exportador; hoy `NOT_RUN`).

### 6.2 Consultas de verificación (Logs Insights, sólo lectura)

Arranques por nodo:

```text
fields @timestamp, instance_id, az, build_version, workers, worker_started, scheduler_started
| filter event = "startup"
| sort @timestamp desc
| limit 20
```

Líder por worker en la última hora:

```text
fields @timestamp, instance_id, worker, lease_held, processed, failed
| filter event = "worker_heartbeat"
| stats latest(lease_held) as lider, sum(processed) as procesados, sum(failed) as fallos by instance_id, worker
```

Lag del scheduler:

```text
fields @timestamp, instance_id, schedule_lag_seconds, due_count, emitted_count
| filter event = "scheduler_tick"
| stats max(schedule_lag_seconds) as lag_max by instance_id
```

### 6.3 Criterios de aceptación de un despliegue

| Criterio | Esperado | Si falla |
|---|---|---|
| Arranques | **2** eventos `startup` con `instance_id` distintos | Nodo que no arrancó o no exporta logs |
| Versión | Ambos con el **mismo** `build_version` = release desplegado | Refresh parcial o LT mixta |
| Workers | `worker_started=true` y `scheduler_started=true` en ambos | Proceso no compone el worker |
| Líder | Exactamente **un** `lease_held=true` por worker en un instante | Dos líderes (bug de lease) o ninguno (sin líder) |
| Seguidor | El otro nodo emite heartbeat con `lease_held=false` | Nodo que no participa |
| Errores | `failed=0` en el primer ciclo; sin `AccessDenied` | IAM o contrato roto |
| Correlación | `instance_id` coincide con las EC2 `InService` del ASG y con la versión de LT | Nodo antiguo sirviendo |

### 6.4 Límites de esta confirmación

- **No sustituye** `aws ec2 describe-instances` ni la versión del Launch
  Template: el log dice lo que el proceso cree; la infraestructura se verifica
  por API.
- **No demuestra entrega real** de un push: eso exige `fcm_send status=ok` con
  `message_id_present=true` y una prueba en dispositivo sintético.
- Hoy no hay exportador, ni worker, ni scheduler: la confirmación es
  **`NOT_RUN`**. No se declara `OBSERVABILIDAD_FCM_VERIFIED`.
- La atribución por petición HTTP a una EC2 concreta sigue sin ser posible
  (CloudFront/ALB no la exponen en la respuesta); el `instance_id` del log es la
  única trazabilidad por nodo.

## 7. Secuencia para habilitar (futura)

1. Implementar exportador y eventos (Go) con pruebas de redacción.
2. Verificar en local que los filtros JSON aceptan los eventos (sin AWS).
3. Gate de coste (`3.00 USD/mes`) y de pipeline.
4. Plan Terraform con `habilitar_metricas_fcm=true` (y alarmas cuando el
   pipeline esté verificado), `version_esperada=<release>`; revisar **sólo**
   filtros/alarmas nuevos, 0 cambios en log group.
5. Apply autorizado por SHA-256; verificar creación y estado `OK`/
   `INSUFFICIENT_DATA` esperado.
6. Tras un despliegue real, ejecutar las consultas de §6.2 y registrar la
   evidencia. Sólo entonces `OBSERVABILIDAD_FCM_VERIFIED`.

## 8. Runbook mínimo por alarma

| Alarma | Primeras comprobaciones | Acción |
|---|---|---|
| A-FCM-1 | ¿Proceso vivo? `systemctl status`; ¿exportador caído?; ¿lease perdido? | Reiniciar/reemplazar nodo; no desactivar alarma |
| A-FCM-2 | ¿Scheduler arrancó? ¿Redis disponible para lease? | Revisar composición; restaurar Redis |
| A-FCM-3 | `error_code` dominante; ¿credenciales FCM? ¿tokens inválidos? | Si `UNREGISTERED`, limpiar registro; si `UNAVAILABLE`, backoff |
| A-FCM-4 | Operación y recurso del `aws_error` | Corregir IAM por Gate; **no** ampliar a `*` |
| A-FCM-5 | ¿Atraso por volumen o por fallo? | Escalar worker o corregir dependencia |
| A-FCM-6 | ¿LT mixta? ¿refresh parcial? | Completar refresh; no ignorar |

## 9. Estado

| Elemento | Estado |
|---|---|
| Filtros/alarmas en Terraform | **Implementados, `count=0`** |
| Eventos en Go | **`NOT_IMPLEMENTED`** |
| Exportador de logs | **`NOT_IMPLEMENTED`** |
| Pipeline AWS de logs | **`LOG_PIPELINE_AWS_VERIFIED = NOT_RUN`** |
| Coste | **3.00 USD/mes `DOC_ONLY`** |
| Entrega de alertas | **`BLOCKED_RFC007_SNS_KMS`** |
| Verificación AWS | **`NOT_RUN`** |
