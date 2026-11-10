# RFC-002 — Ejecución real OpenCode V2 para H08-I / I-000

**Decisión técnica:** adaptar los perfiles de solo lectura H08-A a herramientas operativas con controles V2 y aprobación de riesgo (ASK). **Aprobación humana:** se solicita exclusivamente mediante frase expresa del instalador; archivo por sí solo no autoriza.

## Alcance
`opencode.json`, 8 agentes, AGENTS.md, memoria.md, estado/gates H07, pruebas de permisos y este registro. En I-000 se permite escribir artefactos FinOps, modificar documentación de control mediante RFC, ejecutar pruebas, Git branch/add/commit/push y GitHub CLI crear PR/consultar CI con ASK cuando corresponda. Nunca push directo a main, merge sin autorización del responsable ni publicar E endpoints.

## Restricciones esenciales
- `edit` ASK dentro de repo, excepto secretos, constitución y especificaciones `spec.md`/`plan.md` DENY; archivos de evidencia deben sanitizarse antes de Git.
- Shell ASK; comandos de lectura/test permitidos; AWS solo lista explícita de consultas read-only ASK; AWS mutante bloqueada; Terraform fmt/validate ASK, plan/init/apply/destroy DENY en I-000.
- Git/GitHub automatizados por agente con ASK para operaciones que mutan; merge requerirá aprobación humana explícita e CI verde.
- No ejecutar Docker/WSL ni comandos destructivos, ni leer `.env`, `.aws`, `.ssh`, `.tfvars`, `.tfstate`, PEM/KEY. Prohibido extraer secretos por shells indirectos.
- La autorización se extiende exclusivamente a I-000; tras su cierre la sesión debe detenerse. El perfil persistente deberá revisarse en nuevo Gate antes de cualquier I-010.

## Riesgos y validación
Los patrones de herramientas no son sandbox del sistema operativo. Prohibidas autorizaciones Always para acciones sensibles. Ejecutar `opencode debug agents`, comprobaciones de permisos y las pruebas locales/CI. La prueba local de políticas no equivale a verificación de runtime o nube.

## Enmienda — Lecturas AWS de solo lectura (2026-11-10)

| Campo | Valor |
|---|---|
| Autorización | `APRUEBO_GATE_OPENCODE_LECTURAS_AWS` (decisión humana; parche aprobado y aplicado el 2026-11-10) |
| Rama | `chore/h08-aws-readonly-cli` |
| Base | `main` `53c2536e1f3cabba2b8ad7fda6a62a77ab1ccdbd` |
| Alcance | `opencode.json`: corrección del nombre de comando y excepciones ASK de lectura |

### Defecto corregido

`aws servicequotas get-service-quota*` no coincide con el comando real de AWS CLI 2
(`aws service-quotas get-service-quota`). Al no coincidir, la llamada caía en la
denegación base `aws *` y devolvía `Permission denied: shell`. Se corrige el nombre y se
añade `list-service-quotas`.

Igualmente faltaban excepciones para lecturas necesarias del preflight
(`ec2 describe-instances` entre otras), que quedaban denegadas por la regla base.

### Reglas resultantes (verificadas con `opencode.cmd debug config`)

- **1 denegación base**: `aws *` → `deny`.
- **4 denegaciones de mutación**: `aws * create-*`, `update-*`, `delete-*`, `put-*` → `deny`.
- **33 excepciones `ask`**, todas de lectura: `sts get-caller-identity`, `freetier get-*`,
  `ce get-cost-and-usage`, `budgets describe-budgets`, `billing get-credits`,
  `service-quotas get-*|list-*`, `ec2 describe-{instances,vpcs,subnets,vpc-endpoints,
  security-groups,instance-types,instance-type-offerings}`, `autoscaling describe-*`,
  `elbv2 describe-*`, `elasticache describe-*`, `dynamodb list-tables|describe-table|
  describe-limits`, `s3api list-buckets`, `cloudfront list-distributions|list-vpc-origins`,
  `iam list/get-open-id-connect-provider*`, `iam get-role`, `organizations
  describe-organization`, `pricing get-products`, `iam simulate-principal-policy`.

### Garantías conservadas

`aws *` deny y las cuatro denegaciones de mutación siguen **antes** que cualquier
excepción; las 16 denegaciones de `policies` (Terraform apply/destroy/import/state/plan,
`git push origin main`, `--force`, `reset --hard`, `clean`, docker, wsl, rm) permanecen sin
cambios, igual que las denegaciones de edición de specs, `.env`, tfstate, tfvars, `.pem`,
`.key`, `.aws` y `.ssh`.

### Límite

Una autorización de lectura **no** concede escritura: toda operación mutante sigue
denegada y requiere su Gate propio (bootstrap, plan, apply). El Gate FinOps sigue siendo
independiente de estos permisos de herramienta.

## Enmienda — Plan local del bootstrap (2026-11-10)

| Campo | Valor |
|---|---|
| Autorización | `APRUEBO_GATE_OPENCODE_TERRAFORM_PLAN_LOCAL` |
| Base | `main` `6e03c8e4b1f05bef1b76b7f8dcdd521ae3042ece` |
| Alcance | una única excepción de plan para el bootstrap |

### Defecto corregido

El bloqueo era doble: `deny` en `permissions` (`terraform plan*`) y `deny` en `policies`
(`shell:terraform plan*`). OpenCode v2.2.5 evalúa ambas capas, de modo que levantar solo una
no habría permitido planificar. Además, `policies` solo admite `allow`/`deny`: un `ask` en
esa capa sería inerte.

### Reglas añadidas

- `permissions`: `terraform plan -input=false -out=bootstrap.tfplan` → **ask**.
- `policies`: `shell:terraform plan -input=false -out=bootstrap.tfplan` → **allow**.

El recurso es el **comando literal completo**, no `terraform plan*`. Quedan denegados
`plan -destroy`, `plan -target`, `apply`, `destroy`, `import`, `state`, `init -backend=true`,
`init -migrate-state` e `init -reconfigure`, además de todo el AWS mutante, Git destructivo y
acceso a secretos.

### Límite

Autoriza **planificar el bootstrap**. No autoriza aplicar, migrar el state ni tocar el
entorno de aplicación. El `apply` sigue necesitando su Gate propio.

## Solicitud — Lectura del plan del bootstrap (2026-11-10, NO HABILITADA)

| Campo | Valor |
|---|---|
| Estado | **PROPUESTA — pendiente de frase humana; reglas NO aplicadas** |
| Base | `main` `2958136a824a0d25e91ad87c788dac51a96e2991` |
| Alcance | inspeccionar un `.tfplan` local ya generado; **no** ejecuta Terraform contra AWS |
| Frase requerida | `APRUEBO_PERMISO_TERRAFORM_SHOW_PLAN` |

### Motivo

El plan entregado el 2026-11-10 era inválido (`applyable=false`, `errored=true`, 1 de 8
recursos) y su contenido solo podía determinarse leyendo el artefacto. La verificación se
realizó y quedó documentada en `GATE_H08I_BOOTSTRAP_APPLY.md`, pero **la regla de permiso no
se deja activa**: se retiró de `opencode.json` para no conservar una autorización que no fue
concedida por el responsable.

### Reglas exactas a habilitar solo tras la frase

- `permissions`: `terraform show -json bootstrap.tfplan` → **ask**.
- `policies`: `shell:terraform show -json bootstrap.tfplan` → **allow**.

`terraform show` sobre un fichero de plan es **solo lectura local**: no contacta AWS, no
modifica el state y no aplica cambios.

### Límite

No autoriza `terraform apply`, `destroy`, `import`, `state`, `init` con backend real ni
`plan -target/-destroy`. El `apply` del bootstrap exige su propio Gate y una excepción
posterior, también literal.

## Solicitud — Apply del plan exacto del bootstrap (2026-11-10, NO HABILITADA)

| Campo | Valor |
|---|---|
| Estado | **PROPUESTA — pendiente del Gate del plan y de frase humana** |
| Frase del Gate | `APRUEBO_APPLY_BOOTSTRAP_SHA256_57C195ADB49548B8490445D2A8222EB69A7599B19B10BD775B81E7DC1563874C` |
| Artefacto | `infra/terraform/bootstrap/bootstrap.tfplan` |

### Reglas exactas a habilitar solo después del Gate

- `permissions`: `{"action": "shell", "resource": "terraform apply bootstrap.tfplan", "effect": "ask"}`
- `policies`: `{"action": "tool.use", "resource": "shell:terraform apply bootstrap.tfplan", "effect": "allow"}`

Sin ambas capas la orden se rechaza. No se habilita `apply -auto-approve`, ni `-target`, ni
`destroy`, ni `init -migrate-state`.


