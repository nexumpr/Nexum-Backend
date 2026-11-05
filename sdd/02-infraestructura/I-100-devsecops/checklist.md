# I-100-devsecops — DevSecOps integral y amenaza por superficie

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-050, A-100, I-060, I-080 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## Checklist de revisión (cada punto requiere evidencia, no basta la presencia de este archivo)

### Integridad y autoridad

- [ ] H04 tiene aprobación humana y el contenido coincide con las reglas superiores.
- [ ] Se analizó el PDF docente v3 y el catálogo V6.0.2 sin asumir GCP/MongoDB.
- [ ] Se mantiene el alcance de Backend, no se exige validación Mobile.
- [ ] No se crearon recursos ni artefactos de código/EP antes de H08/H06.

### Decisiones técnicas

- [ ] `INF-100-01` **Threat model:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-100-02` **SAST/SCA:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-100-03` **Secrets/SBOM:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-100-04` **DAST:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-100-05` **IaC:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-100-06` **Pruebas adversariales:** diseño justificado, referencias, supuestos y evidencia trazable.

### Pruebas y cobertura de diseño

- [ ] Modelo amenazas cubre 5 superficies y 75 contratos.
- [ ] CI marca y bloquea vulnerabilidades según severidad/alcance.
- [ ] Pruebas authz Cross-user con 401/403/404 apropiados.
- [ ] DAST autorizado con datos falsos y evidencias.
- [ ] No ejecutar herramientas ofensivas en H05.

### Riesgos, economía y aprobaciones

- [ ] Resuelto o escalado formalmente: Riesgo de pruebas que ataquen producción.
- [ ] Resuelto o escalado formalmente: Falsos negativos de escáner.
- [ ] Resuelto o escalado formalmente: Secrets en artefactos/logs.
- [ ] Costos potenciales (incluido apagado) identificados por servicio en I-000; no se afirma $0 por usar créditos.
- [ ] SCP y disponibilidad de servicios relevantes documentados (no simulados).
- [ ] Gate I-GO tiene resolución expresa del responsable, o esta spec continúa DRAFT/BLOCKED.

### Ejecución futura H08 (NO ejecutada en este lote)

- [ ] Pruebas AWS/CI/HTTP y sus evidencias ejecutadas realmente tras aprobación y release; no cambiar a PASS antes.
