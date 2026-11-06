# Matriz de costos de servicios AWS — I-000 (45 días, us-east-2)

**Fecha de corte:** 2026-11-06 · **Moneda:** USD · **Aritmética:** `Decimal` a 4 decimales
**Motor:** `infra/finops/modelo_costos.py` · **Salida:** `infra/finops/resultados/costos_45d.json`
**Precios:** `aws pricing get-products` (Price List Query API, endpoint `us-east-1`), evidencia en
`infra/finops/evidencia/precios_2026-11-06.json`

> **Trazabilidad de evidencia.** 14 SKU tienen precio `VERIFIED_AWS` con `usagetype` y fecha de
> vigencia. Las partidas sin consulta de precio permanecen `DOC_ONLY`/`HIPOTESIS`/`SUPUESTO` y por
> eso **el bruto verificado es `UNKNOWN`**: no se presenta un total modelado como verificado.

## 1. Totales por escenario

| Escenario | Horas | 24×7 | Bruto calculado 45 d | Bruto verificado 45 d | Partidas sin verificar | Residual apagado |
|---|---|---|---|---|---|---|
| **C — CloudFront + VPC Origin + ALB privado + 2 EC2** | 1080 | Sí | **84.0080** | `UNKNOWN` | 11 | 6.8440 |
| A — 2 EC2 24×7 con ALB público e IPv4 | 1080 | Sí | 91.8080 | `UNKNOWN` | 9 | 6.8440 |
| B — 2 EC2 simultáneas sólo en ventanas de 8 h/día | 360 | **No** | 28.9820 | 28.9820 | 0 | 6.8440 |

- **B no cumple 24×7**: se mantiene únicamente como comparador de ahorro y no es válido para la
  demostración exigida.
- El bruto verificado de C es `UNKNOWN` porque 11 partidas requeridas no tienen precio verificado
  (ver §4). El bruto calculado usa tarifas históricas para esas líneas y sirve como orden de
  magnitud, no como cifra contractual.
- El modelo reproduce exactamente la aritmética histórica V3 (84.0580 USD) cuando se le alimenta
  con las tarifas V3: `test_aritmetica_del_modelo_v3_se_reproduce`.

## 2. Partidas con precio verificado en AWS (14)

| SKU | Cantidad 45 d | Unidad | Precio unitario | Subtotal | `usagetype` verificado |
|---|---|---|---|---|---|
| EC2 `t4g.micro` | 2160 | hora-instancia | 0.0084 | 18.1440 | `USE2-BoxUsage:t4g.micro` |
| ALB fijo | 1080 | hora-ALB | 0.0225 | 24.3000 | `USE2-LoadBalancerUsage` |
| ALB LCU | 1080 | LCU-hora | 0.008 | 8.6400 | `USE2-LCUUsage` |
| ElastiCache Redis OSS `cache.t4g.micro` | 1080 | nodo-hora | 0.016 | 17.2800 | `USE2-NodeUsage:cache.t4g.micro` |
| EBS gp3 | 30 | GB-mes | 0.08 | 2.4000 | `USE2-EBS:VolumeUsage.gp3` |
| EBS snapshot | 3 | GB-mes | 0.05 | 0.1500 | `USE2-EBS:SnapshotUsage` |
| DynamoDB almacenamiento | 3 | GB-mes | 0.25 | 0.7500 | `USE2-TimedStorage-ByteHrs` (tramo 25–∞) |
| DynamoDB lecturas | 2 | millón-RRU | 0.125 | 0.2500 | `USE2-ReadRequestUnits` |
| DynamoDB escrituras | 0.6 | millón-WRU | 0.625 | 0.3750 | `USE2-WriteRequestUnits` |
| DynamoDB PITR | 3 | GB-mes | 0.2 | 0.6000 | `USE2-TimedPITRStorage-ByteHrs` |
| S3 Standard (state) | 3 | GB-mes | 0.023 | 0.0690 | `USE2-TimedStorage-ByteHrs` |
| CloudWatch ingesta de logs | 3 | GB | 0.5 | 1.5000 | `USE2-DataProcessing-Bytes` |
| IPv4 pública | 0 | IP-hora | 0.005 | 0.0000 | `USE2-PublicIPv4:InUseAddress` (excluida por diseño) |
| CloudFront egreso | 10 | GB | 0.085 | 0.8500 | `US-DataTransfer-Out-Bytes` (primer tramo) |

**Defectos corregidos en la extracción:**

1. **ElastiCache** tomaba `USE2-ExtendedSupportYr3-NodeUsage` (0.0260) en lugar de
   `USE2-NodeUsage` (0.0160). Ahora se exige `usagetype` exacto.
2. **DynamoDB almacenamiento** tomaba el tramo gratuito (0.0000, 0–25 GB). Ahora se elige el
   primer tramo con precio distinto de cero (`beginRange` 25).
3. **CloudFront** pasó de 0.0900 (supuesto H05) a 0.0850 verificado; **DynamoDB PITR** y
   **snapshot EBS** se verificaron por primera vez.

## 3. Partidas sin precio verificado (bloquean el bruto verificado)

| Partida | Estado del precio | Motivo |
|---|---|---|
| DynamoDB backup | `DOC_ONLY` | Sin consulta de precio; el SKU on-demand requiere atributos adicionales |
| DynamoDB restore | `DOC_ONLY` | Sin consulta de precio |
| CloudWatch retención de logs | `HIPOTESIS` | Estructura de `usagetype` de retención no confirmada |
| CloudWatch métricas personalizadas | `DOC_ONLY` | Sin consulta de precio |
| CloudWatch alarmas | `DOC_ONLY` | Sin consulta de precio |
| Secrets Manager | `DOC_ONLY` | Código de servicio no incluido en la lista validada |
| S3 solicitudes | `SUPUESTO` | Reserva de modelado, no tarifa |
| Transferencia inter-AZ Redis | `DOC_ONLY` | Depende de tráfico real |
| Certificado CloudFront por defecto | `DISENO_DOC` | Se asume 0 por documentación AWS, sin verificación de precio |
| CloudFront VPC Origin | `DISENO_DOC` | Funcionalidad sin cargo según AWS; elegibilidad no confirmada |
| Gateway endpoints S3+DynamoDB | `DISENO_DOC` | Sin cargo por hora según AWS; no verificado por Price List |
| NAT Gateway / interface endpoints | `UNKNOWN` | Excluidos por diseño; `null` nunca es 0 |
| Impuestos / IVA | `UNKNOWN` | Sin determinación fiscal; no se imputa 0 % |

## 4. Costos residuales (runtime apagado)

Con EC2, ALB, Redis, egreso, transferencia inter-AZ e ingesta detenidos permanecen **6.8440 USD**:
EBS (2.40), snapshot (0.15), almacenamiento DynamoDB (0.75), lecturas/escrituras conservadas por
prudencia (0.625), PITR (0.60), backup (0.30), restore (0.30), retención de logs (0.135), alarmas
(0.30), Secrets Manager (1.20) y S3 state (0.069 + 0.015). Las reservas no AWS (CI/CD, reserva
CloudFront) se excluyen. Las métricas personalizadas y la ingesta se consideran apagables.

## 5. Separación financiera (escenario C)

| Concepto | Valor | Nivel de evidencia |
|---|---|---|
| Costo bruto calculado 45 d | 84.0080 USD | Tarifas mixtas (14 verificadas + 11 históricas) |
| Costo bruto verificado 45 d | `UNKNOWN` | Faltan 11 partidas requeridas |
| Créditos declarados (histórico operador) | 120.00 USD | `DOC_ONLY` |
| Créditos verificados (AWS) | 120.0000 USD | `accountPlanRemainingCredits`, 2026-11-06 |
| Margen de seguridad aplicado | 10 % | Regla del Gate H08-I |
| Créditos requeridos para cobertura TOTAL | `UNKNOWN` | Requiere bruto verificado |
| Cobertura efectiva | `UNKNOWN` | No calculable sin bruto verificado |
| Importe sin cobertura | `UNKNOWN` | No se asume cero |
| Impuestos / IVA | `UNKNOWN` | No imputado |
| Política de gasto personal | CERO | Regla irrenunciable |
| Desembolso personal autorizado | 0.0000 USD | Política, no prueba de ausencia de cobro |
| Riesgo de cobro personal | **PRESENTE** | Cobertura `UNKNOWN` e impuestos sin determinar |

> La política de gasto personal cero **no** demuestra que la infraestructura no genere un importe
> pendiente. Mientras el bruto verificado y el IVA sean desconocidos, el riesgo se declara presente.
