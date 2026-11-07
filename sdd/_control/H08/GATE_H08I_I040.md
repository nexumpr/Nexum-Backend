# Gate H08-I — I-040 `dynamodb` (implementación local)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/GATE_H08I_I040.md` |
| Fecha | 2026-11-07 |
| Spec | `sdd/02-infraestructura/I-040-dynamodb/` (v0.1.0 DRAFT; implementación conforme a la matriz física H05 y A-060) |
| Estado | **INTEGRADO_MAIN — NO DESPLEGADO** (PR #24 MERGED, merge `51e01f6eae37d548b3b790a967859ff314ed6bc7`) |
| Precondición | I-030 cerrada: PR #22 y cierre documental PR #23; `main` `9b2e4ac` |
| Rama | `feat/h08i-i040-dynamodb` |
| Autorización | `APRUEBO_GATE_I040_LOCAL` (2026-11-07) |

## 1. Alcance autorizado

Implementar **localmente** el módulo Terraform de DynamoDB, sus pruebas, la validación FinOps y el registro
documental, con commit, push y **un único PR**. Corregir en ese mismo PR sólo los defectos de I-040.

**No autorizado:** crear/modificar/eliminar recursos AWS, `terraform plan/apply/destroy`, despliegue, cambios de
plan AWS, gasto personal, modificar I-030 o RFC-004, aceptar H-01, implementar I-050 u otras specs, endpoints de
negocio, cambios a los 75 contratos, ni fusionar sin autorización humana específica de PR+HEAD.

## 2. Decisiones de implementación (trazadas a la spec)

| Requisito | Implementación | Fuente |
|---|---|---|
| INF-040-01 | `aws_dynamodb_table` única `NEXUM_CORE_<ENV>`, `PAY_PER_REQUEST`, `PK`(S)/`SK`(S), `SSE`, deletion protection por entorno | spec §INF-040-01, matriz H05 |
| INF-040-02 | Single-table design: prefijos de A-060 ya implementados en `internal/domain/dynamo/keys.go` (USER#, EMAIL#<sha256>, SESSION#) | A-060 §2.2, matriz H05 |
| INF-040-03 | Sin Scan: la tabla sólo expone claves que soportan `GetItem`/`Query` por partición; cursores y ownership en aplicación (A-040/A-060) | spec §INF-040-03 |
| INF-040-04 | Unicidad de email por ítem reservado `EMAIL#<hash>/OWNER` (no GSI único); transacciones en aplicación | A-060 §2.2, A-070 |
| INF-040-05 | Importes en `int64` centavos y fechas UTC separadas: ya fijado en A-070; la tabla no introduce floats | A-070 |
| INF-040-06 | `ttl { attribute_name = "expiresAt", enabled = true }`; TTL **eventual**, no sustituye expiración en app | matriz H05, A-060 §8 |
| INF-040-07 | `GSI1` (GSI1PK/GSI1SK, proyección ALL) justificado por E027–E031; `GSI2` **opt-in** (`habilitar_gsi2 = false`) porque la matriz lo reserva a «workers programados confirmados» y no existe scheduler | spec §INF-040-07 |
| INF-040-08 | `aws_dynamodb_table`, TTL, SSE, tags, PITR; GSI enumerados con `dynamic` y variables, sin omisiones | spec §INF-040-08 |

**Otros límites aplicados:** sin streams (coste y consumidor no aprobado), sin capacidad provisionada
(sin coste con tabla vacía), sin tabla auxiliar de identidad, sin claves KMS propias (SSE administrada por AWS),
sin copias programadas (I-130), `deletion_protection` desactivado en h08i para permitir el teardown autorizado.

## 3. Riesgos de la spec

| ID | Estado tras I-040 | Detalle |
|---|---|---|
| R01 (GSI incorrecto obliga a Scan) | **MITIGADO_EN_DISEÑO** | GSI1 reproduce exactamente `GSI1PK=USER#<id>#TYPE#<type>` / `GSI1SK=<utc>#<id>`, verificado por prueba contra la matriz y contra `keys.go`. La validación con datos reales sigue `NOT_RUN` |
| R02 (retención/PITR costosa) | **PRESUPUESTADA** | DDB_PITR 3 GB-mes = 0.6000 USD/45 d ya incluido en I-000; margen 6.9387 USD. No medido en AWS |
| R03 (borrado de cuenta inconcluso) | **FUERA_DE_ALCANCE** | Orquestación auditable corresponde a H08-E/I-130; la tabla no lo resuelve |
| R04 (transacciones >100 acciones) | **FUERA_DE_ALCANCE** | Límite y límites de metas por request se aplican en A-070/app |

## 4. Evidencia local

| Control | Resultado |
|---|---|
| `terraform fmt -check -recursive` | exit 0 |
| `terraform init -backend=false` (`TF_DATA_DIR` externo) | OK, lock 5.100.0 sin cambios |
| `terraform validate` | **Success** |
| `tests/infra/terraform` | 131 pruebas OK (incluye 23 de contrato I-040) |
| `tests/infra/finops` | 185 pruebas OK (10 nuevas de alineación I-040) |
| `validar_sdd_integral.py --check-git` | PASS |
| `validar_h06.py` | PASS (75 operaciones) |
| `go build/vet/test` | PASS (sin cambios de código Go) |

**Matriz de accesos, seguridad y FinOps:** `sdd/_control/H08/MATRIZ_ACCESOS_I040.md` documenta entidad por
entidad la operación, clave/índice, consistencia, condición de escritura, transacción, aislamiento y coste; prueba
que las 75 operaciones se resuelven sin `Scan` y cuantifica la amplificación de escrituras de `GSI1` (≈0.375 USD
dentro del margen de 6.9387 USD).

**CI aplicable:** Terraform local, CI Go y CI SDD con resultado **PASS real**; **CI Terraform =
`NOT_IMPLEMENTED`** (dueño I-080, no se crea aquí ni se declara verde).

**`terraform validate` acredita configuración válida; NO acredita despliegue.** `AWS_NOT_RUN`,
`INFRA_NOT_DEPLOYED`, coste incurrido 0.0000 USD.

## 5. Dependencias que permanecen abiertas

- **I-060** debe acotar la política IAM a `table_arn` y a los índices (hoy no existe rol).
- **I-130** es dueño de backups programados y de la retención.
- **I-050** (Redis) y **H08-E** (endpoints) siguen `NOT_AUTHORIZED`.
- **RFC-004** y **H-01** no se tocan en este Gate.
