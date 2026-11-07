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

- **Estado:** **IMPLEMENTADO_LOCAL 2026-11-07** (Gate `APRUEBO_GATE_I040_LOCAL`); falta el Gate de despliegue AWS.
- **Entrada:** `spec.md`, `plan.md`, `tasks.md`, `checklist.md` actualizados.
- **Salida:** decisión explícita del responsable y vínculo a evidencia en informe I-GO.
- **Bloqueo:** no crear Terraform/endpoints ni ejecutar AWS.

## Implementación H08-I (2026-11-07) — no reescribe el estado H05

Gate `APRUEBO_GATE_I040_LOCAL`; registro en `sdd/_control/H08/GATE_H08I_I040.md`. Rama
`feat/h08i-i040-dynamodb`. Sin recursos AWS: `AWS_NOT_RUN`, `INFRA_NOT_DEPLOYED`.

| Tarea | Estado real | Evidencia |
|---|---|---|
| T-040-01 Tabla principal | **IMPLEMENTADO_LOCAL** | `infra/terraform/modules/dynamodb`: `NEXUM_CORE_<ENV>`, `PAY_PER_REQUEST`, `PK`/`SK` (S), SSE, deletion protection por variable |
| T-040-02 Entidades y claves | **IMPLEMENTADO_LOCAL** | Prefijos de A-060 ya en `internal/domain/dynamo/keys.go`; prueba compara claves con la matriz H05 |
| T-040-03 Rutas de acceso | **APP (A-040/A-060)** | La tabla sólo soporta `GetItem`/`Query` por partición; sin capacidad de Scan en el diseño; cursores firmados en A-040 |
| T-040-04 Unicidad y consistencia | **APP (A-060/A-070)** | `EMAIL#<sha256>/OWNER` con `ConditionExpression`; transacciones y optimistic locking en app |
| T-040-05 Dinero y tiempo | **APP (A-070)** | `int64` centavos y fechas UTC; la tabla no introduce floats |
| T-040-06 TTL y privacidad | **IMPLEMENTADO_LOCAL** | `ttl { attribute_name = "expiresAt" }`, habilitado; documentado como eventual |
| T-040-07 Índices concretos | **IMPLEMENTADO_LOCAL** | `GSI1` (ALL) justificado por E027–E031; `GSI2` opt-in (`habilitar_gsi2 = false`) por no existir scheduler confirmado |
| T-040-08 Infra Terraform | **IMPLEMENTADO_LOCAL** | `aws_dynamodb_table` + TTL + SSE + PITR + tags; GSI enumerados con `dynamic` |
| T-040-90 Riesgos | **CLASIFICADOS** | R01 mitigado en diseño (`NOT_RUN` en AWS); R02 presupuestado (0.6000 USD); R03 en I-130/H08-E; R04 en A-070 |
| T-040-99 Ratificación | **IMPLEMENTADO_LOCAL** | Gate registrado; despliegue y merge requieren autorización humana específica |

**Pruebas:** 23 de contrato en `tests/infra/terraform/test_dynamodb_contrato.py` y 10 de alineación FinOps en
`tests/infra/finops/test_i040_alineacion.py`; `fmt -check`, `init -backend=false` y `validate` en verde.
CI de Terraform sigue inexistente (dueño I-080): requisito registrado, no suprimido.

**Matriz de accesos y FinOps:** `sdd/_control/H08/MATRIZ_ACCESOS_I040.md` recorre entidad por entidad las
operaciones, claves/índices, consistencia, condiciones, transacciones y coste; demuestra que las 75 operaciones
no necesitan `Scan` y cuantifica la amplificación de `GSI1`. Registra también que el nombre físico
`NEXUM_CORE_<ENV>` (autoridad H05/A-060) difiere en capitalización de la «propuesta» `nexum-core-<entorno>` de
H06, sin cambio de claves ni semántica.
