# I-130-backup-recuperacion — Respaldo, restauración, RTO/RPO y borrado

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-070, I-040, I-070, I-120 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## Checklist de revisión (cada punto requiere evidencia, no basta la presencia de este archivo)

### Integridad y autoridad

- [ ] H04 tiene aprobación humana y el contenido coincide con las reglas superiores.
- [ ] Se analizó el PDF docente v3 y el catálogo V6.0.2 sin asumir GCP/MongoDB.
- [ ] Se mantiene el alcance de Backend, no se exige validación Mobile.
- [ ] No se crearon recursos ni artefactos de código/EP antes de H08/H06.

### Decisiones técnicas

- [ ] `INF-130-01` **RTO/RPO:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-130-02` **Datos:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-130-03` **State:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-130-04` **Restore:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-130-05` **Privacidad:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-130-06` **Runbook:** diseño justificado, referencias, supuestos y evidencia trazable.

### Pruebas y cobertura de diseño

- [ ] Inventario de activos con política de backup y costo.
- [ ] Restore probado con datos sintéticos y RTO/RPO medidos en H08.
- [ ] Backups cifrados y acceso mínimo.
- [ ] Borrado cuenta respeta retención documentada.
- [ ] Terraform state recuperable sin secretos expuestos.

### Riesgos, economía y aprobaciones

- [ ] Resuelto o escalado formalmente: PITR/backup adicional de pago.
- [ ] Resuelto o escalado formalmente: Pérdida de datos al eliminar tabla.
- [ ] Resuelto o escalado formalmente: Restore incorrecto de aportaciones divididas.
- [ ] Costos potenciales (incluido apagado) identificados por servicio en I-000; no se afirma $0 por usar créditos.
- [ ] SCP y disponibilidad de servicios relevantes documentados (no simulados).
- [ ] Gate I-GO tiene resolución expresa del responsable, o esta spec continúa DRAFT/BLOCKED.

### Ejecución futura H08 (NO ejecutada en este lote)

- [ ] Pruebas AWS/CI/HTTP y sus evidencias ejecutadas realmente tras aprobación y release; no cambiar a PASS antes.
