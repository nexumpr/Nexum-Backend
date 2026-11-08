# I-060-iam-seguridad — IAM, límites SCP, secretos y accesos

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-050, I-000, I-010, I-020, I-040, I-050 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## Checklist de revisión (cada punto requiere evidencia, no basta la presencia de este archivo)

### Integridad y autoridad

- [ ] H04 tiene aprobación humana y el contenido coincide con las reglas superiores.
- [ ] Se analizó el PDF docente v3 y el catálogo V6.0.2 sin asumir GCP/MongoDB.
- [ ] Se mantiene el alcance de Backend, no se exige validación Mobile.
- [ ] No se crearon recursos ni artefactos de código/EP antes de H08/H06.

### Decisiones técnicas

- [ ] `INF-060-01` **Identidades:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-060-02` **SCP:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-060-03` **Secretos:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-060-04` **Least privilege:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-060-05` **GitHub OIDC:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-060-06` **Auditoría:** diseño justificado, referencias, supuestos y evidencia trazable.

### Pruebas y cobertura de diseño

- [ ] Matriz principal→acción→recurso con IAM mínimo.
- [ ] SCP detectado no se elude: decisión responsable.
- [ ] OIDC trust política restringe repo/branch/environment.
- [ ] No hay claves persistentes o secretos GitHub para AWS.
- [ ] Retiro de credenciales y revocación de emergencia documentados.

### Riesgos, economía y aprobaciones

- [ ] Resuelto o escalado formalmente: SCP bloquea IAM/EC2/ALB.
- [ ] Resuelto o escalado formalmente: GitHub OIDC no permitido.
- [ ] Resuelto o escalado formalmente: Servicios secretos con tarifa.
- [ ] Resuelto o escalado formalmente: Estado Terraform contiene secretos.
- [ ] Costos potenciales (incluido apagado) identificados por servicio en I-000; no se afirma $0 por usar créditos.
- [ ] SCP y disponibilidad de servicios relevantes documentados (no simulados).
- [ ] Gate I-GO tiene resolución expresa del responsable, o esta spec continúa DRAFT/BLOCKED.

### Ejecución futura H08 (NO ejecutada en este lote)

- [ ] Pruebas AWS/CI/HTTP y sus evidencias ejecutadas realmente tras aprobación y release; no cambiar a PASS antes.

### Gate de implementación local I-060 (2026-11-07)

- [x] `APRUEBO_GATE_I060_LOCAL` concedido explícitamente y registrado.
- [x] Rol EC2 runtime + instance profile declarados y conectados a I-020.
- [x] Política DynamoDB exacta por tabla/índices con `LeadingKeys`; sin Scan ni administración.
- [x] Trust OIDC separado con subjects inmutables y `aud` exactos; roles OIDC no activos.
- [x] `fmt -check`, `init -backend=false` y `validate` ejecutados con salida real (Success).
- [x] 18 pruebas de contrato IAM en verde (183 en la suite).
- [ ] Verificación AWS real de SCP, OIDC provider, environments y permisos efectivos: `NOT_VERIFIED`.
- [ ] Secretos y state resueltos: **bloqueado por RFC-004** y por el riesgo de contraseña Redis en state.
- [ ] PR único revisado; merge y despliegue requieren autorización humana independiente.
