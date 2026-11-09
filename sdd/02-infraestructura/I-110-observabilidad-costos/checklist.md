# I-110-observabilidad-costos — CloudWatch, métricas, tracing y monitoreo FinOps

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-090, I-000, I-020, I-030 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## Checklist de revisión (cada punto requiere evidencia, no basta la presencia de este archivo)

### Integridad y autoridad

- [ ] H04 tiene aprobación humana y el contenido coincide con las reglas superiores.
- [ ] Se analizó el PDF docente v3 y el catálogo V6.0.2 sin asumir GCP/MongoDB.
- [ ] Se mantiene el alcance de Backend, no se exige validación Mobile.
- [ ] No se crearon recursos ni artefactos de código/EP antes de H08/H06.

### Decisiones técnicas

- [ ] `INF-110-01` **Logs:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-110-02` **Métricas:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-110-03` **Alertas:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-110-04` **Retención:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-110-05` **Trazabilidad:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-110-06` **FinOps:** diseño justificado, referencias, supuestos y evidencia trazable.

### Pruebas y cobertura de diseño

- [ ] Tablero y consultas de salud para dos nodos.
- [ ] Alarmas de caída y presupuesto diseñadas.
- [ ] PII nunca presente en logs en fixtures de prueba.
- [ ] Retención y precio CloudWatch explicitados.
- [ ] Runbooks de incidentes relacionados a metricas y cada alarma.

### Riesgos, economía y aprobaciones

- [ ] Resuelto o escalado formalmente: Altos costos de logs/trazas.
- [ ] Resuelto o escalado formalmente: PII inadvertida.
- [ ] Resuelto o escalado formalmente: Alarmas insuficientes para evitar cargos.
- [ ] Costos potenciales (incluido apagado) identificados por servicio en I-000; no se afirma $0 por usar créditos.
- [ ] SCP y disponibilidad de servicios relevantes documentados (no simulados).
- [ ] Gate I-GO tiene resolución expresa del responsable, o esta spec continúa DRAFT/BLOCKED.

### Ejecución futura H08 (NO ejecutada en este lote)

- [ ] Pruebas AWS/CI/HTTP y sus evidencias ejecutadas realmente tras aprobación y release; no cambiar a PASS antes.

- [x] Log group con retencion finita y politica de logs sin PII.
- [x] 3 metric filters y 5 alarmas con runbooks.
- [x] Presupuesto 85 % ACTUAL y 100 % FORECASTED; no se presenta como limite de gasto.
- [x] Dashboard y consultas de dos nodos/p50-p95-p99 documentados.
- [ ] Agente de CloudWatch en instancias (dependencia I-020) y aplicacion AWS: NOT_RUN.
- [ ] Metricas DynamoDB/Redis y OTel runtime: pendientes de identificadores/exportador.

- [ ] Notificacion automatica de alarmas: BLOCKED_RFC007_SNS_KMS (CMK SNS no presupuestada); alarmas visibles y con runbook.
