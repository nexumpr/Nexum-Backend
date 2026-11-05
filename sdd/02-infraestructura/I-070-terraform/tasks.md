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
