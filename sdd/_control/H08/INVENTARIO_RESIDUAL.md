# Inventario de recursos y costo residual — I-120

| Campo | Valor |
|---|---|
| Spec | `sdd/02-infraestructura/I-120-encendido-apagado` |
| Versión | 0.1.0 |
| Estado | **IMPLEMENTADO_LOCAL — NO DESPLEGADO** |
| Fuente de precios | `infra/finops/precios_us-east-2.json` (22 SKU `VERIFIED_AWS`) |
| Evidencia de facturación | **`NOT_VERIFIED`** — no se consultó Cost Explorer ni Billing |

> El costo de este documento se calcula con `Decimal` (R-030) desde el catálogo de I-000 y el
> inventario declarativo. **No** es costo observado: no hay recursos AWS creados ni facturación
> verificada, y no se declara USD 0 en ningún caso.

## 1. Qué significa «residual» aquí

Costo que **persiste por existencia**, es decir, el que no desaparece porque se cierre una ventana de
mantenimiento. Se distinguen tres capas:

| Capa | Qué incluye | ¿Se suma al residual? |
|---|---|---|
| `residente` | Recursos que el código declara (`DECLARADO_NO_CREADO`) | Sí |
| `presupuestado` | Partidas de I-000 que el código no crea hoy (`PRESUPUESTADO_NO_CREADO`) | Sí, separado |
| `no computable` | Costo bloqueado (`BLOQUEADO_RFC004`) | **No** |
| `variable` | RRU/WRU, ingesta, egreso CloudFront, solicitudes S3, inter-AZ Redis | **No** (no se declaran cero) |

El total de 45 días del proyecto **sigue siendo el de I-000** (`102.7830 USD` bruto verificado con
margen `113.0613 USD` sobre `120.0000 USD`). Este documento no lo reemplaza: solo aísla lo que no se
puede apagar.

## 2. Inventario clasificado

### 2.1 Core — 24×7, intocable

| Recurso | Tipo | Base de costo | USD/hora |
|---|---|---|---|
| `asg-backend` (2 × t4g.micro + EBS gp3 30 GB) | `aws_autoscaling_group` | EC2 hora + EBS GB-mes | 0.0201 |
| `alb-interno` (+ 1 LCU) | `aws_lb` | ALB hora + LCU hora | 0.0305 |
| `cloudfront-distribucion` | `aws_cloudfront_distribution` | por uso | 0.0000 |
| `redis-oss` (1 nodo t4g.micro) | `aws_elasticache_replication_group` | nodo hora | 0.0160 |
| `dynamodb-core` (storage + PITR) | `aws_dynamodb_table` | GB-mes | 0.0018 |
| `terraform-state-s3` | `aws_s3_bucket` | GB-mes | 0.0001 |
| `log-group-backend` (retención 7 d) | `aws_cloudwatch_log_group` | GB-mes | 0.0002 |
| `alarmas-cloudwatch` (8) | `aws_cloudwatch_metric_alarm` | alarma-mes | 0.0011 |
| `interface-endpoint-logs` (2 AZ) | `aws_vpc_endpoint` | endpoint hora | 0.0200 |
| `red-vpc` (VPC, subredes, IGW, NACL, SG) | `aws_vpc` | sin cargo | 0.0000 |
| `gateway-endpoints` (S3, DynamoDB) | `aws_vpc_endpoint` | sin cargo | 0.0000 |
| `dynamodb-backup-programado` | `aws_dynamodb_backup` | GB-mes (**presupuestado**) | 0.0004 |
| `secretsmanager-secretos` | `aws_secretsmanager_secret` | **no computable** | — |

### 2.2 Auxiliares — suspendibles en ventana

| Recurso | USD/hora | USD/45 días si estuviera siempre activo |
|---|---|---|
| `interface-endpoint-secretsmanager` (2 AZ) | 0.0200 | 21.6000 |
| `interface-endpoint-ssm` (2 AZ) | 0.0200 | 21.6000 |
| `interface-endpoint-ssmmessages` (2 AZ) | 0.0200 | 21.6000 |
| **Total auxiliar** | **0.0600** | **64.8000** |

La política vigente (D6 de I-000) los mantiene cerrados y los abre solo en ventanas: ese uso real es
`1.4400 USD` por 24 h de ventana, según I-000, no 64.8000.

## 2.3 Fijos frente a variables

| Naturaleza | Recursos | Tratamiento |
|---|---|---|
| **Fijo por existencia** | 2 EC2, ALB, Redis, endpoints de logs, EBS, almacenamiento DynamoDB y PITR, S3 de state, almacenamiento de logs, 8 alarmas | Se suman al residual con precio `VERIFIED_AWS` |
| **Variable por uso** | egreso CloudFront, RRU/WRU de DynamoDB, ingesta de logs, solicitudes S3, inter-AZ de Redis | **No** se suman y **no** se declaran cero |
| **Sin cargo por existencia** | VPC, subredes, IGW, NACL, SG, gateway endpoints, certificado CloudFront, VPC Origins | Se declaran `0.0000` con fuente oficial |
| **No computable** | `secretsmanager` (`BLOQUEADO_RFC004`) | Se excluye y se nombra |
| **Presupuestado sin crear** | backup programado de DynamoDB (I-130 `NOT_AUTHORIZED`) | Se muestra separado |

## 2.4 Costo neto, cr?ditos y margen

| Campo | Estado |
|---|---|
| Estimaci?n bruta (residual fijo) | `97.4412 USD / 45 d` |
| Margen de seguridad 10 % | `107.1853 USD / 45 d` |
| Cr?ditos aplicables | **`NOT_VERIFIED`** (no hay saldo verificado en esta corrida) |
| Costo neto | **desconocido** (`neto_con_creditos_usd = null`) |
| Servicios no verificados | CloudFront egreso, DynamoDB requests, ingesta, S3 requests, inter-AZ, IVA, `secretsmanager` |

El operador expone `creditos_aplicables_usd = null` y `neto_con_creditos_usd = null`
mientras no exista evidencia autorizada: no se inventa el saldo ni el gasto.

El control diario (`verificar_finops_diario.py`) solo admite cifras con bandera de
verificación explícita: `creditos_usd`, `consumo_usd`, `forecast_usd` y `plan_activo`
deben declararse verificados, el plan debe ser `FREE`, no se admiten negativos, `NaN`
ni infinitos, y créditos caducados bloquean. Aun cumpliendo todo, la salida declara
`autoriza_ejecucion_real = false`: una evidencia local no es prueba de facturación.

## 3. Residual calculado

| Concepto | USD/hora | USD/45 días (1080 h) |
|---|---|---|
| Residente (código declarado) | 0.0898 | **96.9973** |
| Presupuestado no creado (backup DynamoDB) | 0.0004 | 0.4438 |
| **Total residual** | **0.0902** | **97.4412** |

Notas obligatorias:

- `secretsmanager-secretos` **no** se suma: `BLOQUEADO_RFC004`.
- Las partidas variables no están incluidas ni se declaran cero.
- La convención de mes es 730 h (24 × 365 / 12), documentada en el operador.
- Cierre de ventana auxiliar ahorra `0.0600 USD/h`; con ventana cerrada el ahorro real de 45 días es
  el de I-000 (`1.4400 USD`), no el máximo teórico.

## 4. Comparación con I-000 (control de contradicción)

| Referencia | USD/45 d | Estado |
|---|---|---|
| I-000 bruto verificado C | 84.0430 | `VERIFIED_AWS` (precios), uso no medido |
| I-000 conectividad (logs permanente + ventanas) | 23.7400 | Modelado |
| I-000 total con margen 10 % | 113.0613 | Sobre 120.0000 de créditos verificados |
| Residual de este documento | 97.4412 | ≤ total de I-000 |

El residual es menor que el total de I-000 porque excluye partidas variables y reservas de modelado.
Esa diferencia **no** es un ahorro: es alcance distinto. Cualquier afirmación de costo cero o de
créditos intactos sigue siendo `NOT_VERIFIED`.

## 5. Costo que permanece tras cerrar una ventana

| Recurso | Se apaga | Sigue facturando |
|---|---|---|
| Endpoints `secretsmanager`, `ssm`, `ssmmessages` | Sí (en ventana) | No, tras eliminar y recrear |
| 2 × EC2 Go | No | Sí |
| ALB + LCU | No | Sí |
| CloudFront | No | Solo por uso |
| Redis OSS | No | Sí |
| DynamoDB (storage + PITR) | No | Sí |
| EBS de las instancias | No | Sí |
| Logs (almacenamiento + ingesta) | No | Sí (ingesta por uso) |
| State S3 | No | Sí (mínimo) |
| Alarmas (8) | No | Sí |

## 6. Lo que este documento no afirma

- No afirma que el costo real sea cero.
- No afirma que los créditos estén intactos: `AWS_CREDITS = NOT_VERIFIED` en esta corrida.
- No sustituye el análisis de 45 días de I-000 ni el Gate FinOps.
- No existe costo observado: `AWS_COST_REAL = NOT_VERIFIED`.

## 7. Evidencia

`py -3.13 herramientas/nexum_operacion.py estado` reproduce esta tabla de forma determinista. Las 54
pruebas de `tests/infra/operacion` verifican aritmética exacta, exclusiones y que el residual nunca se
presenta como cero ni supera el presupuesto de I-000.
