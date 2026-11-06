# Matriz de viabilidad AWS y Free Tier — I-000

**Fecha:** 2026-11-06 · **Región:** us-east-2 (Ohio) · **CloudFront:** global
**Decisión vigente:** `BLOCKED_FINOPS` · **Salida reproducible:** `infra/finops/resultados/viabilidad_i000.json`

## 1. Requisitos frente a evidencia

| # | Requisito (spec I-000) | Estado | Evidencia / motivo |
|---|---|---|---|
| 1 | Plan de cuenta y contexto (INF-000-01) | `NOT_VERIFIED` | `aws sts get-caller-identity` devolvió *Your session has expired. Please reauthenticate using 'aws login'* |
| 2 | Fecha de alta, expiración y créditos | `NOT_VERIFIED` | `aws freetier get-account-plan-state` no ejecutado con sesión válida |
| 3 | Uso real del plan Free Tier | `NOT_RUN` | `aws freetier get-free-tier-usage` pendiente |
| 4 | Costo observado (¿0.00 USD?) | `NOT_RUN` | Cost Explorer no ejecutado: cobra 0.01 USD por solicitud y no hay aprobación específica |
| 5 | Presupuestos existentes | `NOT_RUN` | `aws budgets describe-budgets`; crear presupuestos está prohibido en I-000 |
| 6 | SKU exacto de ElastiCache Redis OSS | `NOT_VERIFIED` | Requiere `describe-cache-engine-versions` + Price List API |
| 7 | CloudFront VPC Origins disponible en la cuenta | `NOT_VERIFIED` | Requiere `list-vpc-origins`; condición del ADR V3 |
| 8 | Cuota de vCPU On-Demand para 2 instancias | `NOT_VERIFIED` | Requiere `servicequotas get-service-quota` (L-1216C47A) |
| 9 | Disponibilidad de t4g.micro por zona | `NOT_VERIFIED` | Requiere `ec2 describe-instance-type-offerings` |
| 10 | Precios vigentes Ohio de todas las partidas | `NOT_VERIFIED` | Catálogo en `DOC_ONLY`/`HIPOTESIS`/`SUPUESTO`; ninguna fila `VERIFIED_AWS` |
| 11 | Dos instancias reales simultáneas (diseño) | `PASS (diseño)` | `requisitos.py`: 2 procesos servidor, AZ distintas, detrás de balanceador real |
| 12 | Derecho a ejecutar 2 instancias (runtime) | `NOT_RUN` | Sin recursos creados; prohibido en I-000 |
| 13 | Conectividad privada sin NAT (diseño) | `PASS (diseño)` | Gateway endpoints S3+DynamoDB, 0 NAT, 0 IPv4 pública |
| 14 | HTTPS de navegador a CloudFront | `PASS (diseño)` | TLS 1.2+ con dominio `*.cloudfront.net` |
| 15 | TLS extremo a extremo hasta ALB | `NO ACEPTADO` | Tramo CloudFront→ALB es HTTP privado; requiere aceptación formal o recotización |
| 16 | Impuestos / IVA en México | `UNKNOWN` | No se imputa 0 % ni monto sin determinación fiscal |
| 17 | Costo bruto a 45 días | `CALCULADO` 84.0580 USD | Con precios no verificados; ver matriz de costos |
| 18 | Cobertura del bruto por créditos | `NOT_VERIFIED` | Créditos verificados 0.00 USD; los 120 USD son históricos |
| 19 | Cero desembolso personal | `PASS (regla)` | El modelo no contempla pago propio ni plan Paid |
| 20 | Presupuesto/alarmas preventivas | `NOT_RUN` | Prohibido crearlas en I-000; además no impiden cargos |

## 2. Motivos del bloqueo

1. `EVIDENCIA_CUENTA_NO_VERIFICADA` — sesión AWS expirada: no hay plan, vigencia ni créditos verificados.
2. `PLAN_NO_DETERMINADO` y `VIGENCIA_NO_DETERMINADA` — sin `get-account-plan-state`.
3. `CREDITOS_NO_VERIFICADOS` — 120 USD declarados en H05 no equivalen a saldo actual.
4. `EVIDENCIA_SERVICIOS_NO_VERIFICADA` — Redis OSS, VPC Origins, cuota vCPU y t4g.micro sin confirmar.
5. `COSTO_OBSERVADO_NO_VERIFICADO` — no se puede afirmar “0 USD facturado”.
6. `PRECIOS_NO_VERIFICADOS` — 28 partidas requeridas sin tarifa `VERIFIED_AWS`.

## 3. Condiciones que no bloquean por sí solas, pero deben cerrarse

- IVA/impuestos indeterminados: no se imputa 0 %.
- TLS extremo a extremo CloudFront→ALB no incluido en la propuesta.
- Reservas de modelado (S3 solicitudes, egreso, CI/CD, reserva CloudFront) son supuestos, no tarifas.
- Duración: aunque el plan declare vigencia suficiente, los créditos pueden agotarse antes de 45 días.

## 4. Qué falta para poder declarar `VIABLE`

1. Sesión AWS válida (`aws login`) y ejecución de las consultas read-only del catálogo (`--listar`).
2. Confirmación de plan Free, fecha de expiración y **saldo real** de créditos.
3. Ejecución de las seis consultas `aws pricing get-products` y actualización del catálogo a `VERIFIED_AWS`.
4. Confirmación de SKU Redis OSS, VPC Origins, cuota vCPU y disponibilidad de t4g.micro.
5. Decisión humana sobre IVA y sobre TLS extremo a extremo (o recotización).
6. Evidencia de costo observado: consola de facturación (acción humana) o autorización concreta para
   una solicitud de Cost Explorer (0.01 USD).

Ninguna de estas condiciones se satisface por inferencia, por disponibilidad de créditos ni por
ausencia de errores.
