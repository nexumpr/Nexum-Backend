# Matriz de viabilidad AWS y Free Tier — I-000

**Fecha:** 2026-11-06 (revisión 3) · **Región:** us-east-2 · **Pricing:** us-east-1
**Decisión:** `BLOCKED_FINOPS` (3 motivos; 1 bloqueo real) · **Salida:** `infra/finops/resultados/viabilidad_i000.json`
**Recursos AWS creados:** ninguno · **Terraform ejecutado:** ninguno

## 1. Evidencia AWS obtenida (sólo lectura)

| Consulta | Resultado real | Estado |
|---|---|---|
| `sts get-caller-identity` | identidad válida (cuenta enmascarada) | `VERIFIED_AWS` |
| `freetier get-account-plan-state` | `FREE` / `ACTIVE` / expira 2027-05-04 / créditos 120.0 USD | `VERIFIED_AWS` |
| `freetier get-free-tier-usage` | `freeTierUsages: []` | `VERIFIED_AWS` |
| `service-quotas get-service-quota` `L-1216C47A` | **16 vCPU** On-Demand (dos `t4g.micro` = 4 vCPU) | `VERIFIED_AWS` |
| `ec2 describe-instance-type-offerings` | `t4g.micro` en 2a, 2b, 2c | `VERIFIED_AWS` |
| `elasticache describe-cache-engine-versions` | Redis hasta 7.1 (`redis7`) | `VERIFIED_AWS` |
| `cloudfront list-vpc-origins` | 0 orígenes; **no** acredita elegibilidad | `VERIFIED_AWS` (listado) |
| `dynamodb describe-limits` | 80 000 / 80 000 cuenta; 40 000 / 40 000 tabla | `VERIFIED_AWS` |
| `pricing get-products` | **22 SKU** con `usagetype` y vigencia | `VERIFIED_AWS` |
| `ce get-cost-and-usage` | **NO EJECUTADO** (0.01 USD/solicitud) | `NOT_RUN` |

### Lo que la evidencia no demuestra

- `list-vpc-origins` enumera orígenes existentes: no prueba que la cuenta pueda **crear** uno.
  Crear un VPC origin es mutante y no está autorizado en I-000.
- `get-free-tier-usage` vacío no prueba ausencia de cargos ni cobertura de 45 días.
- La cuota de 16 vCPU permite el diseño, pero no acredita aprovisionamiento.
- No existe evidencia de runtime: `estado_despliegue = NOT_RUN`.

## 2. Requisitos frente a evidencia

| # | Requisito | Estado |
|---|---|---|
| 1 | Cuenta AWS Free verificada | `VERIFIED_AWS` (FREE/ACTIVE) |
| 2 | Vigencia ≥ 45 días | `VERIFIED_AWS` (2027-05-04) |
| 3 | Créditos suficientes (verificado + 10 %) | `VERIFIED_AWS` (86.9473 ≤ 120) |
| 4 | Precios obligatorios verificados | `VERIFIED_AWS` 22 SKU + `DOC_OFICIAL` 3 partidas |
| 5 | Redis OSS disponible y cotizado | `VERIFIED_AWS` (7.1, 0.016) |
| 6 | Dos EC2 simultáneas viables por cuota | `VERIFIED_AWS` (16 vCPU ≥ 4) |
| 7 | ALB cotizado | `VERIFIED_AWS` (`AWSELB`) |
| 8 | CloudFront cotizado | `VERIFIED_AWS` (0.085/GB) |
| 9 | CloudFront VPC Origins elegible en la cuenta | `NOT_VERIFIED` |
| 10 | Impuestos/IVA determinados | `NOT_VERIFIED` |
| 11 | TLS extremo a extremo aceptado | `NOT_VERIFIED` |
| 12 | Costo observado facturado | `NOT_RUN` |
| 13 | Riesgo de cobro personal | PRESENTE (por IVA indeterminado) |
| 14 | Dos instancias reales (diseño) | `DESIGN_VALIDATED` |
| 15 | Dos instancias reales (runtime) | `NOT_RUN` |
| 16 | Red privada sin NAT + IGW para VPC Origins | `DESIGN_VALIDATED` |
| 17 | HTTPS navegador→CloudFront | `DESIGN_VALIDATED` |

## 3. Motivos automáticos (3) y clasificación de Gate

| Motivo | Clasificación (`matriz_gate`) | Base |
|---|---|---|
| `IMPUESTOS_NO_DETERMINADOS` | **BLOQUEO_REAL** | Gate "Cierre de I-000" incluye impuestos/IVA |
| `VPC_ORIGIN_NO_CONFIRMADO` | VERIFICACION_DIFERIDA_A_FASE_POSTERIOR | Gate: ALB/CloudFront HTTPS son obligaciones de Gates posteriores; spec §9 exige confirmarlo al desplegar |
| `TLS_NO_RESUELTO` | CUMPLIDA_CONDICIONADA_A_APROBACION | spec §9: opción A pendiente de aceptación de seguridad |

Reclasificados por evidencia oficial del plan Free: el **riesgo de cobro personal desaparece** con
plan `FREE/ACTIVE` verificado (AWS no admite cargos durante la vigencia) y el **costo observado** pasa
a control recurrente porque no figura en la lista de cierre del Gate. El riesgo vigente es el
**cierre automático de la cuenta** al agotar créditos o caducar (retención de 90 días).

### 3.b Cobertura en dos escenarios

| Escenario | Bruto | Margen 10 % | Cobertura con 120 USD |
|---|---|---|---|
| A — sólo tarifas verificadas | 79.0430 | **86.9473** | TOTAL |
| B — presupuesto modelado completo | 84.0430 | **92.4473** | TOTAL |

Las reservas de modelado (5.0000 USD) se informan aparte y **no** se eliminan para mejorar la
cobertura; sólo se excluyen del bruto verificado porque no son cargo AWS.

## 4. Acciones que resuelven cada bloqueo

| Bloqueo | Acción necesaria | Quién |
|---|---|---|
| IVA | Determinación fiscal responsable o factura real | Humano |
| VPC Origins | Autorizar `CreateVpcOrigin` en fase de despliegue, o aceptar riesgo residual | Humano (mutante) |
| Costo observado | Captura de Billing/Bills + Free Tier, o autorizar 1 solicitud de Cost Explorer (0.01 USD) | Humano |
| Riesgo de cobro | Se cierra al determinar IVA con cobertura `TOTAL` | Derivado |
| TLS | Firmar opción A (`ACEPTO_TLS_CLOUDFRONT_ALB_HTTP_PRIVADO`) o aportar dominio para B | Humano |

## 5. Modelo de decisión

`VIABLE` exige simultáneamente: cuenta Free vigente ≥ 45 días, todas las partidas requeridas con
evidencia aceptada (`VERIFIED_AWS` o `DOC_OFICIAL` con precio 0), bruto verificado calculable,
cobertura `TOTAL` con margen 10 %, impuestos determinados y TLS resuelto. Cualquier hueco produce
`BLOCKED_FINOPS` con el motivo y la evidencia faltante.
