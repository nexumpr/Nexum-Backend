# Gate H08-I — I-110 `observabilidad-costos` (implementación local)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/GATE_H08I_I110.md` |
| Fecha | 2026-11-08 |
| Spec | `sdd/02-infraestructura/I-110-observabilidad-costos/` (v0.1.0 DRAFT; spec/plan intactos) |
| Estado | **APRUEBO_GATE_I110_LOCAL REGISTRADO — IMPLEMENTADO_LOCAL (sin AWS)** |
| Main base | `6542e724e0b203d8074f41b5abaad739a9b09feb` (PR #30, I-100) |
| Rama | `feat/h08i-i110-observabilidad` |
| Autorización | `APRUEBO_GATE_I110_LOCAL` (mensaje humano independiente, 2026-11-08) |

## 1. Alcance

Implementación local del módulo Terraform de observabilidad (log group con retención finita, metric
filters, cinco alarmas, SNS, presupuesto y dashboard), el registro canónico de logs y las pruebas de
contrato/FinOps. Sin AWS, sin `apply`, sin X-Ray ni access logs.

**No autoriza** crear recursos AWS, aplicar Terraform, configurar el agente de CloudWatch, desplegar ni
iniciar I-120/I-140.

## 2. Implementado

| Requisito | Evidencia | Estado |
|---|---|---|
| INF-110-01 Logs | `RequestRecord` con campos canónicos y validación de claves sensibles; `POLITICA_LOGS.md`; redacción A-090 | IMPLEMENTADO_LOCAL |
| INF-110-02 Métricas | 4 metric filters (5xx, 401, 429, readiness), métricas ALB, `/metrics` local loopback (RPS/latencia/runtime) y consultas p50/p95/p99 | IMPLEMENTADO_LOCAL |
| INF-110-03 Alertas | 8 alarmas: A1 targets<2, A2 5xx, A3 401, A4 readiness, A5 logs sin eventos, A6 429, A7 Redis, A8 DynamoDB; notificación `BLOCKED_RFC007_SNS_KMS` | IMPLEMENTADO_LOCAL con bloqueo |
| INF-110-04 Retención | Log group `/nexum/<entorno>/backend`, retención 7 por defecto, máximo 30 por precondition; coste I-000 | IMPLEMENTADO_LOCAL |
| INF-110-05 Trazabilidad | `traceId`/`requestId` de A-090; dashboard Logs Insights; sin claims/tokens/PII | IMPLEMENTADO_LOCAL |
| INF-110-06 FinOps | Presupuesto declarativo 1 USD sin notificación hasta RFC-007; alineación con líneas CloudWatch de I-000 | IMPLEMENTADO_LOCAL con bloqueo |

## 3. Límites y bloqueos

- **Agente de CloudWatch**: no configurado en I-020; sin él no llegan logs y A5 se dispara. Dependencia
  bloqueante para observabilidad real, no corregida aquí.
- **AWS `NOT_RUN`**: ningún log group, alarma, SNS o presupuesto creado.
- **Notificación de alarmas**: `aws_sns_topic` con clave gestionada por AWS dispara Trivy AWS-0136; una CMK
  de cliente tiene coste no presupuestado. Se retiraron SNS y `alarm_actions`.
  `ALERT_DELIVERY = BLOCKED_RFC007_SNS_KMS`. Las notificaciones de **AWS Budgets por email** son una
  capacidad distinta (sin SNS), implementadas con `emails_presupuesto` vacío por defecto; sin destinatario
  verificado no se declaran funcionales.
- **Dimensiones ALB**: `LoadBalancer` se deriva a `app/<nombre>/<id>` (sin `loadbalancer/`) y `TargetGroup`
  a `targetgroup/<nombre>/<id>`; `dimensiones.tftest.hcl` evalúa los valores en CI con `terraform test`.
- **Logs Insights**: consultas con `pct(latencyMs, N)` (no `percentile`); validación estática, ejecución
  real `NOT_RUN`.
- **Rutas y cardinalidad**: rutas no registradas → `/unknown`; métricas limitadas a 500 series con contador
  de descartes; sin PII, IDs ni tokens en etiquetas.
- **Coste**: 4 metric filters + 8 alarmas; I-000 presupuesta 3 alarmas, por lo que hay **+0.5000 USD/mes**
  estimados (no medidos). El margen de 6.9387 USD lo cubre, pero no se declara medido.
- **Pipeline de logs**: `LOG_PIPELINE_IMPLEMENTED_LOCAL`; `LOG_PIPELINE_AWS_VERIFIED = NOT_RUN`. I-010 ya
  declara el interface endpoint `logs` y I-060 concede `logs:CreateLogStream`/`PutLogEvents` sobre ARNs
  acotados, pero falta que I-020 instale/configure el agente de CloudWatch en las instancias. Sin agente no
  hay entrega real; no se modifica I-020 aquí.
- **Tracing**: W3C y X-Request-ID reutilizan A-090. Pruebas locales cubren válido/inválido; se registra
  **D-110-01 (A-090)**: sólo se valida longitud, no hexadecimalidad/flags, y `X-Request-ID` no tiene
  límite; no se corrige fuera de su Gate.
- **Dinero y créditos**: `AWS_COST_REAL = NOT_VERIFIED`, `AWS_CREDITS = NOT_VERIFIED`,
  `AWS_SCP = NOT_VERIFIED`; no se declara USD 0.
- **Dashboard**: no incluye DynamoDB/Redis hasta recibir sus identificadores (I-040/I-050); OTel/runtime
  Go no exportado (`NOT_RUN`).
- **X-Ray, ALB access logs y tracing 100 %**: no habilitados para evitar coste no presupuestado.
- Siguen abiertos RFC-004, H-01, RFC-006, D-100-01/02, readiness 503 y `AWS_SCP_NOT_VERIFIED`.

## 4. Verificación local

`terraform fmt -check`, `init -backend=false` y `validate` del entorno; pruebas de contrato Terraform
(10) y FinOps (6); Go telemetry (6 pruebas nuevas); regresiones Terraform/FinOps/systemd/CI/CD/DevSecOps,
SDD y SHA-256.

## 5. Cierre

Merge requiere autorización humana ligada a PR y HEAD exactos. No se declara `AWS_DEPLOYED`,
`OBSERVABILITY_VERIFIED` ni `PRODUCTION_READY`. I-120 no se inicia sin su Gate.

## 6. Registro de integración — 2026-11-09

- `APRUEBO_MERGE_PR_31_SHA_c8f1049bdbcdc88ea51aa1fd07ada3b5ca8a940b` concedido por el responsable;
  HEAD verificado por igualdad exacta y `--match-head-commit` antes de fusionar.
- **PR #31 `MERGED`** (`2026-11-09T01:40:17Z`), merge commit y `main` = **`071478a1d23ff2da44bb7b7ee94d9fc98c686fa9`**;
  `git pull --ff-only` dejó `main` local igual a `origin/main`.
- CI pre-merge: 7/7 PASS (runs 42545254647 y 42545254656) sobre el HEAD autorizado.
- CI post-merge en `main`: **SDD documental `success`** (run 42546793045) y **Go Backend CI `success`**
  (run 42546793065, seis jobs en verde).
- Estado: **INTEGRADO_MAIN — NO DESPLEGADO**. `AWS_NOT_RUN`, `LOG_PIPELINE_AWS_VERIFIED = NOT_RUN`,
  `ALERT_DELIVERY = BLOCKED_RFC007_SNS_KMS`, `DAST_NOT_RUN`. Defectos `D-100-01`, `D-100-02` y `D-110-01`
  permanecen abiertos; RFC-004, H-01 y RFC-006 no cambian.
- Este registro es documental: no se tocó Terraform, Go, OpenAPI ni workflows tras el merge.
