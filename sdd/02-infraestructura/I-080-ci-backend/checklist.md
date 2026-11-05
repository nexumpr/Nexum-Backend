# I-080-ci-backend — CI Backend GitHub Actions y quality gates

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-100, A-110, I-060, I-070 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## Checklist de revisión (cada punto requiere evidencia, no basta la presencia de este archivo)

### Integridad y autoridad

- [ ] H04 tiene aprobación humana y el contenido coincide con las reglas superiores.
- [ ] Se analizó el PDF docente v3 y el catálogo V6.0.2 sin asumir GCP/MongoDB.
- [ ] Se mantiene el alcance de Backend, no se exige validación Mobile.
- [ ] No se crearon recursos ni artefactos de código/EP antes de H08/H06.

### Decisiones técnicas

- [ ] `INF-080-01` **Eventos:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-080-02` **PR:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-080-03` **Seguridad CI:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-080-04` **Pruebas:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-080-05` **Fallos:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-080-06` **Versiones:** diseño justificado, referencias, supuestos y evidencia trazable.

### Pruebas y cobertura de diseño

- [ ] Pipeline PR bloquea fallos y es reproducible.
- [ ] Consolidación logs/artifacts sin credenciales.
- [ ] Ejecuta pruebas Go unit/integration/security planificadas.
- [ ] Secreto encontrado bloquea job.
- [ ] Aún sin workflows ejecutados: estado DRAFT, no confundir diseño con CI verde.

### Riesgos, economía y aprobaciones

- [ ] Resuelto o escalado formalmente: CI no puede usar toolchains por plan limitado.
- [ ] Resuelto o escalado formalmente: Pipeline costo/tiempo alto.
- [ ] Resuelto o escalado formalmente: Escaneo IoC inseguro con falsos PASS.
- [ ] Costos potenciales (incluido apagado) identificados por servicio en I-000; no se afirma $0 por usar créditos.
- [ ] SCP y disponibilidad de servicios relevantes documentados (no simulados).
- [ ] Gate I-GO tiene resolución expresa del responsable, o esta spec continúa DRAFT/BLOCKED.

### Ejecución futura H08 (NO ejecutada en este lote)

- [ ] Pruebas AWS/CI/HTTP y sus evidencias ejecutadas realmente tras aprobación y release; no cambiar a PASS antes.
