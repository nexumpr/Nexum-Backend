# I-060 — Matriz IAM, SCP y OIDC

| Campo | Valor |
|---|---|
| Estado | `IMPLEMENTADO_LOCAL`; permisos efectivos AWS `NOT_VERIFIED` |
| AWS/SCP | `AWS_SCP_NOT_VERIFIED`; no se consulta Organizations ni IAM Policy Simulator |

## Matriz principal → acción → recurso → condición → justificación → prueba

| Principal | Acción | Recurso | Condición | Justificación / validación |
|---|---|---|---|---|
| EC2 runtime | `dynamodb:GetItem`, `PutItem`, `UpdateItem`, `DeleteItem`, `ConditionCheckItem` | ARN exacto de tabla I-040 | `dynamodb:LeadingKeys` por namespaces H06/A-060 | CAS/idempotencia; prueba estructural |
| EC2 runtime | `dynamodb:Query` | Tabla e índices exactos habilitados | mismos namespaces; no `Scan` | Lectura particionada/GSI1; GSI2 sólo opt-in |
| EC2 runtime | `s3:GetObject` | **Ninguno configurado**; luego ARN exactos de artefactos | `aws:SecureTransport=true` | I-020 bootstrap; bucket/keys los fija I-080; sin `ListBucket`/write |
| EC2 runtime | `logs:CreateLogStream`, `logs:PutLogEvents` | **Ninguno configurado**; luego log groups exactos | `aws:SecureTransport=true` | Sin sink/agente aprobado; I-110 |
| EC2 runtime | Secrets Manager / SSM / KMS | Ninguno | RFC-004 PROPUESTO | No se aprueba opción S3 ni se expone secreto |
| EC2 runtime | `elasticache:Connect` | Ninguno | No aplica | I-050 usa password RBAC, no IAM auth |
| GitHub CI | Ninguna acción AWS | Ninguno | Trust exacto opcional; sin provider/environment actual | Workflows usan `contents:read`; no `id-token:write` |
| Terraform plan | Sin permission policy | Ninguno | Subject exacto `main` | I-070 backend/state/locking pendiente |
| Terraform apply | Sólo `iam:PassRole` del runtime, si se activa | ARN exacto runtime role | `iam:PassedToService=ec2.amazonaws.com`; environment `aws-deploy` | Sin Create/Update/Delete; resto depende de I-080/I-090 + Gate despliegue |

IAM no concede una acción `TransactWriteItems`: AWS autoriza la transacción por las acciones subyacentes de cada ítem (`PutItem`, `UpdateItem`, `DeleteItem`, `ConditionCheckItem`). Se excluyen `Scan`, `Batch*`, `DescribeTable`, `ListTables`, administración IAM y `dynamodb:*`.

## Namespaces `dynamodb:LeadingKeys`

- `USER#*` — particiones privadas. El rol EC2 es compartido y **no** reemplaza la validación de titularidad en Go con el `sub` del JWT (R-031).
- `EMAIL#*`, `SESSION#*`, `REFRESH#*`, `ONE_USE#*`, `LOOKUP#SESSION#*` — identidad y tokens.
- `PUBLIC#CATALOG`, `PUBLIC#CONTENT` — datos públicos acotados.
- `DUE#*` sólo si I-040 habilita GSI2 y existe worker autorizado.

## Trust OIDC (provider no verificado)

GitHub API verificó repo creado 2026-11-05, owner ID `349666511`, repo ID `1447636433`. `aud=sts.amazonaws.com`.

| Rol | Subject exacto | Estado |
|---|---|---|
| CI validation | `repo:nexumpr@349666511/Nexum-Backend@1447636433:environment:ci-validation` | Environment no configurado/protegido; rol off, sin policy |
| Terraform plan | `repo:nexumpr@349666511/Nexum-Backend@1447636433:ref:refs/heads/main` | Trust scaffold; permisos I-070 pendientes |
| Terraform apply | `repo:nexumpr@349666511/Nexum-Backend@1447636433:environment:aws-deploy` | Environment no configurado/protegido; rol off; PassRole acotado si se habilita |

No se crea/consulta IAM OIDC provider; ARN vacío por defecto significa que no se instancian roles. El subject de environment **no restringe por sí solo la rama**: GitHub debe configurar branch restrictions y required reviewers antes de que I-080/I-090 habiliten apply.

## SCP, MFA y estado

- SCP: **`AWS_SCP_NOT_VERIFIED`**; sin consultas a Organizations ni policy simulator. Un SCP Deny no se elude con IAM Allow.
- MFA humano, provider OIDC real y permisos efectivos de la cuenta: `NOT_VERIFIED`.
- AWS IAM resources: sólo Terraform local; no se crearon recursos (`AWS_NOT_RUN`).

## Secretos y Terraform state

- **Redis password:** I-050 usa `aws_elasticache_user.authentication_mode.passwords`; provider AWS 5.100.0 documenta que passwords quedan en texto plano en raw state. `sensitive` sólo redacciona salidas.
- `passwords_wo` está documentado en provider 6.41.0, requiere Terraform 1.11+, pero el repo usa >=1.4 y fija 5.100. No se actualiza I-050/provider aquí.
- RFC-004 sigue PROPUESTO: no se concede `GetSecretValue`, SSM ni KMS. El backend/locking Terraform es de I-070 y aún no existe.
- **Bloqueo de despliegue:** no pasar un secreto real a Terraform hasta resolver gestión de state/secret mediante decisión/RFC.

## Costos y dependencias

IAM/STS no añade cargo directo. No se añaden secretos, KMS, endpoints, CloudWatch resources ni recursos AWS. Artefactos S3 (I-080), logging (I-110), SSM de mantenimiento (I-120/I-140), backend state (I-070) y CI/CD (I-080/I-090) quedan sin permisos hasta definir ARN y costos.
