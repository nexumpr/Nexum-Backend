# I-130-backup-recuperacion — Respaldo, restauración, RTO/RPO y borrado

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-070, I-040, I-070, I-120 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## Gestión de tareas H05 (solo especificación y aceptación de diseño)

Estado inicial de todos los trabajos: `TODO`, salvo evidencia local de integridad H04 documentada por el responsable. No marcar como completado por generar archivos.

### T-130-01 — Cerrar rto/rpo

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-070, I-040, I-070, I-120.
- **Trabajo:** Objetivos propuestos para revisión: RPO 24h si backup diario y RTO 4h en laboratorio, ajustados tras experiencia/costeo. No afirmar objetivos logrados sin restore medido.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-130-02 — Cerrar datos

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-070, I-040, I-070, I-120.
- **Trabajo:** Dynamo on-demand backup o PITR si habilitado/financiable; costos por GB y restauración, separación env, tags y retención. No renunciar a restaurabilidad por ahorro sin ADR humano.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-130-03 — Cerrar state

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-070, I-040, I-070, I-120.
- **Trabajo:** Respaldo y versionado del backend Terraform aprobado; IAM policies y parámetros de recuperación fuera Git, secretos con proceso de rotación y acceso auditado.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-130-04 — Cerrar restore

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-070, I-040, I-070, I-120.
- **Trabajo:** Recuperar en tabla NUEVA aislada y comparar recuentos/checksums/relaciones sin sobreescribir datos activos; validar movimientos y saldos con invariantes, probar sesion tras recuperación.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-130-05 — Cerrar privacidad

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-070, I-040, I-070, I-120.
- **Trabajo:** Política de retención/borrado de cuenta: backups pueden conservar datos hasta caducar bajo aviso/obligaciones aplicables; inhabilitar acceso y documentar eliminación diferida, sin afirmar borrado instantáneo de copias inmutables.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-130-06 — Cerrar runbook

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-070, I-040, I-070, I-120.
- **Trabajo:** Secuencia incidente→pausa de escrituras→verificación backup→restauración→reconciliación→reanudación, evidence con duración y costo.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-130-90 — Revisión de riesgos y dependencias

- **Estado:** TODO.
- **Trabajo:** resolver o escalar: PITR/backup adicional de pago, Pérdida de datos al eliminar tabla, Restore incorrecto de aportaciones divididas.
- **Criterio DoD:** cada riesgo clasificado RESOLVED/BLOCKED/ACCEPTED con firma/fecha; no usar ACCEPTED para incumplir requisito docente sin aclaración.

### T-130-99 — Ratificación de la especificación

- **Estado:** REVIEW pendiente.
- **Entrada:** `spec.md`, `plan.md`, `tasks.md`, `checklist.md` actualizados.
- **Salida:** decisión explícita del responsable y vínculo a evidencia en informe I-GO.
- **Bloqueo:** no crear Terraform/endpoints ni ejecutar AWS.
