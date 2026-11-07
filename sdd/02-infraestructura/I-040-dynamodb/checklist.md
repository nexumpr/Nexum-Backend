# I-040-dynamodb — DynamoDB: tablas, claves, GSI, TTL y datos

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-060, A-070, E001..E075, I-000 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## Checklist de revisión (cada punto requiere evidencia, no basta la presencia de este archivo)

### Integridad y autoridad

- [ ] H04 tiene aprobación humana y el contenido coincide con las reglas superiores.
- [ ] Se analizó el PDF docente v3 y el catálogo V6.0.2 sin asumir GCP/MongoDB.
- [ ] Se mantiene el alcance de Backend, no se exige validación Mobile.
- [ ] No se crearon recursos ni artefactos de código/EP antes de H08/H06.

### Decisiones técnicas

- [x] `INF-040-01` **Tabla principal:** diseño justificado, referencias, supuestos y evidencia trazable.
  — implementada `NEXUM_CORE_<ENV>`, `PAY_PER_REQUEST`, `PK`/`SK` (S), SSE, protection por entorno (2026-11-07). No es evidencia AWS.
- [x] `INF-040-02` **Entidades y claves:** diseño justificado, referencias, supuestos y evidencia trazable.
  — prefijos de A-060 ya implementados en Go y verificados contra la matriz H05.
- [ ] `INF-040-03` **Rutas de acceso:** diseño justificado, referencias, supuestos y evidencia trazable.
  — pertenece a la aplicación (A-040/A-060); la tabla no habilita Scan.
- [ ] `INF-040-04` **Unicidad y consistencia:** diseño justificado, referencias, supuestos y evidencia trazable.
  — `EMAIL#<sha256>/OWNER` y transacciones viven en la app (A-060/A-070).
- [ ] `INF-040-05` **Dinero y tiempo:** diseño justificado, referencias, supuestos y evidencia trazable.
  — `int64` centavos y UTC en A-070; sin floats en la tabla.
- [x] `INF-040-06` **TTL y privacidad:** diseño justificado, referencias, supuestos y evidencia trazable.
  — `ttl` sobre `expiresAt`, habilitado; TTL eventual documentado.
- [x] `INF-040-07` **Índices concretos:** diseño justificado, referencias, supuestos y evidencia trazable.
  — `GSI1` justificado (E027–E031); `GSI2` opt-in porque no hay worker programado confirmado.
- [x] `INF-040-08` **Infra Terraform futuro:** diseño justificado, referencias, supuestos y evidencia trazable.
  — `aws_dynamodb_table` + TTL + SSE + PITR + tags, GSI enumerados sin omisiones.

### Pruebas y cobertura de diseño

- [ ] Inventario completo de tablas, PK/SK, GSI y entidades 75-ruta.
- [ ] Pruebas contractuales de ownership, email único y no Scan.
- [ ] Atomicidad/consistencia en contribuciones y pagos; prueba concurrencia.
- [ ] TTL/backup y costos estimados, incluso con tabla vacía.
- [ ] IaC futuro representa todas las claves e índices aprobados y pruebas Postdeploy.

### Riesgos, economía y aprobaciones

- [ ] Resuelto o escalado formalmente: GSI incorrecto obliga a Scan.
- [ ] Resuelto o escalado formalmente: Retención/PITR costosa.
- [ ] Resuelto o escalado formalmente: Borrado de cuenta inconcluso.
- [ ] Resuelto o escalado formalmente: Transacciones sobre 100 elementos superan límites.
- [ ] Costos potenciales (incluido apagado) identificados por servicio en I-000; no se afirma $0 por usar créditos.
- [ ] SCP y disponibilidad de servicios relevantes documentados (no simulados).
- [ ] Gate I-GO tiene resolución expresa del responsable, o esta spec continúa DRAFT/BLOCKED.

### Ejecución futura H08 (NO ejecutada en este lote)

- [ ] Pruebas AWS/CI/HTTP y sus evidencias ejecutadas realmente tras aprobación y release; no cambiar a PASS antes.

### Gate de implementación local I-040 (2026-11-07)

- [x] `APRUEBO_GATE_I040_LOCAL` concedido explícitamente.
- [x] Módulo `infra/terraform/modules/dynamodb` escrito y compuesto en `envs/h08i`.
- [x] `fmt -check`, `init -backend=false` y `validate` ejecutados con salida real (Success).
- [x] 23 pruebas de contrato + 10 de alineación FinOps en verde.
- [ ] PR único revisado; merge y despliegue requieren autorización humana independiente.
- [ ] Verificación real en AWS (tabla, TTL, PITR, consultas sin Scan): `NOT_RUN`.
