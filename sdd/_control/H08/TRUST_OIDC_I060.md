# I-060 — Trust OIDC de GitHub Actions

| Campo | Valor |
|---|---|
| Repo | `nexumpr/Nexum-Backend` |
| Creación | GitHub API informó `2026-11-05`; owner ID `349666511`, repo ID `1447636433` |
| Fuente | GitHub API de lectura + [OIDC AWS oficial](https://docs.github.com/en/actions/how-tos/secure-your-work/security-harden-deployments/oidc-in-aws) |
| Estado | **Trust declarativo local; provider/roles no verificados ni activos** |

## Subjects exactos

El repo se creó después del 2026-07-15, por lo que se usan los immutable subject claims documentados por GitHub.
La audiencia del proveedor debe ser `sts.amazonaws.com`.

| Identidad | `sub` exacto | Función / límite |
|---|---|---|
| CI validation | `repo:nexumpr@349666511/Nexum-Backend@1447636433:environment:ci-validation` | Sin AWS policy; environment inexistente/protection no verificada |
| Terraform plan | `repo:nexumpr@349666511/Nexum-Backend@1447636433:ref:refs/heads/main` | Trust-only; backend y permisos de lectura corresponden a I-070/I-080 |
| Terraform apply | `repo:nexumpr@349666511/Nexum-Backend@1447636433:environment:aws-deploy` | Trust-only + `iam:PassRole` al runtime, sólo EC2; environment no existe/protection no verificada |

## Condiciones y controles

- `StringEquals` para `aud` y `sub`; no `StringLike`, wildcards, pull request subjects ni credenciales estáticas.
- La variable `github_oidc_provider_arn` está vacía por defecto. I-060 no crea un provider que podría duplicar uno
  existente ni consulta IAM para comprobarlo; **ningún** role OIDC se instancia mientras el ARN no se verifique.
- Los workflows existentes `.github/workflows/go-backend.yml` y `sdd-validacion.yml` sólo tienen `contents:read`;
  ninguno declara `id-token:write`, `configure-aws-credentials` ni acceso AWS.
- Los roles CI/plan/apply tienen **cero permission policies**, salvo `iam:PassRole` exacto del role apply al role
  runtime con `iam:PassedToService=ec2.amazonaws.com`. La política de plan depende del backend/locking de I-070;
  el inventario de acciones apply y workflows pertenece a I-080/I-090.
- GitHub API no devolvió environments configurados. Antes de habilitar apply se requiere crear/proteger
  `aws-deploy` con required reviewers y restricciones de branch. Un subject de environment **no** restringe la
  rama por sí solo.
- La protección de branch `main`, environments, IdP OIDC, SCP y permisos efectivos de AWS no se verifican aquí.

## Estado

`AWS_SCP_NOT_VERIFIED` · `OIDC_PROVIDER_NOT_VERIFIED` · `GITHUB_ENVIRONMENTS_NOT_CONFIGURED` ·
`NO_AWS_RESOURCES_CREATED` · `NO_AWS_CREDENTIALS_IN_GITHUB`.
