# I-000-cuenta-finops — Cuenta AWS, plan Free y estudio de costo cero

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | GOV-07, GOV-15, A-000, A-110, R-001..060 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## Checklist de revisión (cada punto requiere evidencia, no basta la presencia de este archivo)

### Integridad y autoridad

- [ ] H04 tiene aprobación humana y el contenido coincide con las reglas superiores.
- [ ] Se analizó el PDF docente v3 y el catálogo V6.0.2 sin asumir GCP/MongoDB.
- [ ] Se mantiene el alcance de Backend, no se exige validación Mobile.
- [ ] No se crearon recursos ni artefactos de código/EP antes de H08/H06.

### Decisiones técnicas

- [ ] `INF-000-01` **Plan y contexto:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-000-02` **Matriz por recurso:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-000-03` **Dos escenarios:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-000-04` **Costeo:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-000-05` **Bloqueo de decisión:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-000-06` **Controles:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-000-07` **Investigación con fecha:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-000-08` **Separación de responsabilidades:** diseño justificado, referencias, supuestos y evidencia trazable.

### Pruebas y cobertura de diseño

- [ ] Tabla de precios/limitaciones con sello de fecha y servicio, sin «gratis» por suposición.
- [ ] Costeo a 45 días de escenarios A/B con rangos de consumo y crédito separado.
- [ ] Detección explícita de toda línea que tenga costo >0 y de incompatibilidades SCP/plan.
- [ ] ADR de factibilidad firmado por humano antes de I-GO; ninguna tarifa desconocida marcada 0.
- [ ] Plan de alarmas, shutdown y costo residual de datos y servicios aunque el runtime esté apagado.

### Riesgos, economía y aprobaciones

- [ ] Resuelto o escalado formalmente: Servicios limitados por Free/SCP.
- [ ] Resuelto o escalado formalmente: Tarifas que cambian por región/fecha y consumo.
- [ ] Resuelto o escalado formalmente: Créditos agotados o vencidos antes de evaluación.
- [ ] Resuelto o escalado formalmente: ALB, IP, Redis y observabilidad con facturación incremental.
- [ ] Costos potenciales (incluido apagado) identificados por servicio en I-000; no se afirma $0 por usar créditos.
- [ ] SCP y disponibilidad de servicios relevantes documentados (no simulados).
- [ ] Gate I-GO tiene resolución expresa del responsable, o esta spec continúa DRAFT/BLOCKED.

### Ejecución futura H08 (NO ejecutada en este lote)

- [ ] Pruebas AWS/CI/HTTP y sus evidencias ejecutadas realmente tras aprobación y release; no cambiar a PASS antes.
