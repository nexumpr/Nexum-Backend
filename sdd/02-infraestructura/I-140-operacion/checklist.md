# I-140-operacion — Runbooks, incidentes y verificación operacional

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-090, I-020, I-030, I-070, I-090 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## Checklist de revisión (cada punto requiere evidencia, no basta la presencia de este archivo)

### Integridad y autoridad

- [ ] H04 tiene aprobación humana y el contenido coincide con las reglas superiores.
- [ ] Se analizó el PDF docente v3 y el catálogo V6.0.2 sin asumir GCP/MongoDB.
- [ ] Se mantiene el alcance de Backend, no se exige validación Mobile.
- [ ] No se crearon recursos ni artefactos de código/EP antes de H08/H06.

### Decisiones técnicas

- [ ] `INF-140-01` **Runbooks:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-140-02` **Prioridades:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-140-03` **Drills:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-140-04` **Prueba y aceptación:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-140-05` **Acceso:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-140-06` **Inventario final:** diseño justificado, referencias, supuestos y evidencia trazable.

### Pruebas y cobertura de diseño

- [ ] Plan de respuesta con diagnóstico, mitigación, rollback y cierre.
- [ ] Drills de caída por componente documentados.
- [ ] 2 targets saludables en modo ON.
- [ ] Procedimiento de eliminación IaC no toca datos sin aprobación.
- [ ] Matriz responsabilidades de operación solo Backend.

### Riesgos, economía y aprobaciones

- [ ] Resuelto o escalado formalmente: No hay acceso de recuperación a nodos.
- [ ] Resuelto o escalado formalmente: Inconsistencia tras retry.
- [ ] Resuelto o escalado formalmente: Ausencia de registros para incidentes.
- [ ] Costos potenciales (incluido apagado) identificados por servicio en I-000; no se afirma $0 por usar créditos.
- [ ] SCP y disponibilidad de servicios relevantes documentados (no simulados).
- [ ] Gate I-GO tiene resolución expresa del responsable, o esta spec continúa DRAFT/BLOCKED.

### Ejecución futura H08 (NO ejecutada en este lote)

- [ ] Pruebas AWS/CI/HTTP y sus evidencias ejecutadas realmente tras aprobación y release; no cambiar a PASS antes.
- [x] 10 runbooks con los 8 campos obligatorios y estado `DOCUMENTADO_NO_EJECUTADO`.
- [x] Matriz S1–S4 con coherencia de tiempos y escalamiento.
- [x] 5 drills planificados con `NOT_RUN`; prohibido `EJECUTADO_AWS` en local.
- [x] Inventario derivado del código con `existe_en_aws = NOT_VERIFIED`.
- [x] Política de acceso sin root, sin SSH público y con identidades temporales.
- [ ] Drills reales y cierre de incidentes: `NOT_RUN` (requieren Gate de despliegue).
- [ ] Ratificación humana de tiempos de notificación y severidades.
