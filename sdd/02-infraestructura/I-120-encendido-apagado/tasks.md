# I-120-encendido-apagado — Encendido, apagado y preservación de datos

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | I-000, I-020, I-030, I-040, I-050 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## Gestión de tareas H05 (solo especificación y aceptación de diseño)

Estado inicial de todos los trabajos: `TODO`, salvo evidencia local de integridad H04 documentada por el responsable. No marcar como completado por generar archivos.

### T-120-01 — Cerrar estados

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y I-000, I-020, I-030, I-040, I-050.
- **Trabajo:** `OFF` runtime efímero deshabilitado; `STARTING`, `ON_2_HEALTHY` (único estado operativo validable), `DRAINING`, `ERROR`, `COST_REVIEW`. Toda transición requiere actor, timestamp y runbook; datos persisten.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-120-02 — Cerrar encender

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y I-000, I-020, I-030, I-040, I-050.
- **Trabajo:** Aprobación humana, check plan/credits, desiredCapacity=2 y healthcheck de targets y Redis; no exponer URL hasta 2 healthy; pruebas HTTP y auditoría de costo inicial.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-120-03 — Cerrar apagar

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y I-000, I-020, I-030, I-040, I-050.
- **Trabajo:** Drenar requests, bloquear trabajos, recoger logs, bajar o destruir runtime efímero autorizado; detener EC2 no equivale a eliminar ALB/IPv4/Redis/EBS/billing. Persistir Dynamo y Terraform state; registrar recursos residuales con precios.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-120-04 — Cerrar dns/alb

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y I-000, I-020, I-030, I-040, I-050.
- **Trabajo:** Evaluar si ALB puede permanecer y cuánto factura; modo más económico puede requerir destruir ALB y reprovisionar posteriormente (cambio DNS y TLS), previa autorización. No eliminar tablas por defecto.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-120-05 — Cerrar seguridad

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y I-000, I-020, I-030, I-040, I-050.
- **Trabajo:** Nunca utilizar destroy-all en un script de apagado. Separar explícitamente `destroy-runtime` de `destroy-data` y `destroy-state` con protección fuerte.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-120-06 — Cerrar prueba

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y I-000, I-020, I-030, I-040, I-050.
- **Trabajo:** Tras OFF, obtener Resource Inventory y Billing, verificar datos en Dynamo intactos y restaurar ON con 2 nodos; comparar costos reales.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-120-90 — Revisión de riesgos y dependencias

- **Estado:** TODO.
- **Trabajo:** resolver o escalar: Apagado incompleto mantiene ALB/Redis facturados, Destroy accidental de datos, Gastos mientras runtime OFF.
- **Criterio DoD:** cada riesgo clasificado RESOLVED/BLOCKED/ACCEPTED con firma/fecha; no usar ACCEPTED para incumplir requisito docente sin aclaración.

### T-120-99 — Ratificación de la especificación

- **Estado:** REVIEW pendiente.
- **Entrada:** `spec.md`, `plan.md`, `tasks.md`, `checklist.md` actualizados.
- **Salida:** decisión explícita del responsable y vínculo a evidencia en informe I-GO.
- **Bloqueo:** no crear Terraform/endpoints ni ejecutar AWS.
