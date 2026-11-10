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
