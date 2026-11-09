# I-120-encendido-apagado — Encendido, apagado y preservación de datos

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | I-000, I-020, I-030, I-040, I-050 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## Checklist de revisión (cada punto requiere evidencia, no basta la presencia de este archivo)

### Integridad y autoridad

- [ ] H04 tiene aprobación humana y el contenido coincide con las reglas superiores.
- [ ] Se analizó el PDF docente v3 y el catálogo V6.0.2 sin asumir GCP/MongoDB.
- [ ] Se mantiene el alcance de Backend, no se exige validación Mobile.
- [ ] No se crearon recursos ni artefactos de código/EP antes de H08/H06.

### Decisiones técnicas

- [ ] `INF-120-01` **Estados:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-120-02` **Encender:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-120-03` **Apagar:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-120-04` **DNS/ALB:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-120-05` **Seguridad:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-120-06` **Prueba:** diseño justificado, referencias, supuestos y evidencia trazable.

### Pruebas y cobertura de diseño

- [ ] Control de estados on/off y acciones por recurso.
- [ ] Nunca pérdida de DynamoDB ni secretos durante apagado.
- [ ] No publicar servicio con 1 nodo fingiendo doble instancia.
- [ ] Costo residual enumerado tras OFF.
- [ ] Prueba restart/restauración desde OFF con datos existentes.

### Riesgos, economía y aprobaciones

- [ ] Resuelto o escalado formalmente: Apagado incompleto mantiene ALB/Redis facturados.
- [ ] Resuelto o escalado formalmente: Destroy accidental de datos.
- [ ] Resuelto o escalado formalmente: Gastos mientras runtime OFF.
- [ ] Costos potenciales (incluido apagado) identificados por servicio en I-000; no se afirma $0 por usar créditos.
- [ ] SCP y disponibilidad de servicios relevantes documentados (no simulados).
- [ ] Gate I-GO tiene resolución expresa del responsable, o esta spec continúa DRAFT/BLOCKED.

### Ejecución futura H08 (NO ejecutada en este lote)

- [ ] Pruebas AWS/CI/HTTP y sus evidencias ejecutadas realmente tras aprobación y release; no cambiar a PASS antes.

## Verificacion de implementacion local (2026-11-09)

- [x] Modulo Terraform de guardas sin recursos AWS (`infra/terraform/modules/operacion`).
- [x] Inventario declarativo unico leido por Terraform y por el operador.
- [x] Allowlist auxiliar explicita y separada del inventario.
- [x] Deny por defecto ante recurso sin etiqueta, desconocido o incoherente.
- [x] Core marcado como suspendible falla el plan y se deniega en el operador.
- [x] `destroy-data`, `destroy-state` y `destroy-all` rechazados por diseno.
- [x] Doble instancia exigida para publicar (`targets_sanos >= 2`).
- [x] Residual calculado con `Decimal`, separado en residente/presupuestado/variable/bloqueado.
- [x] Umbrales 45/35/20 aplicados; sin saldo verificado se declara BLOQUEADO.
- [x] 54 pruebas del operador, 27 de contrato Terraform y 7 de alineacion FinOps.
- [ ] Ejecucion real OFF/ON con inventario y facturacion: `NOT_RUN` (requiere Gate de despliegue).
- [ ] Backups de DynamoDB: bloqueados por I-130 `NOT_AUTHORIZED`.
