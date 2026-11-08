# I-090-cd-backend — CD, versionamiento, autorizaciones y rollback

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-110, I-020, I-030, I-060, I-080 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## Checklist de revisión (cada punto requiere evidencia, no basta la presencia de este archivo)

### Integridad y autoridad

- [ ] H04 tiene aprobación humana y el contenido coincide con las reglas superiores.
- [ ] Se analizó el PDF docente v3 y el catálogo V6.0.2 sin asumir GCP/MongoDB.
- [ ] Se mantiene el alcance de Backend, no se exige validación Mobile.
- [ ] No se crearon recursos ni artefactos de código/EP antes de H08/H06.

### Decisiones técnicas

- [ ] `INF-090-01` **Matriz versiones:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-090-02` **Aprobación:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-090-03` **Despliegue seguro:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-090-04` **Pruebas HTTP:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-090-05` **Fallos y rollback:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-090-06` **Cierre:** diseño justificado, referencias, supuestos y evidencia trazable.

### Pruebas y cobertura de diseño

- [ ] Release enlaza commit + SHA de binario + OpenAPI + pruebas.
- [ ] GitHub environment humano controla deploy.
- [ ] Siempre dos nodos reales durante release activo, con health.
- [ ] Prueba HTTP y datos persistentes por cada función nueva.
- [ ] Rollback ensayado y documentado antes de producción.

### Riesgos, economía y aprobaciones

- [ ] Resuelto o escalado formalmente: Rollout reduce nodos por debajo 2.
- [ ] Resuelto o escalado formalmente: Fallos HTTP durante cutover.
- [ ] Resuelto o escalado formalmente: Falta permiso OIDC/deploy.
- [ ] Resuelto o escalado formalmente: Presupuesto impide doble capacidad transitoria.
- [ ] Costos potenciales (incluido apagado) identificados por servicio en I-000; no se afirma $0 por usar créditos.
- [ ] SCP y disponibilidad de servicios relevantes documentados (no simulados).
- [ ] Gate I-GO tiene resolución expresa del responsable, o esta spec continúa DRAFT/BLOCKED.

### Ejecución futura H08 (NO ejecutada en este lote)

- [ ] Pruebas AWS/CI/HTTP y sus evidencias ejecutadas realmente tras aprobación y release; no cambiar a PASS antes.

- [x] Workflow CD manual con entorno protegido, autorizacion ligada al SHA, artefacto verificado y sin credenciales estaticas.
- [x] Guardas de 2 nodos sanos antes/despues, MinHealthyPercentage=100, health HTTPS 200 y rollback a LT previa.
- [x] POLITICA_RELEASES.md, PLAN_ROLLBACK.md y EVIDENCIA_DEPLOY.md (NOT_RUN).
- [x] Binario binario-<sha> publicado por CI para promocion.
- [ ] Entorno aws-deploy con revisores, rol OIDC y CD_HABILITADO: PENDIENTE_APLICACION_HUMANA; sin ellos el preflight falla.
- [ ] Ejecucion real y ensayo de rollback: NOT_RUN (requieren Gate AWS).
