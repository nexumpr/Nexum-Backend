# I-030-balanceo-https — Balanceador real, TLS, DNS, health y failover

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-030, I-000, I-010, I-020 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## Checklist de revisión (cada punto requiere evidencia, no basta la presencia de este archivo)

### Integridad y autoridad

- [ ] H04 tiene aprobación humana y el contenido coincide con las reglas superiores.
- [ ] Se analizó el PDF docente v3 y el catálogo V6.0.2 sin asumir GCP/MongoDB.
- [ ] Se mantiene el alcance de Backend, no se exige validación Mobile.
- [ ] No se crearon recursos ni artefactos de código/EP antes de H08/H06.

### Decisiones técnicas

- [ ] `INF-030-01` **Alternativa candidata:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-030-02` **Certificado:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-030-03` **Health:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-030-04` **Redirección:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-030-05` **Failover:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-030-06` **Evidencias:** diseño justificado, referencias, supuestos y evidencia trazable.

### Pruebas y cobertura de diseño

- [ ] URL pública con certificado de CA confiable, TLS 1.2+.
- [ ] Distribución a dos targets simultáneos demostrada después del deploy.
- [ ] Failover de un nodo sin pérdida de persistencia.
- [ ] Cálculo ALB hourly+LCU y uso IPv4 / DNS verificado.
- [ ] No publicar credenciales ni origen interno en headers.

### Riesgos, economía y aprobaciones

- [ ] Resuelto o escalado formalmente: ALB no habilitado en plan Free concreto.
- [ ] Resuelto o escalado formalmente: Certificado/dominio no disponibles.
- [ ] Resuelto o escalado formalmente: ALB mantiene cobro cuando los nodos están apagados.
- [ ] Costos potenciales (incluido apagado) identificados por servicio en I-000; no se afirma $0 por usar créditos.
- [ ] SCP y disponibilidad de servicios relevantes documentados (no simulados).
- [ ] Gate I-GO tiene resolución expresa del responsable, o esta spec continúa DRAFT/BLOCKED.

### Ejecución futura H08 (NO ejecutada en este lote)

- [ ] Pruebas AWS/CI/HTTP y sus evidencias ejecutadas realmente tras aprobación y release; no cambiar a PASS antes.
