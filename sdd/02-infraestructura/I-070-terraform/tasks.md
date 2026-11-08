# I-070-terraform — IaC Terraform: stacks, state, drift y seguridad

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-110, I-000..I-060 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## Gestión de tareas H05 (solo especificación y aceptación de diseño)

Estado inicial de todos los trabajos: `TODO`, salvo evidencia local de integridad H04 documentada por el responsable. No marcar como completado por generar archivos.

### T-070-01 — Cerrar roots y módulos

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-110, I-000..I-060.
- **Trabajo:** Definir `infra/terraform/bootstrap` (state si aprobado), `infra/terraform/environments/dev` y módulos vpc/security, compute, lb/tls, dynamodb, redis, observability, iam, dns. Cada entorno variables explícitas, labels y tagging.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-070-02 — Cerrar state

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-110, I-000..I-060.
- **Trabajo:** Backend remoto S3 cifrado/versionado con locking compatible elegido tras precio/permiso; bootstrap con control humano. Alternativa local solo para diseño/offline; nunca state real sensible en Git, log o release. Costo bucket/state incluido en I-000.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-070-03 — Cerrar plan y lock

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-110, I-000..I-060.
- **Trabajo:** `terraform fmt -check`, `terraform init -backend=false` cuando aplique, `validate`, `plan -out=...` solo H08 con credenciales autorizadas; artifact plan revisado por humano antes de apply. Almacenamiento seguro y TTL del plan.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-070-04 — Cerrar drift

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-110, I-000..I-060.
- **Trabajo:** `plan -refresh-only`/diff documentado, no corregir cambios inesperados automáticamente. `prevent_destroy` para datos persistentes, doble confirmación en eliminación de recursos no efímeros.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-070-05 — Cerrar secuencia

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-110, I-000..I-060.
- **Trabajo:** Bootstrap (fuera de runtime), network, IAM, Dynamo, Redis, EC2/ALB, observability, DNS. Destruir runtime en orden inverso; datos y state se preservan salvo autorización separada.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-070-06 — Cerrar condiciones

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-110, I-000..I-060.
- **Trabajo:** No hardcode secrets, no `-auto-approve`, no deploy desde PR no confiable, no `terraform destroy` masivo por ahorro, ni cambiar AWS Free a Paid.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-070-90 — Revisión de riesgos y dependencias

- **Estado:** TODO.
- **Trabajo:** resolver o escalar: Servicio backend state cobrable o SCP bloqueado, Lock incompatible o terraform version drift, Destroy borra datos y provoca pérdida.
- **Criterio DoD:** cada riesgo clasificado RESOLVED/BLOCKED/ACCEPTED con firma/fecha; no usar ACCEPTED para incumplir requisito docente sin aclaración.

### T-070-99 — Ratificación de la especificación

- **Estado:** REVIEW pendiente.
- **Entrada:** `spec.md`, `plan.md`, `tasks.md`, `checklist.md` actualizados.
- **Salida:** decisión explícita del responsable y vínculo a evidencia en informe I-GO.
- **Bloqueo:** no crear Terraform/endpoints ni ejecutar AWS.

## Implementacion H08-I (2026-11-08)

Gate APRUEBO_GATE_I070_LOCAL registrado en sdd/_control/H08/GATE_H08I_I070.md. Rama feat/h08i-i070-terraform. AWS_NOT_RUN: el bootstrap no se ha aplicado.
- T-070-01 Roots y modulos: IMPLEMENTADO_LOCAL. Raiz bootstrap (state/lock) mas modulos existentes; blueprint en BLUEPRINT_MODULOS_TF.md.
- T-070-02 State: IMPLEMENTADO_LOCAL. Bucket versionado, SSE-S3, Block Public Access, ownership enforced, politica TLS, lifecycle y prevent_destroy. Aplicacion NOT_RUN.
- T-070-03 Plan y lock: IMPLEMENTADO_LOCAL. Locking nativo S3 (use_lockfile) con Terraform >= 1.10; sin tabla DynamoDB; backend.tf.example inactivo; solo init -backend=false autorizado.
- T-070-04 Drift: DOCUMENTADO. prevent_destroy y runbook de refresh-only con revision humana; sin correccion automatica.
- T-070-05 Secuencia: DOCUMENTADO. bootstrap, red, IAM, DynamoDB, Redis, computo/balanceo, observabilidad y DNS; destruccion inversa.
- T-070-06 Condiciones: DOCUMENTADO y probado. Sin auto-approve, sin credenciales, sin deploy desde PR, sin destroy masivo.
- T-070-90 Riesgos: CLASIFICADOS. Sin CMK (limite declarado), state no medido, SCP NOT_VERIFIED, backend inactivo.
- T-070-99 Ratificacion: IMPLEMENTADO_LOCAL. Gate registrado; merge y primer apply requieren autorizacion separada.

Pruebas: 22 de contrato en tests/infra/terraform/test_terraform_backend_contrato.py (218 en la suite); incluye politica IAM de state/tflock y casos negativos. fmt/init/validate OK en bootstrap y entorno.

### Cierre I-070: custodia y recuperacion del state de bootstrap (2026-11-08)

- Hueco corregido: el runbook cubria recuperacion del state de runtime, pero no del state propio del bootstrap (creado en local antes de existir el bucket).
- RUNBOOK_IAC.md 2-4: custodia cifrada fuera del repo con SHA-256 y serial/lineage, prohibicion de state en Git/artifacts, migracion a la clave bootstrap/terraform.tfstate con init -migrate-state autorizado, verificacion de lineage/serial/recursos y plan sin cambios inesperados, y recuperacion por importacion con Spec=I-070-terraform sin recrear el bucket.
- Separacion por clave: bootstrap/terraform.tfstate y envs/h08i/terraform.tfstate, cada uno con su .tflock; sin tabla DynamoDB.
- Pruebas: 29 de contrato I-070 (225 en la suite), incluidas separacion de states, prohibicion de migracion sin Gate y ausencia de state en Git/artifacts.
- FinOps: sin buckets ni recursos adicionales; se mantiene SSE-S3, retencion de versiones 30 dias (cambiarla exigiria decision) y prevent_destroy.
- AWS_NOT_RUN: ninguna migracion ni import ejecutado.

### Correccion de revision: custodia, importacion e inventario (2026-11-08)

- Custodia/migracion: el state original permanece en infra/terraform/bootstrap/ hasta que la migracion este verificada; se duplica la copia de seguridad cifrada (SHA-256 verificado) y se registran lineage/serial sin contenido. No se mueve el unico state operativo antes de init -migrate-state.
- Migracion: se compara lineage/serial/recursos con la evidencia previa y NO se asume incremento de serial; se exige plan autorizado sin cambios inesperados antes de retirar la copia local.
- Importacion de recuperacion: criterio N to import con 0 to add, 0 to change, 0 to replace y 0 to destroy; direcciones coincidentes con el codigo, sin doble importacion y parada si se propone reemplazar el bucket. El apply del import exige Gate humano independiente.
- Inventario: 8 direcciones declaradas (7 AWS + terraform_data.validacion_bootstrap), derivadas del codigo y no de una cifra historica. Sin tabla DynamoDB de locking.
- Pruebas: 33 de contrato I-070 (229 en la suite Terraform).
- AWS_NOT_RUN: sin migracion, import, plan remoto ni recursos creados.
