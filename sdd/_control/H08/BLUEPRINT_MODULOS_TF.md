# I-070 — Blueprint de módulos y raíces Terraform

| Campo | Valor |
|---|---|
| Estado | `IMPLEMENTADO_LOCAL`; aplicación AWS `NOT_RUN` |
| Gate | `APRUEBO_GATE_I070_LOCAL` |
| Raíces | `infra/terraform/bootstrap` (state/lock) · `infra/terraform/envs/h08i` (runtime) |
| Módulos | `red`, `computo`, `balanceo`, `dynamodb`, `redis`, `iam` |

## 1. Estructura

```
infra/terraform/
  bootstrap/            # bucket de state + tabla de lock (se aplica primero, Gate humano)
  envs/h08i/            # runtime: red, computo, balanceo, dynamodb, redis, iam
    backend.tf.example  # plantilla del backend remoto; NO activa
  modules/
    red/                # VPC, subredes privadas, SGs, endpoints, NACL
    computo/            # LT + ASG (2 x t4g.micro), systemd, bootstrap
    balanceo/           # ALB interno, TG, CloudFront VPC origin
    dynamodb/           # tabla unica NEXUM_CORE_<ENV>, GSI1, TTL, PITR
    redis/              # ElastiCache Redis OSS, RBAC, TLS
    iam/                # rol runtime, politicas, trust OIDC condicional
```

## 2. Dependencias entre raíces y módulos

| Origen | Consume | Nota |
|---|---|---|
| `bootstrap` | — | Se aplica con credenciales humanas; su state es local y no se versiona |
| `envs/h08i` | `red` | VPC, subredes y security groups |
| `envs/h08i` | `computo` | Subredes + SG backend + instance profile de `iam` |
| `envs/h08i` | `balanceo` | Subredes + SG ALB/backend + nombre del ASG |
| `envs/h08i` | `dynamodb` | `table_arn` e índices alimentan la política de `iam` |
| `envs/h08i` | `redis` | Subredes + SG redis + password (bloqueado por RFC-004/state) |
| `envs/h08i` | `iam` | Rol runtime + trust OIDC condicional |
| Backend remoto | `bootstrap` | Sólo tras aplicar bootstrap y autorizar `init` con backend real |

## 3. Orden de aplicación previsto (no ejecutado)

1. `bootstrap` (state + lock) — Gate de infraestructura independiente.
2. Verificación de lectura del bucket/tabla y activación de `backend.tf` (copia de la plantilla).
3. `red`, `iam`, `dynamodb`, `redis`, `computo`, `balanceo`, observabilidad y DNS según I-110/I-150.
4. Destrucción en orden inverso, preservando state y datos salvo autorización separada.

## 4. Fuera de este blueprint

- Puede que el nombre físico del bucket requiera sufijo único de cuenta; se aporta por variable, no se inventa.
- Permisos IAM de plan/apply y workflows AWS pertenecen a I-080/I-090.
- Observabilidad y DNS pertenecen a I-110/I-150.
