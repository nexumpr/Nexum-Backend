# I-040-dynamodb — DynamoDB: tablas, claves, GSI, TTL y datos

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-060, A-070, E001..E075, I-000 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## Gestión de tareas H05 (solo especificación y aceptación de diseño)

Estado inicial de todos los trabajos: `TODO`, salvo evidencia local de integridad H04 documentada por el responsable. No marcar como completado por generar archivos.

### T-040-01 — Cerrar tabla principal

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-060, A-070, E001..E075, I-000.
- **Trabajo:** Candidata NEXUM_CORE_<env>: PK(S), SK(S); atributos GSI1PK/GSI1SK, GSI2PK/GSI2SK solo si su patrón de acceso está justificado; modo PAY_PER_REQUEST inicialmente sujeto a costos reales; SSE, deletion protection según ambiente.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-040-02 — Cerrar entidades y claves

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-060, A-070, E001..E075, I-000.
- **Trabajo:** USER#<id>/PROFILE, USER#<id>/INCOME, USER#<id>/MOVEMENT#timestamp#id, USER#<id>/GOAL#id, USER#<id>/RECURRING#id, USER#<id>/BUDGET#month#cat, USER#<id>/CONTRIBUTION#id, USER#<id>/CONSENT#version, USER#<id>/SESSION#id; reservas email únicas como EMAIL#<hash-normalizado>/OWNER mediante ConditionExpression.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-040-03 — Cerrar rutas de acceso

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-060, A-070, E001..E075, I-000.
- **Trabajo:** Query por propietario (userId derivado de JWT), fecha y tipo, GSI sparsely indexed; cursores LastEvaluatedKey firmados; nunca Scan sobre datos personales en request. Construir matriz E001-E075→operaciones (GetItem/Query/UpdateItem/TransactWriteItems).
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-040-04 — Cerrar unicidad y consistencia

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-060, A-070, E001..E075, I-000.
- **Trabajo:** Email único sin scan, registro/activación condicionado; todas mutaciones financieras versionadas con optimistic concurrency, transacciones idempotentes; up to 100 actions, max 4MB y no dos acciones sobre mismo ítem. Limitar nº metas/allocations por operación.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-040-05 — Cerrar dinero y tiempo

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-060, A-070, E001..E075, I-000.
- **Trabajo:** amountCents int64; instante occurredAt UTC y timezone IANA del usuario, fecha date-only separada; no float; no duplicar descuentos de recurrentes fijos.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-040-06 — Cerrar ttl y privacidad

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-060, A-070, E001..E075, I-000.
- **Trabajo:** TTL aplicado a token de recuperación, claves temporales y sesiones caducadas cuando sea legalmente válido; TTL es eventual, nunca reemplaza comprobación de expiración en app. Backups/retención y borrado real de cuenta requieren orquestación auditable y pruebas.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-040-07 — Cerrar índices concretos

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-060, A-070, E001..E075, I-000.
- **Trabajo:** GSI1: GSI1PK/SK en movements filtrados por usuario+tipo/tiempo; GSI2: índices sparse de notificaciones/reminders por estado/próxima fecha solo con consulta justificable. Prohibido crear GSI especulativos sin medición/uso.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-040-08 — Cerrar infra terraform futuro

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-060, A-070, E001..E075, I-000.
- **Trabajo:** aws_dynamodb_table, TTL, SSE, tags, PITR condicionado a presupuesto y riesgo (no descartable sin aprobación), y role policy recursos acotados. Los GSI/tables deben enumerarse en plan y variables sin omisiones.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-040-90 — Revisión de riesgos y dependencias

- **Estado:** TODO.
- **Trabajo:** resolver o escalar: GSI incorrecto obliga a Scan, Retención/PITR costosa, Borrado de cuenta inconcluso, Transacciones sobre 100 elementos superan límites.
- **Criterio DoD:** cada riesgo clasificado RESOLVED/BLOCKED/ACCEPTED con firma/fecha; no usar ACCEPTED para incumplir requisito docente sin aclaración.

### T-040-99 — Ratificación de la especificación

- **Estado:** REVIEW pendiente.
- **Entrada:** `spec.md`, `plan.md`, `tasks.md`, `checklist.md` actualizados.
- **Salida:** decisión explícita del responsable y vínculo a evidencia en informe I-GO.
- **Bloqueo:** no crear Terraform/endpoints ni ejecutar AWS.
