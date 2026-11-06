# Matriz de conectividad privada — I-000 (revisión 5)

**Fecha:** 2026-11-06 · **Topología:** 2 EC2 privadas en 2 AZ, **sin NAT**, gateway endpoints S3/DynamoDB
**Módulo:** `infra/finops/conectividad.py` · **Pruebas:** `tests/infra/finops/test_conectividad.py`
**Precio verificado:** interface endpoint `USE2-VpcEndpoint-Hours` = **0.01 USD/h por AZ**
**Resultado:** `CONECTIVIDAD_PRIVADA_NO_CUBIERTA` (bloqueo real)

## 1. Dependencia por dependencia

| Dependencia del Backend | ¿Necesita red? | Mecanismo | Coste 45 d (2 AZ) | Fuente |
|---|---|---|---|---|
| DynamoDB | Sí | **Gateway endpoint** | 0 | privatelink/pricing |
| S3 (artefactos, respaldos, terraform state) | Sí | **Gateway endpoint** | 0 | privatelink/pricing |
| Secrets Manager (claves JWT, credenciales) | Sí | **Interface endpoint** | 21.6000 | Secrets Manager + PrivateLink |
| CloudWatch Logs (observabilidad) | Sí | **Interface endpoint** | 21.6000 | CloudWatch Logs interface VPC |
| CloudWatch metrics (métricas del agente) | Sí | **Interface endpoint** | 21.6000 | CloudWatch interface VPC |
| SSM (parches, comandos, despliegue) | Sí | **Interface endpoint** | 21.6000 | Systems Manager PrivateLink |
| SSM Messages | Sí | **Interface endpoint** | 21.6000 | Systems Manager PrivateLink |
| EC2 Messages | Sí | **Interface endpoint** | 21.6000 | Systems Manager PrivateLink |
| Credenciales de instancia (IAM role) | No | IMDS link-local | 0 | EC2 user guide |
| Sincronización horaria | No | Amazon Time Sync `169.254.169.123` | 0 | EC2 user guide |
| Health checks del ALB / CloudFront | No | Entrada por VPC Origins | 0 | CloudFront VPC origins |

Los **gateway endpoints no dan conectividad a Secrets Manager, CloudWatch ni SSM**. No existe NAT en
el diseño y NAT tampoco es gratuito. Las interface endpoints se cobran **por hora y por AZ**, no por
uso.

## 2. Recotización del presupuesto completo

| Escenario | Endpoints | ENI | Coste conectividad | Bruto total | Con margen 10 % | Créditos | Déficit |
|---|---|---|---|---|---|---|---|
| Mínimo (renunciando a SSM en línea) | Secrets Manager + Logs | 4 | 43.2000 | 122.2430 | **134.4673** | 120 | **14.4673** |
| Completo según SDD | 6 servicios | 12 | 129.6000 | 213.6430 | **235.0073** | 120 | **115.0073** |

Fórmula: `servicios × 2 AZ × 1080 h × 0.01 USD`.

## 3. Por qué no se reduce alcance unilateralmente

- Renunciar a **SSM** cambia la administración y el despliegue aprobados (I-120/I-130).
- Renunciar a **CloudWatch** incumple observabilidad (GOV-10, I-090).
- Sustituir **Secrets Manager** por secretos embebidos viola la constitución (R-019, GOV-09).
- Añadir **NAT** implica costo por hora y por GB, también fuera de cobertura.

Cualquiera de esas vías exige **RFC de cambio de alcance** con aprobación humana; no se aplica aquí.

## 4. Alternativa sin costo (para RFC, no aplicada)

| Necesidad | Alternativa sin coste | Coste de cumplimiento |
|---|---|---|
| Artefactos y despliegue | S3 gateway endpoint + AMI inmutable reconstruida en CI (sin SSM en la instancia) | Cambia el modelo de parches/despliegue |
| Logs | Escritura a S3 vía gateway endpoint con ciclo de vida | Cambia la observabilidad de CloudWatch a S3 |
| Métricas | Métricas básicas de hipervisor (gratuitas, sin agente) | Pierde métricas de aplicación |
| Secretos | Inyección en el arranque desde un almacén cifrado en S3 con política restringida | Cambia la gestión de secretos aprobada |
| Hora / credenciales | Link-local (ya contemplado) | Ninguno |

Esta alternativa **elimina el costo de conectividad** pero modifica requisitos aprobados: sólo es
viable con RFC y firma humana.

## 5. Estado

`CONECTIVIDAD_PRIVADA_NO_CUBIERTA` — bloqueo real. Ningún recurso creado; ningún endpoint
aprovisionado; Terraform no ejecutado.
