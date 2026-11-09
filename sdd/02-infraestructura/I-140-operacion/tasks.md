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

## Implementacion H08-I (2026-11-09) — LOCAL, sin AWS

Gate `APRUEBO_GATE_I140_LOCAL`. Rama `feat/h08i-i140-operacion`. Nada se ejecuta contra AWS.

- **T-140-01 (Runbooks):** IMPLEMENTADO_LOCAL. 10 procedimientos con responsable, precondiciones, diagnostico, mitigacion, rollback, evidencia, criterio y estado `DOCUMENTADO_NO_EJECUTADO`.
- **T-140-02 (Prioridades):** IMPLEMENTADO_LOCAL. S1–S4 con tiempos, escalamiento y rollback; coherencia validada y `PENDIENTE_DE_RATIFICACION_HUMANA`.
- **T-140-03 (Drills):** IMPLEMENTADO_LOCAL. 5 drills planificados en `NOT_RUN`; el validador rechaza `EJECUTADO_AWS`, acciones destructivas y drills con AWS sin datos sinteticos.
- **T-140-04 (Prueba y aceptacion):** IMPLEMENTADO_LOCAL. Inventario derivado del codigo y contraste con los validadores SDD; AWS `NOT_VERIFIED`.
- **T-140-05 (Acceso):** IMPLEMENTADO_LOCAL. Politica sin root, sin SSH publico, con identidades temporales y SSM solo en ventana.
- **T-140-06 (Inventario final):** IMPLEMENTADO_LOCAL. Modulos y recursos declarados; un nombre no prueba existencia.
- **T-140-90 (Riesgos):** acceso de recuperacion a nodos limitado a SSM en ventana; inconsistencia tras reintentos cubierta por idempotencia de A-070; registros de incidentes dependientes del agente CloudWatch (I-020). Los tres CONDICIONADOS a verificacion real.
- **T-140-99 (Ratificacion):** REVIEW pendiente; merge requiere autorizacion humana ligada a PR + HEAD.

### Ampliacion de alcance (2026-11-09)

- **T-140-01:** 12 runbooks × 13 campos; validador de runbooks y de comandos peligrosos.
- **T-140-02:** matriz S1–S3 con ciclo completo; tiempos `PENDIENTE_DECISION_HUMANA`.
- **T-140-03:** 12 drills con estados locales y `NOT_RUN_AWS`; prohibido `demuestra_aws`.
- **T-140-04:** matriz de aceptacion 12 A + 16 I + 75 E y criterios cloud pendientes.
- **T-140-05:** politica de acceso documentada y validada.
- **T-140-06:** inventario trazable con core 24x7 y auxiliares autorizables.
