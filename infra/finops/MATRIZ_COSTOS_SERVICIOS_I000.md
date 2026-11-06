# Matriz de costos de servicios AWS — I-000 (45 días, us-east-2)

**Fecha:** 2026-11-06 (revisión 3) · **Moneda:** USD · **Aritmética:** `Decimal` a 4 decimales
**Motor:** `infra/finops/modelo_costos.py` · **Salida:** `infra/finops/resultados/costos_45d.json`
**Precios:** `aws pricing get-products` (endpoint `us-east-1`) → `infra/finops/evidencia/precios_2026-11-06.json`

## 1. Grados de evidencia

| Grado | Significado | Cuenta para bruto verificado |
|---|---|---|
| `VERIFIED_AWS` | Precio de la Price List API con `usagetype` y fecha | Sí |
| `DOC_OFICIAL` | AWS documenta expresamente que **no hay cargo** (precio 0) | Sí, sólo si el precio es 0 y hay `fuente_oficial` |
| `DOC_ONLY` / `HIPOTESIS` / `SUPUESTO` | Modelado histórico o reserva no tarifaria | No |
| `UNKNOWN` | Precio no determinado; **nunca** se interpreta como 0 | No |

## 2. Totales por escenario

| Escenario | Horas | 24×7 | Bruto calculado | Bruto verificado | Residual apagado |
|---|---|---|---|---|---|
| **C — CloudFront + VPC Origin + ALB privado + 2 EC2** | 1080 | Sí | **84.0430** | **79.0430** | **6.8790** |
| A — 2 EC2 24×7 con ALB público e IPv4 | 1080 | Sí | 91.8430 | 89.8430 | 6.8790 |
| B — 2 EC2 simultáneas en ventanas de 8 h/día | 360 | **No** | 28.9820 | 28.9820 | 6.8790 |

- La diferencia de **5.0000 USD** entre bruto calculado y verificado de C corresponde a las reservas
  de modelado **no facturables** (`CI_CD_RESERVA` 2.00 y `RESERVA_CLOUDFRONT_VARIABLE` 3.00), que no
  cuentan como precio verificado.
- **B no cumple 24×7**: sólo comparador de ahorro, no entorno válido.

## 3. Partidas con precio verificado en AWS (22 SKU)

| SKU | Cantidad 45 d | Unidad | Precio | Subtotal | `usagetype` |
|---|---|---|---|---|---|
| EC2 `t4g.micro` | 2160 | hora-instancia | 0.0084 | 18.1440 | `USE2-BoxUsage:t4g.micro` |
| ALB fijo | 1080 | hora-ALB | 0.0225 | 24.3000 | `USE2-LoadBalancerUsage` |
| ALB LCU | 1080 | LCU-hora | 0.008 | 8.6400 | `USE2-LCUUsage` |
| Redis OSS `cache.t4g.micro` | 1080 | nodo-hora | 0.016 | 17.2800 | `USE2-NodeUsage:cache.t4g.micro` |
| EBS gp3 | 30 | GB-mes | 0.08 | 2.4000 | `USE2-EBS:VolumeUsage.gp3` |
| EBS snapshot | 3 | GB-mes | 0.05 | 0.1500 | `USE2-EBS:SnapshotUsage` |
| DynamoDB almacenamiento | 3 | GB-mes | 0.25 | 0.7500 | `USE2-TimedStorage-ByteHrs` (tramo 25–∞) |
| DynamoDB lecturas | 2 | millón-RRU | 0.125 | 0.2500 | `USE2-ReadRequestUnits` |
| DynamoDB escrituras | 0.6 | millón-WRU | 0.625 | 0.3750 | `USE2-WriteRequestUnits` |
| DynamoDB PITR | 3 | GB-mes | 0.2 | 0.6000 | `USE2-TimedPITRStorage-ByteHrs` |
| DynamoDB backup | 3 | GB-mes | 0.1 | 0.3000 | `USE2-TimedBackupStorage-ByteHrs` |
| DynamoDB restore | 2 | GB | 0.15 | 0.3000 | `USE2-RestoreDataSize-Bytes` |
| S3 Standard (state) | 3 | GB-mes | 0.023 | 0.0690 | `USE2-TimedStorage-ByteHrs` |
| S3 solicitudes Tier1 | 10 | mil-solicitudes | 0.005 | 0.0500 | `USE2-Requests-Tier1` |
| CloudWatch ingesta | 3 | GB | 0.5 | 1.5000 | `USE2-DataProcessing-Bytes` |
| CloudWatch retención | 4.5 | GB-mes | 0.03 | 0.1350 | `USE2-TimedStorage-ByteHrs` |
| CloudWatch métricas | 4.5 | métrica-mes | 0.3 | 1.3500 | `USE2-CW:MetricMonitorUsage` |
| CloudWatch alarmas | 3 | alarma-mes | 0.1 | 0.3000 | `USE2-CW:AlarmMonitorUsage` |
| Secrets Manager | 3 | secreto-mes | 0.4 | 1.2000 | `USE2-AWSSecretsManager-Secrets` |
| Transferencia inter-AZ | 10 | GB | 0.01 | 0.1000 | `USE2-DataTransfer-Regional-Bytes` |
| CloudFront egreso | 10 | GB | 0.085 | 0.8500 | `US-DataTransfer-Out-Bytes` (primer tramo) |
| IPv4 pública | 0 | IP-hora | 0.005 | 0.0000 | `USE2-PublicIPv4:InUseAddress` (excluida) |

## 4. Partidas sin cargo, con documentación oficial (`DOC_OFICIAL`)

| Partida | Alcance exacto | Fuente |
|---|---|---|
| Certificado CloudFront por defecto | TLS con `*.cloudfront.net`; no cubre dominio propio | `aws.amazon.com/cloudfront/pricing/` |
| CloudFront VPC Origins | Funcionalidad sin cargo adicional; exige Internet Gateway adjunto e IPv4 libre en subred privada | `docs.aws.amazon.com/.../private-content-vpc-origins.html` |
| Gateway endpoints S3 + DynamoDB | Sin cargo por hora ni por GB procesado | `aws.amazon.com/privatelink/pricing/` |

Los tres tienen precio 0 y fuente oficial; **no** son SKU de Price List y por eso no se marcan
`VERIFIED_AWS`.

## 5. Partidas con cantidad cero (demostrado)

| Partida | Motivo |
|---|---|
| IPv4 pública EC2/ALB | Topología C sin IPv4 públicas |
| Zona Route 53 | Se usa `*.cloudfront.net`; no hay dominio propio |
| NAT Gateway | Salida por gateway endpoints; el IGW de VPC Origins no enruta |
| Interface endpoints | Sólo se usan gateway endpoints (los interface tienen costo por hora) |

## 6. Separación financiera (escenario C)

| Concepto | Valor | Nivel |
|---|---|---|
| Costo bruto calculado 45 d | 84.0430 USD | 22 SKU verificados + reservas modeladas |
| Costo bruto verificado 45 d | **79.0430 USD** | `VERIFIED_AWS` + `DOC_OFICIAL` |
| Créditos declarados (histórico) | 120.00 USD | `DOC_ONLY` |
| Créditos verificados (AWS) | **120.0000 USD** | `accountPlanRemainingCredits` |
| Margen de seguridad | 10 % | Gate H08-I |
| Créditos requeridos | **86.9473 USD** | bruto verificado × 1.10 |
| Cobertura | **TOTAL** | 86.9473 ≤ 120.0000 |
| Importe sin cobertura | 0.0000 USD | calculado |
| Impuestos / IVA | `UNKNOWN` | no imputado |
| Política de gasto personal | CERO | regla |
| Desembolso personal autorizado | 0.0000 USD | política, no prueba |
| Riesgo de cobro personal | **PRESENTE** | por IVA no determinado |
| Residual con runtime apagado | 6.8790 USD | persistente |

> Cobertura `TOTAL` **no** significa `VIABLE`: el IVA indeterminado y los demás bloqueos mantienen la
> decisión en `BLOCKED_FINOPS`.
