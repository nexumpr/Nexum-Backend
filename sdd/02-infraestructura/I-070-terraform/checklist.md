# I-070-terraform — IaC Terraform: stacks, state, drift y seguridad

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-110, I-000..I-060 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## Checklist de revisión (cada punto requiere evidencia, no basta la presencia de este archivo)

### Integridad y autoridad

- [ ] H04 tiene aprobación humana y el contenido coincide con las reglas superiores.
- [ ] Se analizó el PDF docente v3 y el catálogo V6.0.2 sin asumir GCP/MongoDB.
- [ ] Se mantiene el alcance de Backend, no se exige validación Mobile.
- [ ] No se crearon recursos ni artefactos de código/EP antes de H08/H06.

### Decisiones técnicas

- [ ] `INF-070-01` **Roots y módulos:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-070-02` **State:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-070-03` **Plan y lock:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-070-04` **Drift:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-070-05` **Secuencia:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-070-06` **Condiciones:** diseño justificado, referencias, supuestos y evidencia trazable.

### Pruebas y cobertura de diseño

- [ ] Lista exhaustiva de recursos/variables/outputs y dependencias.
- [ ] Plan reproducible y sin secretos en H08.
- [ ] Gates humanos before apply/destroy, backend/state preservado.
- [ ] Drift identificable, rollback de IaC definido.
- [ ] Modelos completos de tables/GSI y Redis sin reemplazos.

### Riesgos, economía y aprobaciones

- [ ] Resuelto o escalado formalmente: Servicio backend state cobrable o SCP bloqueado.
- [ ] Resuelto o escalado formalmente: Lock incompatible o terraform version drift.
- [ ] Resuelto o escalado formalmente: Destroy borra datos y provoca pérdida.
- [ ] Costos potenciales (incluido apagado) identificados por servicio en I-000; no se afirma $0 por usar créditos.
- [ ] SCP y disponibilidad de servicios relevantes documentados (no simulados).
- [ ] Gate I-GO tiene resolución expresa del responsable, o esta spec continúa DRAFT/BLOCKED.

### Ejecución futura H08 (NO ejecutada en este lote)

- [ ] Pruebas AWS/CI/HTTP y sus evidencias ejecutadas realmente tras aprobación y release; no cambiar a PASS antes.

### Gate de implementacion local I-070 (2026-11-08)

- [x] APRUEBO_GATE_I070_LOCAL concedido y registrado.
- [x] Bootstrap declarado: bucket de state versionado, cifrado, privado y protegido.
- [x] prevent_destroy en el state y politica que exige TLS; contrato IAM de state/tflock disenado.
- [x] Backend remoto NO activado; plantilla .example inactiva y documentada.
- [x] fmt -check, init -backend=false y validate del bootstrap y del entorno en verde.
- [x] Locking nativo de S3 (use_lockfile, Terraform >= 1.10); sin tabla DynamoDB.
- [ ] Aplicacion real del bootstrap y activacion del backend: requieren Gate de infraestructura y credenciales humanas.
- [ ] PR unico revisado; merge y primer apply requieren autorizacion humana independiente.
- [x] 22 pruebas de contrato I-070 (218 en la suite).
- [x] Custodia y migracion del state del bootstrap documentadas (RUNBOOK_IAC.md 2-4), con separacion por clave y recuperacion por importacion.
- [x] 29 pruebas de contrato I-070 (225 en la suite).
- [x] Corregida la secuencia de custodia/migracion: el state original no se mueve antes de migrar; respaldo cifrado verificado por SHA-256.
- [x] Criterio de importacion corregido: N to import con 0 add/change/replace/destroy y direcciones coincidentes con el codigo.
- [x] Inventario declarativo de 8 direcciones (7 AWS + terraform_data), sin cifras historicas fijas.
- [x] 33 pruebas de contrato I-070 (229 en la suite Terraform).
