# Matriz de conectividad privada — I-000 (revisión 6, diseño E)

**Fecha:** 2026-11-06 · **Política:** API y DynamoDB 24/7; auxiliares por ventanas
**Topología:** 2 EC2 privadas en 2 AZ, sin NAT, gateway endpoints S3/DynamoDB
**Precio verificado:** interface endpoint `USE2-VpcEndpoint-Hours` = **0.01 USD/h por AZ**
**Resultado:** `VIABLE` con conectividad de **23.7400 USD** en 45 días

## 1. Regla que gobierna el diseño

Con 79.0430 USD de bruto verificado y 120 USD de créditos, el margen del 10 % sólo admite
**un interface endpoint permanente** (21.60 USD). Dos permanentes (43.20) dejan el presupuesto sin
cobertura (134.47 > 120). Por eso el diseño separa permanencia de temporalidad.

## 2. Clasificación por servicio

| Servicio | Origen de las solicitudes | Modo | Horas | Coste 45 d |
|---|---|---|---|---|
| `logs` | Backend Go y agente de CloudWatch | **PERMANENTE** | 1080 | 21.6000 |
| `secretsmanager` | Backend al arrancar/rotar (caché cifrada en EBS) | TEMPORAL | 24 | 0.4800 |
| `ssm` | Agente SSM y GitHub Actions | TEMPORAL | 24 | 0.4800 |
| `ssmmessages` | Agente SSM | TEMPORAL | 24 | 0.4800 |
| `monitoring` | Agente de CloudWatch | **NO REQUERIDO** | — | 0 |
| `ec2messages` | Agente SSM heredado | **NO REQUERIDO** | — | 0 |
| Gateway endpoints S3/DynamoDB | Backend y Terraform | PERMANENTE | — | 0 |

- **Métricas y alarmas sin `monitoring`:** se derivan del log ya ingerido con *metric filters* de
  CloudWatch (evaluación del lado del servicio). Se conserva GOV-10 sin `PutMetricData` desde la EC2.
- **`ec2messages`:** el agente SSM moderno opera por `ssmmessages`; si una versión concreta lo exige,
  se habilita dentro de la ventana con cargo a la reserva de contingencia (0.50 USD).
- **Secretos:** caché cifrada en EBS, sobrevive al reinicio; se refresca en ventana. Rotación y
  recuperación siguen disponibles activando la ventana (~2 min de creación del endpoint).

## 3. Coste de conectividad

| Concepto | Importe |
|---|---|
| Permanente (1 × 2 AZ × 1080 h) | 21.6000 |
| Temporales (3 × 2 AZ × 24 h) | 1.4400 |
| Procesamiento de datos (20 GB × 0.01) | 0.2000 |
| Reserva de contingencia (`ec2messages`, picos) | 0.5000 |
| **Total 45 días** | **23.7400** |

## 4. Escenarios comparados

| Escenario | Permanentes | Temporales | Coste conectividad |
|---|---|---|---|
| A — todos permanentes | logs, secretsmanager, ssm, ssmmessages | — | 86.4000 |
| B — sólo operación | logs | — | 21.6000 |
| C — operación + ventanas | logs | secretsmanager, ssm, ssmmessages | 23.0400 |
| D — una sola AZ | logs | 3 | 11.5200 |
| **E — optimizado** | **logs** | **secretsmanager, ssm, ssmmessages** | **23.0400** |

D no se adopta: un endpoint en una AZ es punto único de fallo para un componente crítico.

## 5. Presupuesto completo con diseño E

| Concepto | Importe |
|---|---|
| Bruto verificado (sin conectividad) | 79.0430 |
| Conectividad E | 23.7400 |
| **Bruto total** | **102.7830** |
| Margen 10 % → requerido | **113.0613** |
| Créditos verificados | 120.0000 |
| **Cobertura** | **TOTAL** · sobrante **6.9387** |
| Consumo estimado | 2.2841 USD/día |

## 6. Modos operativos

### Modo OPERACIÓN (permanente)

Activos: 2 EC2 Go, ALB, CloudFront + VPC Origin, DynamoDB, Redis OSS, EBS, `logs` endpoint, gateway
endpoints. Health checks y alarmas por metric filter operan sin intervención. No requiere acciones
manuales.

### Modo MANTENIMIENTO Y PRUEBAS (ventanas)

Se habilitan `secretsmanager`, `ssm` y `ssmmessages` (2 AZ). Usos: rotación de secretos, parches,
diagnóstico, despliegues, pruebas de recuperación. Al cerrar la ventana se verifica que API y
DynamoDB siguen disponibles, se conservan evidencias y se desactivan sólo los recursos temporales.
El ciclo es idempotente y auditable y se implementará en Terraform en las specs de automatización.

## 7. Riesgos declarados

1. Reinicio de instancia **sin** ventana: usa la caché de secretos; si la caché estuviera corrupta,
   la API no arranca hasta abrir ventana (~2 min).
2. Pérdida de logs bufferizados si una instancia falla antes de la ventana.
3. Métricas con granularidad derivada de logs (no hay `PutMetricData` continuo).
4. 8 ventanas × 3 h es una previsión; más ventanas elevan el coste a 0.06 USD/h por servicio.
5. Si se exige `monitoring` permanente, el presupuesto pasa a 134.47 y **no** hay cobertura.
