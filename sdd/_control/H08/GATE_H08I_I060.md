# Gate H08-I — I-060 `iam-seguridad` (implementación local)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/GATE_H08I_I060.md` |
| Fecha | 2026-11-07 |
| Spec | `sdd/02-infraestructura/I-060-iam-seguridad/` (v0.1.0 DRAFT; spec/plan no modificados) |
| Estado | **APRUEBO_GATE_I060_LOCAL REGISTRADO — IMPLEMENTADO_LOCAL PARCIAL (sin AWS)** |
| Main base | `8c314015dcd065f9188bde239be26ddf76cfc1c1` |
| Rama | `feat/h08i-i060-iam-seguridad` |
| Autorización | `APRUEBO_GATE_I060_LOCAL` (mensaje humano independiente, 2026-11-07) |

## 1. Alcance autorizado

Implementación local de identidades y políticas IAM, contratos trust OIDC, pruebas y registros de I-060, con un
único PR. No autoriza AWS, `terraform plan/apply/destroy`, despliegue, cambio de plan, aprobar RFC-004/H-01 ni
modificar otras especificaciones.

## 2. Implementación

| Requisito | Evidencia | Estado |
|---|---|---|
| INF-060-01 Runtime EC2 | Rol + instance profile enlazados a I-020; política DynamoDB exacta | IMPLEMENTADO_LOCAL |
| INF-060-01 CI/plan/apply | Trust OIDC separado; roles sin policies, condicionales a provider verificado | DECLARADO_LOCAL / NO ACTIVOS |
| INF-060-02 SCP | Sin consulta AWS/Organizations | `AWS_SCP_NOT_VERIFIED` |
| INF-060-03 Secretos | Sin permisos Secret Manager/SSM/KMS ni valores | `BLOCKED_RFC004` + riesgo state I-050 |
| INF-060-04 Least privilege | Tabla/índices y LeadingKeys; S3/logs sólo con ARN exactos | IMPLEMENTADO_LOCAL |
| INF-060-05 OIDC | Audiencia y subjects inmutables exactos; provider no creado ni verificado | DECLARADO_LOCAL / NOT_VERIFIED |
| INF-060-06 Auditoría | Matrices IAM/SCP, OIDC, secretos/rotación, riesgos | IMPLEMENTADO_LOCAL |

## 3. Permisos principales

Detalle en `sdd/_control/H08/MATRIZ_IAM_SCP.md`.

| Principal | Acciones/recursos | Límite |
|---|---|---|
| EC2 runtime | Get/Put/Update/Delete/ConditionCheck sobre tabla I-040; Query tabla + índices exactos | `dynamodb:LeadingKeys` restringe namespaces; sin Scan, Batch ni administración |
| EC2 → S3 | GetObject sobre ARN exactos | Lista vacía hasta I-080 fije bucket/keys; no ListBucket |
| EC2 → Logs | CreateLogStream/PutLogEvents sobre grupos exactos | Lista vacía hasta I-110 defina sink |
| EC2 → Redis | Ninguno | I-050 usa password RBAC, no IAM authentication |
| EC2 → secretos | Ninguno | RFC-004 PROPUESTO; no se aprueba su opción S3 ni Secrets Manager |
| GitHub CI/plan | Trust-only, sin policies | Sin provider ARN; I-070/I-080 deben definir state/jobs/permisos |
| GitHub apply | Trust environment exacto + solo PassRole al runtime y solo EC2, si se activa | Disabled por defecto; environment `aws-deploy` no existe/no está protegido/verificado |

## 4. Hallazgos/bloqueos no resueltos

### SCP y OIDC

- **SCP = `AWS_SCP_NOT_VERIFIED`**: no se consultó AWS/Organizations. Un IAM Allow no supera un SCP Deny.
- El OIDC provider no se consulta ni crea aquí; ARN vacío por defecto significa **ningún role OIDC instanciado**.
- Repositorio verificado por GitHub API: owner ID `349666511`, repo ID `1447636433`, creado 2026-11-05. Se usan los
  subjects inmutables posteriores al 2026-07-15, no el formato mutable anterior.
- GitHub no tiene environments configurados al preflight; `aws-deploy` protection queda `NOT_VERIFIED`.
- Los workflows actuales no solicitan `id-token: write` y no usan AWS.

### RFC-004 / state Redis

El provider AWS **5.100.0** documenta que `aws_elasticache_user.authentication_mode.passwords` se almacena en
texto plano en el raw Terraform state. `sensitive=true` sólo redacta ciertas salidas y **no evita state/plan**.
`passwords_wo` aparece en provider 6.41.0 y requiere Terraform 1.11+, mientras el repo permite >=1.4 y fija 5.100.
No se cambia I-050/provider/Terraform en este Gate. Bloqueo de despliegue: no pasar una contraseña real a Terraform
hasta decisión/RFC para resolver state/secrets. **RFC-004 continúa PROPUESTO**.

### Otras dependencias

- I-070: backend/state/locking y permisos de plan; no definidos.
- I-080/I-090: jobs, matrices de acciones IaC y GitHub OIDC; no definidos.
- I-080: bucket/ARN de artefactos de I-020; queda vacía la lista `s3:GetObject`.
- I-110: log groups/sink; queda vacía la lista Logs.
- I-050: Gate menciona `elasticache:Connect`, pero el recurso actual usa password RBAC; I-060 no concede esa acción.
- MFA de identidades humanas y permisos/SCP efectivos: no verificados.

## 5. Evidencia local

`terraform fmt`, `init -backend=false` con `TF_DATA_DIR` externo, `validate`, pruebas de IAM, regresión Go,
gosec/govulncheck/gitleaks, SDD y SHA-256 se registrarán tras la ejecución. No se ejecuta `plan`, no se muta AWS.
CI Terraform = `NOT_IMPLEMENTED` (dueño I-080).

## 6. Cierre

Este Gate sólo permite implementación local. El merge requiere aprobación humana separada ligada a PR+HEAD exactos
(A-110/R-004); despliegue requiere Gate independiente. No declarar `AWS_DEPLOYED`, `AWS_VERIFIED`,
`PRODUCTION_READY` ni `FINOPS_GARANTIZADO`.
