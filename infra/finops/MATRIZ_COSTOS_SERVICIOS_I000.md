# Matriz de costos de servicios AWS — I-000 (45 días, us-east-2)

**Fecha de corte:** 2026-11-06 · **Moneda:** USD · **Importes exactos:** `Decimal` a 4 decimales
**Motor:** `infra/finops/modelo_costos.py` · **Salida:** `infra/finops/resultados/costos_45d.json`

> **Advertencia de evidencia:** ninguna partida está verificada contra la AWS Price List API en esta
> corrida (sesión AWS expirada). Todos los precios son `DOC_ONLY`, `HIPOTESIS`, `SUPUESTO` o
> `DISENO_DOC` a partir de `sdd/02-infraestructura/FINOPS_45_DIAS_MODELO_V3.json` (2026-11-05).
> Se reproducen para poder auditar la aritmética, no como tarifas vigentes.

## 1. Totales por escenario

| Escenario | Horas encendido | 24×7 | Bruto 45 d | Costo residual apagado | Válido para demostración |
|---|---|---|---|---|---|
| **C — CloudFront + VPC Origin + ALB privado + 2 EC2 privadas** | 1080 | Sí | **84.0580** | **6.8440** | Sí |
| A — 2 instancias 24×7 con ALB público e IPv4 públicas | 1080 | Sí | 91.8580 | 6.8440 | Sí (comparador) |
| B — 2 instancias simultáneas sólo en ventanas de 8 h/día | 360 | **No** | 29.0320 | 6.8440 | **No** |

- El escenario B **no** se presenta como 24×7: incumple la disponibilidad exigida.
- El escenario A es un recomputo con las tarifas V3 más IPv4 pública de las instancias; **no**
  reproduce el importe histórico V2 de 103.41 USD por diferencias de SKU. No modela IPv4 del ALB
  internet-facing, por lo que es un piso, no un techo.
- El total C reproduce **exactamente** `estimado_total_45d_usd` de V3 (`84.0580`), verificado por
  `tests/infra/finops/test_modelo_costos.py::test_reproduce_total_v3_documentado`.

## 2. Partidas del escenario C

| Partida | Cantidad | Unidad | Precio unitario | Subtotal 45 d | Estado de evidencia |
|---|---|---|---|---|---|
| EC2 Go (2 instancias) | 2160 | hora-instancia | 0.0084 | 18.1440 | DOC_ONLY |
| ALB fijo | 1080 | hora-ALB | 0.0225 | 24.3000 | DOC_ONLY |
| ALB LCU | 1080 | LCU-hora | 0.008 | 8.6400 | DOC_ONLY |
| ElastiCache Redis OSS | 1080 | nodo-hora | 0.016 | 17.2800 | DOC_ONLY |
| EBS gp3 | 30 | GB-mes | 0.08 | 2.4000 | DOC_ONLY |
| EBS snapshots | 3 | GB-mes | 0.05 | 0.1500 | HIPOTESIS |
| DynamoDB storage | 3 | GB-mes | 0.25 | 0.7500 | HIPOTESIS |
| DynamoDB lecturas | 2 | millón-RRU | 0.125 | 0.2500 | DOC_ONLY |
| DynamoDB escrituras | 0.6 | millón-WRU | 0.625 | 0.3750 | DOC_ONLY |
| DynamoDB PITR | 3 | GB-mes | 0.2 | 0.6000 | DOC_ONLY |
| DynamoDB backup | 3 | GB-mes | 0.1 | 0.3000 | DOC_ONLY |
| DynamoDB restore | 2 | GB-restaurado | 0.15 | 0.3000 | DOC_ONLY |
| CloudWatch ingesta de logs | 3 | GB | 0.5 | 1.5000 | DOC_ONLY |
| CloudWatch retención de logs | 4.5 | GB-mes | 0.03 | 0.1350 | HIPOTESIS |
| CloudWatch métricas personalizadas | 4.5 | métrica-mes | 0.3 | 1.3500 | DOC_ONLY |
| CloudWatch alarmas | 3 | alarma-mes | 0.1 | 0.3000 | DOC_ONLY |
| Secrets Manager | 3 | secreto-mes | 0.4 | 1.2000 | DOC_ONLY |
| S3 Terraform state | 3 | GB-mes | 0.023 | 0.0690 | DOC_ONLY |
| S3 solicitudes | 1 | lote | 0.015 | 0.0150 | SUPUESTO |
| Egreso a internet | 10 | GB | 0.09 | 0.9000 | SUPUESTO |
| Transferencia inter-AZ Redis | 10 | GB | 0.01 | 0.1000 | DOC_ONLY |
| Reserva CI/CD (no AWS) | 1 | lote | 2.00 | 2.0000 | SUPUESTO |
| Certificado CloudFront por defecto | 1 | certificado | 0 | 0.0000 | DISENO_DOC |
| CloudFront VPC Origin | 1 | funcionalidad | 0 | 0.0000 | DISENO_DOC |
| Gateway endpoints S3+DynamoDB | 2 | endpoint | 0 | 0.0000 | DISENO_DOC |
| Reserva prudencial CloudFront (no tarifaria) | 1 | reserva | 3.00 | 3.0000 | SUPUESTO |
| IPv4 pública ALB / EC2 | 0 | IP-hora | 0.005 | 0.0000 | DOC_ONLY (excluida por diseño) |
| Zona Route 53 | 0 | zona-mes | 0.5 | 0.0000 | DOC_ONLY (excluida por diseño) |
| NAT Gateway / interface endpoints | 0 | — | UNKNOWN | — | UNKNOWN (excluida por diseño) |

## 3. Costos residuales (runtime apagado)

Con EC2, ALB, Redis, egreso e ingesta detenidos, permanecen **6.8440 USD**: EBS, snapshots,
almacenamiento DynamoDB, PITR, backup, restauración, retención de logs, alarmas, Secrets Manager y
S3 state. Partidas no AWS (reserva CI/CD y reserva CloudFront) se excluyen del residual.
DynamoDB lecturas/escrituras se mantienen de forma conservadora aunque el tráfico se detenga.

## 4. Precios desconocidos declarados (nunca 0)

| SKU | Motivo |
|---|---|
| `NAT_GATEWAY_HORA` | No cotizado; excluido por diseño. Si el diseño cambia, recotizar |
| `INTERFACE_ENDPOINT_HORA` | No cotizado; excluido por diseño |
| `IMPUESTO_IVA` | Sin determinación fiscal; no se asume 0 % ni un monto |
| `EC2_T4G_MICRO_HORA_VERIFICADO` | Marcador de revalidación pendiente (Price List API) |

## 5. Separación financiera

| Concepto | Valor | Estado |
|---|---|---|
| Bruto 45 d (escenario C) | 84.0580 USD | Calculado con precios DOC_ONLY |
| Créditos declarados | 120.00 USD | **No verificado** en AWS |
| Créditos verificados | 0.00 USD | `NOT_VERIFIED` |
| Cobro neto | 84.0580 USD | `max(0, bruto − créditos verificados)` |
| Impuestos / IVA | `UNKNOWN` | No imputado |
| Desembolso personal | 0.0000 USD | Regla irrenunciable |
