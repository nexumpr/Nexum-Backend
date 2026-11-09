# Inventario de infraestructura — I-140 (INF-140-06)

| Campo | Valor |
|---|---|
| Spec | `I-140-operacion` |
| Versión | 0.1.0 |
| Estado | **IMPLEMENTADO_LOCAL — NO DESPLEGADO** |
| Recursos | 20 (17 core, 3 auxiliares) |
| Existencia en AWS | **`AWS_NOT_VERIFIED`** |

> Inventario derivado de `infra/terraform/operacion/inventario.json` (I-120),
> `infra/terraform/respaldo/inventario_backup.json` (I-130) y el catálogo de precios de I-000.
> Un recurso declarado en Terraform **no** equivale a un recurso existente en AWS.

| Recurso | Tipo | Fuente | Owner | Criticidad | Costo USD/h | Estado declarado | AWS |
|---|---|---|---|---|---|---|---|
| `asg-backend` | `aws_autoscaling_group` | infra/terraform/operacion/inventario.json (I-120) | Backend / Infraestructura | **CORE_24X7** | 0.0168 | DECLARADO_NO_CREADO | NOT_VERIFIED |
| `alb-interno` | `aws_lb` | infra/terraform/operacion/inventario.json (I-120) | Infraestructura | **CORE_24X7** | 0.0305 | DECLARADO_NO_CREADO | NOT_VERIFIED |
| `cloudfront-distribucion` | `aws_cloudfront_distribution` | infra/terraform/operacion/inventario.json (I-120) | Infraestructura | **CORE_24X7** | 0.0000 | DECLARADO_NO_CREADO | NOT_VERIFIED |
| `redis-oss` | `aws_elasticache_replication_group` | infra/terraform/operacion/inventario.json (I-120) | Backend | **CORE_24X7** | 0.0160 | DECLARADO_NO_CREADO | NOT_VERIFIED |
| `dynamodb-core` | `aws_dynamodb_table` | infra/terraform/operacion/inventario.json (I-120) | Backend / Datos | **CORE_24X7** | 0.0000 | DECLARADO_NO_CREADO | NOT_VERIFIED |
| `terraform-state-s3` | `aws_s3_bucket` | infra/terraform/operacion/inventario.json (I-120) | Infraestructura | **CORE_24X7** | 0.0000 | DECLARADO_NO_CREADO | NOT_VERIFIED |
| `log-group-backend` | `aws_cloudwatch_log_group` | infra/terraform/operacion/inventario.json (I-120) | Operacion / SRE | **CORE_24X7** | 0.0000 | DECLARADO_NO_CREADO | NOT_VERIFIED |
| `alarmas-cloudwatch` | `aws_cloudwatch_metric_alarm` | infra/terraform/operacion/inventario.json (I-120) | Operacion / SRE | **CORE_24X7** | 0.0000 | DECLARADO_NO_CREADO | NOT_VERIFIED |
| `interface-endpoint-logs` | `aws_vpc_endpoint` | infra/terraform/operacion/inventario.json (I-120) | Infraestructura | **CORE_24X7** | 0.0200 | DECLARADO_NO_CREADO | NOT_VERIFIED |
| `red-vpc` | `aws_vpc` | infra/terraform/operacion/inventario.json (I-120) | Infraestructura | **CORE_24X7** | 0.0000 | DECLARADO_NO_CREADO | NOT_VERIFIED |
| `gateway-endpoints` | `aws_vpc_endpoint` | infra/terraform/operacion/inventario.json (I-120) | Infraestructura | **CORE_24X7** | 0.0000 | DECLARADO_NO_CREADO | NOT_VERIFIED |
| `interface-endpoint-secretsmanager` | `aws_vpc_endpoint` | infra/terraform/operacion/inventario.json (I-120) | Infraestructura | **AUXILIAR_AUTORIZABLE** | 0.0200 | DECLARADO_NO_CREADO | NOT_VERIFIED |
| `interface-endpoint-ssm` | `aws_vpc_endpoint` | infra/terraform/operacion/inventario.json (I-120) | Infraestructura | **AUXILIAR_AUTORIZABLE** | 0.0200 | DECLARADO_NO_CREADO | NOT_VERIFIED |
| `interface-endpoint-ssmmessages` | `aws_vpc_endpoint` | infra/terraform/operacion/inventario.json (I-120) | Infraestructura | **AUXILIAR_AUTORIZABLE** | 0.0200 | DECLARADO_NO_CREADO | NOT_VERIFIED |
| `dynamodb-backup-programado` | `aws_dynamodb_backup` | infra/terraform/operacion/inventario.json (I-120) | Backend / Datos | **CORE_24X7** | 0.0000 | PRESUPUESTADO_NO_CREADO | NOT_VERIFIED |
| `secretsmanager-secretos` | `aws_secretsmanager_secret` | infra/terraform/operacion/inventario.json (I-120) | Seguridad | **CORE_24X7** | 0.0000 | BLOQUEADO_RFC004 | NOT_VERIFIED |
| `dynamodb-core` | `aws_dynamodb_table` | infra/terraform/respaldo/inventario_backup.json (I-130) | Backend / Datos | **CORE_24X7** | 0.0000 | DECLARADO_NO_CREADO | NOT_VERIFIED |
| `dynamodb-backup-on-demand` | `aws_dynamodb_backup` | infra/terraform/respaldo/inventario_backup.json (I-130) | Backend / Datos | **CORE_24X7** | 0.0000 | OPCIONAL_NO_CREADO | NOT_VERIFIED |
| `terraform-state` | `aws_s3_bucket` | infra/terraform/respaldo/inventario_backup.json (I-130) | Infraestructura | **CORE_24X7** | 0.0000 | DECLARADO_NO_CREADO | NOT_VERIFIED |
| `secretos-bootstrap` | `aws_secretsmanager_secret` | infra/terraform/respaldo/inventario_backup.json (I-130) | Infraestructura | **CORE_24X7** | 0.0000 | BLOQUEADO_RFC004 | NOT_VERIFIED |

## Invariantes

1. El core 24×7 (2 EC2, ASG, ALB, CloudFront, Redis, DynamoDB, red, IAM, state, CloudWatch) no se
   apaga ni se reduce a una instancia por conveniencia económica.
2. `AUXILIAR_AUTORIZABLE` solo cubre recursos de ventana declarados en I-120.
3. Todo estado cloud permanece `AWS_NOT_VERIFIED`.
4. Los costos son estimación por existencia desde precios verificados del catálogo;
   `AWS_COST_REAL` sigue `NOT_VERIFIED`.

## Regeneración

```powershell
py -3.13 herramientas/verificar_operacion_i140.py inventario
```