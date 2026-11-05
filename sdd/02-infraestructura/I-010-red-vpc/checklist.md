# I-010-red-vpc — Red VPC, zonas, seguridad y conectividad

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-020, A-030, A-050, I-000 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## Checklist de revisión (cada punto requiere evidencia, no basta la presencia de este archivo)

### Integridad y autoridad

- [ ] H04 tiene aprobación humana y el contenido coincide con las reglas superiores.
- [ ] Se analizó el PDF docente v3 y el catálogo V6.0.2 sin asumir GCP/MongoDB.
- [ ] Se mantiene el alcance de Backend, no se exige validación Mobile.
- [ ] No se crearon recursos ni artefactos de código/EP antes de H08/H06.

### Decisiones técnicas

- [ ] `INF-010-01` **Topología mínima:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-010-02` **Puertos y flujos:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-010-03` **NAT e IP:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-010-04` **Identidades y datos:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-010-05` **Controles de seguridad:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-010-06` **Alcance:** diseño justificado, referencias, supuestos y evidencia trazable.

### Pruebas y cobertura de diseño

- [ ] Diagrama red con CIDR, subredes, SG e interfaces incluidas.
- [ ] Matriz de flujos con pruebas positivas/negativas documentadas.
- [ ] Ruta de salida de backend compatible con DynamoDB, logs y actualización segura.
- [ ] Sin exposición de Redis ni puertos administrativos al público.
- [ ] Estimación de NAT/endpoints/IP en matriz I-000, incluso cuando se elija no usarlos.

### Riesgos, economía y aprobaciones

- [ ] Resuelto o escalado formalmente: Costo NAT/endpoints e IPv4 inadvertido.
- [ ] Resuelto o escalado formalmente: AZ insuficientes en una cuenta Free.
- [ ] Resuelto o escalado formalmente: Rutas sin salida que dejan nodos irrecuperables.
- [ ] Costos potenciales (incluido apagado) identificados por servicio en I-000; no se afirma $0 por usar créditos.
- [ ] SCP y disponibilidad de servicios relevantes documentados (no simulados).
- [ ] Gate I-GO tiene resolución expresa del responsable, o esta spec continúa DRAFT/BLOCKED.

### Ejecución futura H08 (NO ejecutada en este lote)

- [ ] Pruebas AWS/CI/HTTP y sus evidencias ejecutadas realmente tras aprobación y release; no cambiar a PASS antes.
