# Matriz de viabilidad AWS y Free Tier — I-000

**Fecha:** 2026-11-06 · **Región comercial:** us-east-2 (Ohio) · **Pricing:** us-east-1
**Decisión vigente:** `BLOCKED_FINOPS` · **Salida reproducible:** `infra/finops/resultados/viabilidad_i000.json`
**Perfil AWS:** `nexum` (sesión válida durante esta corrida) · **Recursos creados:** ninguno

## 1. Evidencia AWS obtenida (sólo lectura)

| Consulta | Resultado real | Estado |
|---|---|---|
| `aws sts get-caller-identity` | Identidad obtenida; cuenta enmascarada en la evidencia | `VERIFIED_AWS` |
| `aws freetier get-account-plan-state` | `accountPlanType=FREE`, `accountPlanStatus=ACTIVE`, expiración `2027-05-04T01:22:26Z`, `accountPlanRemainingCredits = 120.0 USD` | `VERIFIED_AWS` |
| `aws freetier get-free-tier-usage` | `freeTierUsages: []` (sin consumo reportado) | `VERIFIED_AWS` |
| `aws service-quotas get-service-quota` (`L-1216C47A`) | Cuota de vCPU On-Demand = **16** | `VERIFIED_AWS` |
| `aws ec2 describe-instance-type-offerings` (`t4g.micro`) | Disponible en **us-east-2a, us-east-2b, us-east-2c** | `VERIFIED_AWS` |
| `aws elasticache describe-cache-engine-versions` | Motor Redis hasta **7.1** (`redis7`), 6 versiones | `VERIFIED_AWS` |
| `aws cloudfront list-vpc-origins` | `VpcOriginList.Items = []` (0 orígenes existentes) | `VERIFIED_AWS` (listado) |
| `aws dynamodb describe-limits` | Lecturas/escrituras de cuenta 80 000; por tabla 40 000 | `VERIFIED_AWS` |
| `aws pricing get-products` (14 SKU) | Precios on-demand con `usagetype` y vigencia | `VERIFIED_AWS` |
| `aws ce get-cost-and-usage` | **NO EJECUTADO**: 0.01 USD por solicitud sin aprobación | `NOT_RUN` |

### Lo que la evidencia **no** demuestra

- `list-vpc-origins` enumeró **orígenes existentes** (ninguno). No acredita que la cuenta pueda
  **crear** VPC Origins: `cloudfront_vpc_origin_habilitado` queda `null` y bloquea.
- `get-free-tier-usage` vacío significa que la cuenta no reporta consumo de Free Tier en este
  momento; **no** prueba que no existan cargos ni que el plan cubra 45 días de operación.
- La cuota de 16 vCPU permite dos `t4g.micro` (2 vCPU), pero **no** demuestra que se vayan a
  aprovisionar: eso corresponde a fases posteriores.
- No se creó ni modificó ningún recurso, presupuesto, alarma o certificado.

## 2. Requisitos frente a evidencia

| # | Requisito | Estado | Evidencia / motivo |
|---|---|---|---|
| 1 | Plan de cuenta y contexto | `VERIFIED_AWS` | `FREE` / `ACTIVE` |
| 2 | Vigencia que cubra 45 días | `VERIFIED_AWS` | Expira 2027-05-04 (≈180 días) |
| 3 | Créditos verificados | `VERIFIED_AWS` | 120.0 USD (`accountPlanRemainingCredits`) |
| 4 | Costo bruto verificado 45 d | `UNKNOWN` | 11 partidas requeridas sin precio verificado |
| 5 | Cobertura de créditos | `UNKNOWN` | No calculable sin bruto verificado |
| 6 | Impuestos / IVA | `UNKNOWN` | Sin determinación fiscal; no se asume 0 % |
| 7 | Redis OSS disponible y cotizado | `VERIFIED_AWS` | Motor 7.1; `USE2-NodeUsage` 0.016 USD/nodo-hora |
| 8 | Dos EC2 simultáneas viables por cuota | `VERIFIED_AWS` | 16 vCPU ≥ 2 vCPU requeridas |
| 9 | `t4g.micro` en dos AZ | `VERIFIED_AWS` | 2a, 2b, 2c |
| 10 | ALB cotizado | `VERIFIED_AWS` | 0.0225/h + 0.008/LCU-h (código `AWSELB`) |
| 11 | CloudFront cotizado | `VERIFIED_AWS` | Egreso 0.085/GB (primer tramo US) |
| 12 | CloudFront VPC Origins disponible | `NOT_VERIFIED` | `list-vpc-origins` no acredita elegibilidad |
| 13 | Precios obligatorios verificados | `NOT_VERIFIED` | 11 partidas requeridas pendientes |
| 14 | IVA y riesgos fiscales resueltos | `NOT_VERIFIED` | Sin determinación fiscal |
| 15 | TLS conforme a decisión humana | `NOT_VERIFIED` | Tramo CloudFront→ALB es HTTP privado; sin aceptación documentada |
| 16 | Costo observado (¿0.00 USD?) | `NOT_RUN` | Cost Explorer no ejecutado (0.01 USD/solicitud) |
| 17 | Dos instancias reales (diseño) | `DESIGN_VALIDATED` | 2 procesos servidor, AZ distintas, detrás de balanceador real |
| 18 | Dos instancias reales (runtime) | `NOT_RUN` | Sin recursos creados; prohibido en I-000 |
| 19 | Conectividad privada sin NAT (diseño) | `DESIGN_VALIDATED` | Gateway endpoints S3+DynamoDB; 0 NAT; 0 IPv4 pública |
| 20 | HTTPS de navegador a CloudFront (diseño) | `DESIGN_VALIDATED` | TLS 1.2+ con `*.cloudfront.net` |
| 21 | Política de gasto personal | CERO | Regla vigente; no acredita ausencia de cobro |

## 3. Motivos del bloqueo (7)

1. `IMPUESTOS_NO_DETERMINADOS` — no se asume IVA 0 %.
2. `VPC_ORIGIN_NO_CONFIRMADO` — `list-vpc-origins` no prueba elegibilidad de la cuenta.
3. `COSTO_OBSERVADO_NO_VERIFICADO` — Cost Explorer no ejecutado por su costo por solicitud.
4. `PRECIOS_NO_VERIFICADOS` — 11 partidas requeridas sin precio `VERIFIED_AWS`.
5. `COSTO_VERIFICADO_DESCONOCIDO` — sin bruto verificado no hay cifra contractual.
6. `COBERTURA_NO_CALCULABLE` — la cobertura de los 120 USD no puede afirmarse.
7. `TLS_NO_RESUELTO` — el tramo CloudFront→ALB sigue en HTTP privado sin aceptación humana.

## 4. Qué falta para `VIABLE`

1. Precio `VERIFIED_AWS` de las 11 partidas pendientes (backup/restore DynamoDB, retención,
   métricas y alarmas de CloudWatch, Secrets Manager, solicitudes S3, transferencia inter-AZ y las
   tres partidas de diseño sin cargo).
2. Determinación fiscal del IVA aplicable.
3. Decisión humana sobre TLS extremo a extremo (o recotización con ACM y nombre válido).
4. Confirmación de elegibilidad de CloudFront VPC Origins en la cuenta.
5. Evidencia de costo observado: consola de facturación (acción humana) o autorización concreta
   para una solicitud de Cost Explorer (0.01 USD).

Ninguna condición se relaja por disponibilidad de créditos ni por ausencia de errores.

## 5. Modelo de decisión

`VIABLE` sólo si **todos** los requisitos bloqueantes están verificados: cuenta Free vigente ≥ 45
días, precios `VERIFIED_AWS` completos, bruto verificado calculable, cobertura `TOTAL` con margen
del 10 %, impuestos determinados y TLS resuelto. Cualquier hueco produce `BLOCKED_FINOPS` con el
motivo y la evidencia que falta.
