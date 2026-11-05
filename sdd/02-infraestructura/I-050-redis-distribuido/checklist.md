# I-050-redis-distribuido — Redis compartido real y seguridad

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-080, I-000, I-010, I-020 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## Checklist de revisión (cada punto requiere evidencia, no basta la presencia de este archivo)

### Integridad y autoridad

- [ ] H04 tiene aprobación humana y el contenido coincide con las reglas superiores.
- [ ] Se analizó el PDF docente v3 y el catálogo V6.0.2 sin asumir GCP/MongoDB.
- [ ] Se mantiene el alcance de Backend, no se exige validación Mobile.
- [ ] No se crearon recursos ni artefactos de código/EP antes de H08/H06.

### Decisiones técnicas

- [ ] `INF-050-01` **Servicio:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-050-02` **Topología:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-050-03` **Red:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-050-04` **Objetivos:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-050-05` **Fallo:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-050-06` **Prueba compartida:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-050-07` **Costo:** diseño justificado, referencias, supuestos y evidencia trazable.

### Pruebas y cobertura de diseño

- [ ] 2 nodos acceden a mismo Redis; prueba compartida posterior.
- [ ] TLS/ACL y networking documentados.
- [ ] Lista de claves, TTL, invalidación por evento y rate limiting.
- [ ] Comportamiento ante caída: error/circuit-breaker consistente.
- [ ] Costo mensual y disponibilidad Free/SCP evidenciados.

### Riesgos, economía y aprobaciones

- [ ] Resuelto o escalado formalmente: ElastiCache no habilitado.
- [ ] Resuelto o escalado formalmente: Autogestión agrega tercera instancia con costo.
- [ ] Resuelto o escalado formalmente: Redis sin TLS/ACL.
- [ ] Resuelto o escalado formalmente: Pérdida de caché o throttle genera inconsistencia.
- [ ] Costos potenciales (incluido apagado) identificados por servicio en I-000; no se afirma $0 por usar créditos.
- [ ] SCP y disponibilidad de servicios relevantes documentados (no simulados).
- [ ] Gate I-GO tiene resolución expresa del responsable, o esta spec continúa DRAFT/BLOCKED.

### Ejecución futura H08 (NO ejecutada en este lote)

- [ ] Pruebas AWS/CI/HTTP y sus evidencias ejecutadas realmente tras aprobación y release; no cambiar a PASS antes.
