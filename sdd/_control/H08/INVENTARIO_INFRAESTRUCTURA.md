# Inventario de infraestructura — I-140 (INF-140-06)

| Campo | Valor |
|---|---|
| Spec | `I-140-operacion` |
| Versión | 0.3.0 |
| Estado | **IMPLEMENTADO_LOCAL — NO DESPLEGADO** |
| Recursos consolidados | 17 (14 core, 3 auxiliares) |
| Capacidad del ASG | declarada 2 (min=desired=max) — **no implica instancias reales** |
| Existencia en AWS | **`AWS_NOT_VERIFIED`** |
| Costo observado | **`NOT_VERIFIED`** |

> Derivado de `infra/terraform/operacion/inventario.json` (I-120),
> `infra/terraform/respaldo/inventario_backup.json` (I-130), el catálogo `precios_us-east-2.json` y el
> módulo `computo`. Un recurso declarado en Terraform **no** equivale a uno existente en AWS.

## 1. Recursos

| Recurso | Tipo | Owner | Criticidad | USD/h | USD/mes | Estado declarado | AWS |
|---|---|---|---|---|---|---|---|
| `alarmas-cloudwatch` | `aws_cloudwatch_metric_alarm` | Operacion / SRE | **CORE_24X7** | 0.0011 | 0.8000 | DECLARADO_NO_CREADO | NOT_VERIFIED |
| `alb-interno` | `aws_lb` | Infraestructura | **CORE_24X7** | 0.0305 | 0.0000 | DECLARADO_NO_CREADO | NOT_VERIFIED |
| `asg-backend` | `aws_autoscaling_group` | Backend / Infraestructura | **CORE_24X7** | 0.0201 | 2.4000 | DECLARADO_NO_CREADO | NOT_VERIFIED |
| `cloudfront-distribucion` | `aws_cloudfront_distribution` | Infraestructura | **CORE_24X7** | 0.0000 | 0.0000 | DECLARADO_NO_CREADO | NOT_VERIFIED |
| `dynamodb-backup-on-demand` | `aws_dynamodb_backup` | Backend / Datos | **CORE_24X7** | 0.0004 | 0.3000 | OPCIONAL_NO_CREADO | NOT_VERIFIED |
| `dynamodb-backup-programado` | `aws_dynamodb_backup` | Backend / Datos | **CORE_24X7** | 0.0004 | 0.3000 | PRESUPUESTADO_NO_CREADO | NOT_VERIFIED |
| `dynamodb-core` | `aws_dynamodb_table` | Backend / Datos | **CORE_24X7** | 0.0018 | 1.3500 | DECLARADO_NO_CREADO | NOT_VERIFIED |
| `gateway-endpoints` | `aws_vpc_endpoint` | Infraestructura | **CORE_24X7** | 0.0000 | 0.0000 | DECLARADO_NO_CREADO | NOT_VERIFIED |
| `interface-endpoint-logs` | `aws_vpc_endpoint` | Infraestructura | **CORE_24X7** | 0.0200 | 0.0000 | DECLARADO_NO_CREADO | NOT_VERIFIED |
| `interface-endpoint-secretsmanager` | `aws_vpc_endpoint` | Infraestructura | **AUXILIAR_AUTORIZABLE** | 0.0200 | 0.0000 | DECLARADO_NO_CREADO | NOT_VERIFIED |
| `interface-endpoint-ssm` | `aws_vpc_endpoint` | Infraestructura | **AUXILIAR_AUTORIZABLE** | 0.0200 | 0.0000 | DECLARADO_NO_CREADO | NOT_VERIFIED |
| `interface-endpoint-ssmmessages` | `aws_vpc_endpoint` | Infraestructura | **AUXILIAR_AUTORIZABLE** | 0.0200 | 0.0000 | DECLARADO_NO_CREADO | NOT_VERIFIED |
| `log-group-backend` | `aws_cloudwatch_log_group` | Operacion / SRE | **CORE_24X7** | 0.0002 | 0.1350 | DECLARADO_NO_CREADO | NOT_VERIFIED |
| `red-vpc` | `aws_vpc` | Infraestructura | **CORE_24X7** | 0.0000 | 0.0000 | DECLARADO_NO_CREADO | NOT_VERIFIED |
| `redis-oss` | `aws_elasticache_replication_group` | Backend | **CORE_24X7** | 0.0160 | 0.0000 | DECLARADO_NO_CREADO | NOT_VERIFIED |
| `secretsmanager-secretos` | `aws_secretsmanager_secret` | Seguridad | **CORE_24X7** | 0.0000 | 0.0000 | BLOQUEADO_RFC004 | NOT_VERIFIED |
| `terraform-state-s3` | `aws_s3_bucket` | Infraestructura | **CORE_24X7** | 0.0001 | 0.0690 | DECLARADO_NO_CREADO | NOT_VERIFIED |

## 2. Consolidación sin duplicar cargos

| Nombre de origen | Consolidado en |
|---|---|
| `dynamodb-core` | `dynamodb-core` |
| `terraform-state` | `terraform-state-s3` |
| `secretos-bootstrap` | `secretsmanager-secretos` |

Cada cargo se contabiliza **una vez** por `(sku, base)` aunque provenga de dos fuentes; ambas fuentes
se conservan como trazabilidad. Cargos distintos del mismo recurso se mantienen.

| Verificación | Importe |
|---|---|
| PITR 3 GB × 0.20 | **0.6000 USD/mes** (no 1.20) |
| Almacenamiento DynamoDB 3 GB × 0.25 | 0.7500 USD/mes |
| `dynamodb-core` total | **1.3500 USD/mes** (no 1.95) |
| S3 state 3 GB × 0.023 | **0.0690 USD/mes** (no 0.138) |

Conflictos de cantidad detectados: **0**. Si dos fuentes declaran
cantidades distintas para el mismo SKU, se informa el conflicto y no se elige en silencio.

## 3. Unidades y estados de precio

| Unidad | Tratamiento |
|---|---|
| USD/hora | Se suma directamente |
| USD/mes | Se conserva en `costo_mes_usd` y se divide por 730 solo para la vista horaria |
| Consumo variable (RRU/WRU, ingreso, egreso, solicitudes) | **No** se suma ni se declara cero |
| Precio ausente | Anula el total del recurso y queda en `precios_desconocidos` |

Precios desconocidos: **0** · Owners pendientes: **0**.

## 4. Invariantes

1. El core 24×7 no se apaga ni se reduce a una instancia por conveniencia económica.
2. El ASG declara `min = desired = max = 2`; **no** se afirma que existan dos EC2 reales.
3. `AUXILIAR_AUTORIZABLE` solo cubre recursos de ventana declarados en I-120.
4. Todo estado cloud y el costo real permanecen `NOT_VERIFIED`.
5. Ningún precio desconocido se convierte en 0 USD y ningún cargo se factura dos veces.

## 5. Regeneración

```powershell
py -3.13 herramientas/verificar_operacion_i140.py inventario
```