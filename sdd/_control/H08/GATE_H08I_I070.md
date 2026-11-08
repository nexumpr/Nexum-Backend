# Gate H08-I — I-070 `terraform` (implementación local)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/GATE_H08I_I070.md` |
| Fecha | 2026-11-08 |
| Spec | `sdd/02-infraestructura/I-070-terraform/` (v0.1.0 DRAFT; spec/plan intactos) |
| Estado | **APRUEBO_GATE_I070_LOCAL REGISTRADO — IMPLEMENTADO_LOCAL (sin AWS)** |
| Main base | `c8e37a78efaa1c9ddc0e412b2b96d465eec4911b` (PR #26, I-060) |
| Rama | `feat/h08i-i070-terraform` |
| Autorización | `APRUEBO_GATE_I070_LOCAL` (mensaje humano independiente, 2026-11-08) |

## 1. Alcance

Implementación local del bootstrap del backend remoto (bucket de state con locking nativo de S3), la plantilla de
backend del entorno, los documentos de blueprint/matriz/runbook y sus pruebas.

**No autoriza** AWS, `terraform plan/apply/destroy`, `init` con backend real, despliegue ni cambios de plan.

## 2. Implementado

| Requisito | Evidencia | Estado |
|---|---|---|
| INF-070-01 Roots y módulos | `infra/terraform/bootstrap` (state/lock) + módulos existentes red/computo/balanceo/dynamodb/redis/iam documentados en `BLUEPRINT_MODULOS_TF.md` | IMPLEMENTADO_LOCAL |
| INF-070-02 State | Bucket S3 versionado, SSE-S3, Block Public Access, ownership `BucketOwnerEnforced`, política que exige TLS, lifecycle de versiones no actuales; `prevent_destroy`. Custodia del state **propio** del bootstrap: el original permanece operativo hasta migrar, respaldo cifrado fuera del repo con SHA-256/`lineage`/`serial`, e importación de recuperación con `N to import` y `0 add/change/replace/destroy` | IMPLEMENTADO_LOCAL; aplicación `NOT_RUN` |
| INF-070-03 Plan y lock | Locking **nativo de S3** (`use_lockfile = true`, Terraform >= 1.10) en la plantilla de backend; **sin** tabla DynamoDB (deprecada); `init -backend=false` es lo único autorizado | IMPLEMENTADO_LOCAL |
| INF-070-04 Drift y recuperación | `prevent_destroy` en el bucket; runbook de `refresh-only`, custodia/migración del state del bootstrap (sin asumir incremento de `serial`), locks huérfanos, recuperación por versiones e importación planificada sin recrear, y rollback con revisión humana | DOCUMENTADO |
| INF-070-05 Secuencia | bootstrap → red → IAM → DynamoDB → Redis → cómputo/balanceo → observabilidad → DNS; destrucción inversa | DOCUMENTADO en `RUNBOOK_IAC.md` |
| INF-070-06 Condiciones | Sin `-auto-approve`, sin credenciales en repo, sin deploy desde PR no confiable, sin destroy masivo por ahorro | DOCUMENTADO y probado |

## 3. Límites y bloqueos

- **SSE-S3 (AES256) en lugar de CMK de KMS**: una CMK tiene coste fijo mensual no presupuestado en I-000. Se declara
  el límite: sin CMK no hay revocación criptográfica inmediata. Cambiarlo exige RFC y recotización.
- **Backend remoto no activo**: `envs/h08i` sigue sin bloque `backend`; la plantilla `.example` no la lee Terraform.
  Activarla exige aplicar el bootstrap con Gate humano, IAM/S3 reales y autorización específica.
- **Permisos de plan/apply**: siguen sin definirse. I-060 sólo aporta trust OIDC y un `iam:PassRole` acotado; la
  matriz de acciones IaC pertenece a I-080/I-090 y al Gate de despliegue.
- **Coste**: bucket de state (línea `S3_STATE` 3 GB-mes + solicitudes) ya figura en I-000; el consumo real **no está
  medido**. No se crea tabla de lock DynamoDB: el mecanismo está deprecado y sería un recurso no presupuestado.
- **Versión de Terraform**: `use_lockfile` exige >= 1.10; raíces fijadas a `>= 1.10.0, < 2.0.0`. Instalado en el
  entorno: **1.16.6**. El CI de I-080 deberá fijar la misma versión mínima.
- **Permisos IAM de state/lock**: contrato en `infra/terraform/policies/terraform-state-access.json.tftpl`
  (ListBucket con `s3:prefix`, Get/Put del state, Get/Put/Delete sólo del `.tflock`, Deny explícito de borrado del
  state). Se asignarán a plan/apply en I-080/I-090; no se modifican roles de I-060.
- `AWS_SCP_NOT_VERIFIED`, RFC-004 PROPUESTO, H-01 abierto y contraseña Redis en state permanecen sin cambios.

## 4. Evidencia local

`terraform fmt -check`, `init -backend=false` (TF_DATA_DIR externo) y `validate` del bootstrap y del entorno;
pruebas de contrato I-070; regresiones Terraform/FinOps/systemd/herramientas/Go; SDD y SHA-256.

**`terraform validate` y las pruebas acreditan configuración válida, no infraestructura desplegada.**

## 5. Cierre

Merge requiere autorización humana ligada a PR y HEAD exactos (A-110/R-004). Despliegue y primer `apply` del
bootstrap requieren Gate independiente. No se declara `AWS_DEPLOYED`, `AWS_VERIFIED` ni `PRODUCTION_READY`.

## 6. Correcciones de revisión (2026-11-08, PR #27)

Tres inconsistencias del procedimiento de recuperación, corregidas dentro de I-070 y sin tocar AWS:

1. **Custodia/migración**: el runbook ordenaba mover el único state operativo antes de migrarlo, dejando a
   `init -migrate-state` sin archivo que leer. Ahora el original permanece en `infra/terraform/bootstrap/` hasta que
   la migración esté verificada; se duplica la **copia de seguridad cifrada**, con SHA-256 verificado y
   `lineage`/`serial` registrados sin contenido.
2. **Aceptación de la importación**: se distinguía mal un plan de import de un plan sin cambios. Ahora se exige
   `N to import` (direcciones previstas) con `0 to add`, `0 to change`, `0 to replace` y `0 to destroy`, direcciones
   coincidentes con el código y sin doble importación del mismo recurso; un reemplazo del bucket detiene el proceso.
3. **Inventario**: el runbook citaba «7 recursos»; el inventario se deriva del código y hoy son **8 direcciones**
   (7 AWS + `terraform_data.validacion_bootstrap`), sin cifras históricas fijas.

No se asume que la migración incremente el `serial`; el criterio es la coincidencia de contenido, no un número.
Pruebas de contrato I-070: **33** (229 en la suite Terraform). `AWS_NOT_RUN`: sin migración, import ni recursos.
