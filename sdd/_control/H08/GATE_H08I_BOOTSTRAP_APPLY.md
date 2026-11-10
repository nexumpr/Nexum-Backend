# Gate H08-I — Aplicación del bootstrap de Terraform (I-070)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/GATE_H08I_BOOTSTRAP_APPLY.md` |
| Fecha | 2026-11-10 |
| Spec | `sdd/02-infraestructura/I-070-terraform/` (bootstrap del state remoto) |
| `main` base | `2958136a824a0d25e91ad87c788dac51a96e2991` |
| Rama | `fix/h08i-bootstrap-apply-gate` |
| Estado | **PREPARADO — PENDIENTE DE AUTORIZACIÓN HUMANA** |
| Artefacto | `infra/terraform/bootstrap/bootstrap.tfplan` (no versionado: `.gitignore`) |
| SHA-256 del plan | `57C195ADB49548B8490445D2A8222EB69A7599B19B10BD775B81E7DC1563874C` |
| Tamaño / generación | 7 746 bytes · `2026-11-10T17:00:10Z` |
| Identidad usada | cuenta `713797602090`, rol `AccountFullAccessRole` vía sesión `aws login`; sin claves permanentes |
| Recursos AWS creados | **0** (`AWS_NOT_RUN`) |

## 1. Alcance autorizable

Aplicar **exactamente** el plan con el SHA-256 de la tabla. El plan crea **7 recursos AWS**
(bucket de state S3 y sus 6 controles) y **1 recurso local** `terraform_data`, sin destruir,
modificar ni reemplazar nada.

**No autoriza**: migración del state al backend remoto, `init -migrate-state`, `init -reconfigure`,
`terraform state`, `import`, `destroy`, ni ninguna spec I-010..I-150 o H08-E.

## 2. Hechos verificados (2026-11-10)

| Comprobación | Resultado |
|---|---|
| `git rev-parse HEAD` | `2958136a824a0d25e91ad87c788dac51a96e2991` |
| `terraform show -json` · `applyable` | `true` |
| `terraform show -json` · `complete` | `true` |
| `terraform show -json` · `errored` | `false` |
| `resource_changes` | **8** (7 `registry.terraform.io/hashicorp/aws` + 1 `terraform.io/builtin/terraform`) |
| Acciones | **8 `create`** · 0 `update` · 0 `delete` · 0 `replace` |
| Importaciones | 0 |
| `sufijo_cuenta` | `713797602090` |
| `region` | `us-east-2` |
| `dias_retencion_versiones` | `30` |
| Bucket resultante | `nexum-h08i-tfstate-713797602090` |
| Verificador local | `PASS` (`herramientas/verificar_plan_bootstrap.py`) |

Inventario exacto del plan:

1. `aws_s3_bucket.state`
2. `aws_s3_bucket_versioning.state`
3. `aws_s3_bucket_server_side_encryption_configuration.state`
4. `aws_s3_bucket_public_access_block.state`
5. `aws_s3_bucket_ownership_controls.state`
6. `aws_s3_bucket_policy.state`
7. `aws_s3_bucket_lifecycle_configuration.state`
8. `terraform_data.validacion_bootstrap` (local)

Comando de verificación reproducible:

```
py -3.13 herramientas/verificar_plan_bootstrap.py \
  --plan-json <salida de terraform show -json> \
  --tfplan infra/terraform/bootstrap/bootstrap.tfplan \
  --sha-esperado 57C195ADB49548B8490445D2A8222EB69A7599B19B10BD775B81E7DC1563874C
```

Resultado obtenido: `{"resultado": "PASS", "problemas": [], "sha256": "57C195AD…874C", "recursos": 8}`.

> La regla de permiso `terraform show -json bootstrap.tfplan` **no está habilitada**. Su
> solicitud consta en RFC-002 (`APRUEBO_PERMISO_TERRAFORM_SHOW_PLAN`). La verificación ya se
> ejecutó y su salida queda registrada aquí; repetirla requiere habilitar esa regla o que el
> operador ejecute el comando manualmente.

## 3. Defecto del plan entregado y corrección

El `bootstrap.tfplan` previo (SHA-256 `036C05D0AD258E3614AA3274406639EB5777EECA2646232E4499D7E44DE495F5`)
**no era aplicable**. Evidencia: `applyable=false`, `complete=false`, `errored=true` y **1 de 8**
recursos planificados (solo `terraform_data.validacion_bootstrap`).

Tres causas reales, reproducidas en ejecución directa:

1. `AWS_PROFILE=nexum-tf-bootstrap` apuntaba a un perfil **inexistente** en `~/.aws/config`
   (solo existen `nexum` y `nexum-login`): `failed to get shared config profile`.
2. El root `infra/terraform/bootstrap` **no declaraba `provider "aws"`**: Terraform aborta con
   *Provider requires explicit configuration*. `terraform validate` no lo detecta.
3. `var.sufijo_cuenta` quedó vacío y la precondition del bucket falló (`nexum-h08i-tfstate-`).

Correcciones aplicadas en esta rama:

- Bloque `provider "aws"` con `region = var.region` y `default_tags` (sin credenciales).
- Prueba de regresión `test_declara_proveedor_aws_explicito` en
  `tests/infra/terraform/test_terraform_backend_contrato.py`.
- Verificador `herramientas/verificar_plan_bootstrap.py` + 13 pruebas en
  `herramientas/tests/test_plan_bootstrap.py`.
- Plan regenerado con `sufijo_cuenta` explícito y un `credential_process` temporal **fuera del
  repositorio** (`%TEMP%`), sin claves en disco ni en Git.

## 4. Coste

| Partida | Importe | Estado |
|---|---|---|
| S3 state (3 GB-mes × 0.023) | 0.0690 USD/mes | Ya presupuestado en I-000; **no es coste nuevo** |
| Equivalente 45 días | ≈ 0.1035 USD | Estimación |
| KMS, DynamoDB de lock, NAT, IPv4 pública | 0.0000 USD | No se crean |
| Coste real observado | `NOT_VERIFIED` | No se consultó Cost Explorer |

El margen teórico de I-000 sigue siendo 6.9387 USD sobre 120.0000 USD verificados. **No** se declara
gasto cero: es un recurso dentro de una partida ya aprobada.

## 5. Autorización requerida

Frase exacta (una sola vez, en mensaje humano independiente):

```
APRUEBO_APPLY_BOOTSTRAP_SHA256_57C195ADB49548B8490445D2A8222EB69A7599B19B10BD775B81E7DC1563874C
```

Además, `terraform apply*` está **denegado** en `opencode.json`. Tras la frase, la excepción mínima a
registrar en RFC-002 es exactamente:

- `permissions`: `{"action": "shell", "resource": "terraform apply bootstrap.tfplan", "effect": "ask"}`
- `policies`: `{"action": "tool.use", "resource": "shell:terraform apply bootstrap.tfplan", "effect": "allow"}`

Sin ambas capas la orden se rechaza. No se habilita `apply -auto-approve`, ni `-target`, ni `destroy`.

## 6. Procedimiento exacto tras la autorización

1. `git rev-parse HEAD` y `git status --porcelain` (árbol limpio; si hay cambios, **detener**).
2. Recalcular el SHA-256 del plan; si difiere del de este Gate, **detener**.
3. Ejecutar el verificador; debe devolver `PASS` con 8 recursos.
4. Aplicar el plan exacto: `terraform apply bootstrap.tfplan` (sin `-auto-approve`).
5. Custodiar el state (§7).
6. Verificar el bucket creado (solo lectura) y registrar el resultado.
7. Actualizar `memoria.md`, `ESTADO_Y_GATES.md` y este Gate con el resultado real.

## 7. Custodia del state

- El state local `infra/terraform/bootstrap/terraform.tfstate` **permanece en su ubicación operativa**;
  está excluido por `.gitignore` y nunca se commitea.
- Se duplica una **copia de seguridad cifrada fuera del repositorio** con SHA-256, `lineage` y `serial`
  registrados (sin contenido).
- No se mueve el original antes de la migración: `init -migrate-state` necesita leerlo.
- La migración al backend S3 es un **Gate distinto** (paso 3 del plan de despliegue).

## 8. Verificación posterior (solo lectura)

- `aws s3api list-buckets` confirma la existencia del bucket.
- `aws s3api get-bucket-versioning`, `get-bucket-encryption`, `get-public-access-block` y
  `get-bucket-policy` confirman los controles (requieren autorización ASK por comando).
- `terraform show` del state local confirma 8 recursos y `prevent_destroy` en el bucket.

## 9. Rollback

- El bucket tiene `prevent_destroy = true`: **no** se destruye por conveniencia.
- Un fallo parcial no autoriza `destroy`; se sigue `RUNBOOK_IAC.md` y se abre incidente.
- Si el SHA no coincide o el verificador falla: no se aplica nada y se regenera el plan con un Gate nuevo.

## 10. Bloqueos que este Gate no resuelve

`RFC-004` (secretos de bootstrap), `H-01` (TLSv1 por certificado por defecto), `RFC-006` (WAF
temporal), `RFC-007` (SNS/CMK), `D-100-01`, `D-100-02`, `D-110-01`, `AWS_SCP_NOT_VERIFIED`,
`OIDC`/environments `NOT_VERIFIED`, readiness 503 y la condición presupuestaria de I-000.

**Este documento no se autoaprueba**: su estado es `PREPARADO` hasta que exista la frase humana.
