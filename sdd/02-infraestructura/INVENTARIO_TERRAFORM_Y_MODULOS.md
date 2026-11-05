# H05 — Inventario de recursos, Terraform future modules y dependencias

**Documento de diseño solamente. No crea `.tf` durante H05.** Todos los recursos se etiquetarán Project=Nexum, Environment, Owner=Backend, CostCenter, ManagedBy=Terraform. Ids/ARNs se generan en apply futuro y no se inventan.

| Grupo IaC futuro | Recurso candidato / configuración mínima de contrato | Dependencias / condición | Requisito |
|---|---|---|---|
| state bootstrap | S3 bucket de state privado, versionado, cifrado y locking compatible, sin public access | Aprobación IAM/SCP, tarifa S3, rol autorizador | I-070/I-130 |
| network | VPC, subnets AZ1/AZ2, route tables, SG ALB/Go/Redis, endpoints si precio permite | Plan Free, dos AZ, cotizar egress/NAT/IPv4 | I-010/I-000 |
| compute | Launch Template, roles EC2, ASG desired=2/min=2/max=2 ON, target attachment, EBS protegido | dos EC2 reales, binario SHA, IAM, precio doble | I-020 |
| load-balancer | ALB, target group, HTTPS listener443, ACM cert y DNS si disponible | 2 healthy + dominio + tarifa ALB + permisos SCP | I-030 |
| db | Dynamo table PK/SK, GSI1, GSI2 según justificación, TTL, tags, encryption, PITR/backup | model E001-E075 e IAM; billing backup | I-040 |
| redis | ElastiCache Redis OSS con TLS/Auth y SG o tercer nodo Redis dedicado | costo/SCP y requisito Redis real; no mock | I-050 |
| identity | instance role, OIDC CI role, deploy role, least privilege and trust conditions | cuenta/namescope repo, restricciones SCP | I-060 |
| CI/CD | workflows GitHub Actions, branch/env approvals, artifacts SHA, releases, rollback | solo H08 luego H07, sin credenciales fijas | I-080/I-090 |
| monitoring | Log groups retention, alarms, CloudWatch/OTEL endpoints, budget monitor | cotizar logs/alarms, PII no logging | I-110 |
| backup | Dynamo backup/PITR, estado remoto y restore test | retención/privacy y costo | I-130 |
| runtime toggle | ASG ON desired2 / OFF desired0 + delete runtime separado, data protected | costo residual; autorización para destruir | I-120/I-140 |
| API contract | HTTPS URL, OpenAPI artifact con SHA, smoke real con Dynamo y 2 nodes | OpenAPI specs H06 y release gates H08 | I-150 |

### Parámetros de módulo a decidir sin inventar recursos

- `aws_region=us-east-2` por contexto; `environment`, `project`, `vpc_cidr`, `subnet_cidrs_az1_az2`, `domain_name`, `certificate_arn`, `instance_type`, `desired_capacity_on=2`, `instance_profile`, `ddb_billing_mode`, `ddb_gsi_definitions`, `ddb_backup_policy`, `redis_flavour`, `redis_capacity`, `log_retention_days`, `artifact_sha256`, `enable_ephemeral_runtime`, `budget_thresholds`.
- Secrets/keys se obtienen mediante canal seguro, no en `terraform.tfvars` versionados; use sensitive outputs para redacción pero recuerde que state puede contener valores sensibles.
- Outputs de IaC restringidos: `api_https_url`, `target_group_arn` (solo en entorno privado), `instance_ids` en evidencia protegida, `ddb_table_name`, `redis_endpoint_private`, `artifact_digest`; no imprimir credenciales ni access tokens.
- Pre-apply: precios/SCP PASS, H05/H06/H07 aprobados, release autorizado, `plan` revisado, backups y rollback establecidos. Cualquier cambio de arquitectura se tramita por RFC.
