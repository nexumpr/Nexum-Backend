# I-140-operacion — Runbooks, incidentes y verificación operacional

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-090, I-020, I-030, I-070, I-090 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## Gestión de tareas H05 (solo especificación y aceptación de diseño)

Estado inicial de todos los trabajos: `TODO`, salvo evidencia local de integridad H04 documentada por el responsable. No marcar como completado por generar archivos.

### T-140-01 — Cerrar runbooks

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-090, I-020, I-030, I-070, I-090.
- **Trabajo:** Crear procedimientos para deployment, health fail, Dynamo throttle/timeout, Redis unavailable, certificados, secrets rotation, 401/403 no esperados, contención de presupuesto, incidentes y eliminación IaC segura.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-140-02 — Cerrar prioridades

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-090, I-020, I-030, I-070, I-090.
- **Trabajo:** S1 pérdida/filtración datos o servicio, S2 degradación parcial, S3 warnings; escalamiento humano, tiempos de notificación y rollback.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-140-03 — Cerrar drills

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-090, I-020, I-030, I-070, I-090.
- **Trabajo:** Aislar instancia A y validar B, reemplazo target, fallo Redis con degradación definida, cuotas Dynamo y timeout, rotación JWT; solo H08 con autorización y datos sintéticos.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-140-04 — Cerrar prueba y aceptación

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-090, I-020, I-030, I-070, I-090.
- **Trabajo:** Comparar implementación contra 16 specs I, 12 specs A y contratos E; URL HTTPS/HTTP real, log/trace y Dynamo persistente. Registrar evidencia por escenario.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-140-05 — Cerrar acceso

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-090, I-020, I-030, I-070, I-090.
- **Trabajo:** Operación segura con identidades temporales; no shell de root, no abrir SSH público o cambiar SCP a ciegas.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-140-06 — Cerrar inventario final

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-090, I-020, I-030, I-070, I-090.
- **Trabajo:** Lista de recursos/owners/tags, servicios 24×7 vs efímeros y facturación residual. Nombres de recursos no hacen prueba de existencia.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-140-90 — Revisión de riesgos y dependencias

- **Estado:** TODO.
- **Trabajo:** resolver o escalar: No hay acceso de recuperación a nodos, Inconsistencia tras retry, Ausencia de registros para incidentes.
- **Criterio DoD:** cada riesgo clasificado RESOLVED/BLOCKED/ACCEPTED con firma/fecha; no usar ACCEPTED para incumplir requisito docente sin aclaración.

### T-140-99 — Ratificación de la especificación

- **Estado:** REVIEW pendiente.
- **Entrada:** `spec.md`, `plan.md`, `tasks.md`, `checklist.md` actualizados.
- **Salida:** decisión explícita del responsable y vínculo a evidencia en informe I-GO.
- **Bloqueo:** no crear Terraform/endpoints ni ejecutar AWS.
