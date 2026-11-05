# H05 — Pasos exactos de verificación manual en Windows 11 / Warp / PowerShell 7

**SOLO CONSULTAS DE LECTURA AWS** cuando exista CLI, credencial válida y política que las permita. Estos comandos NO se ejecutan durante la instalación ZIP. No pegar salidas con cuentas, access keys, token, nombres sensibles ni correos aquí; enmascarar ID de cuenta.

## Comprobaciones locales de CLI

```powershell
Get-Command aws -ErrorAction SilentlyContinue
aws --version
Get-Command terraform -ErrorAction SilentlyContinue
Get-Command py -ErrorAction SilentlyContinue
```

## Identidad/plan y servicios disponibles (sin modificar AWS)

```powershell
$env:AWS_PROFILE = 'nexum'
$env:AWS_DEFAULT_REGION = 'us-east-2'
aws sts get-caller-identity --profile nexum --region us-east-2 --query '{Arn:Arn}' --output json
aws account get-account-information --profile nexum --region us-east-2
aws freetier get-account-plan-state --profile nexum --region us-east-1
```

Si algún comando `account` o `freetier` no existe en esa versión de CLI, o responde AccessDenied, **registrar READ_BLOCKED**; consultar consola AWS Billing/Free Tier con la identidad autorizada. No crear políticas alternativas ni cambiar plan por la prueba.

## Inventario opcional de solo lectura (puede estar denegado por SCP/IAM)

```powershell
aws ec2 describe-instances --profile nexum --region us-east-2 --query 'Reservations[].Instances[].{State:State.Name,Type:InstanceType}' --output json
aws elbv2 describe-load-balancers --profile nexum --region us-east-2 --query 'LoadBalancers[].{Type:Type,State:State.Code}' --output json
aws dynamodb list-tables --profile nexum --region us-east-2 --output json
aws elasticache describe-cache-clusters --profile nexum --region us-east-2 --output json
aws autoscaling describe-auto-scaling-groups --profile nexum --region us-east-2 --query 'AutoScalingGroups[].{Desired:DesiredCapacity,Min:MinSize,Max:MaxSize}' --output json
```

Estas consultas NO prueban permiso de creación/aplicación, solo lectura. También pueden revelar recursos existentes; no publicar salidas completas. Si la cuenta está limpia, respuestas vacías no prueban elegibilidad Free ni costo 0.

## Consola necesaria cuando la API no da el dato

1. AWS Console → **Billing and Cost Management** → Free Tier: plan, créditos y expiración; capturas con datos identificativos ocultos.
2. **AWS Pricing Calculator**: región US East (Ohio), escenarios A 45 días 24×7 y B uso temporal (2 instancias simultáneas durante ON); capturar precios brutos y créditos separados.
3. **Service quotas / Account Management**: verificar que ELB, EC2, ElastiCache/Redis, VPC, DynamoDB, ACM, CloudWatch y IAM/OIDC estén permitidos en el plan y SCP. No elevar el plan.
4. **AWS Budgets**: zero-spend alert/Free Tier usage alerts, no asumir que evita facturas.
5. Registrar cada precio/riesgo en `MATRIZ_COSTOS_Y_BLOQUEOS_H05.md` (anexo del paquete), vía RFC/ADR revisado; **no marcar I-GO aprobado** sin datos reales.

## Después de H05

H06 entrega contratos OpenAPI de endpoints, no integración Mobile. H07 publica SDD después de gates aprobados. H08 habilita Terraform/Go/CI/CD con autorizaciones humanas específicas.
