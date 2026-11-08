# I-060-iam-seguridad â€” IAM, lÃ­mites SCP, secretos y accesos

| Campo | Valor |
|---|---|
| Etapa | H05 â€” Infraestructura AWS |
| VersiÃ³n | 0.1.0 |
| Estado | **DRAFT / REVIEW â€” NO APROBADO** |
| Trazabilidad | A-050, I-000, I-010, I-020, I-040, I-050 |
| ImplementaciÃ³n | H08+, tras H07 aprobado y autorizaciÃ³n humana por release |

## Checklist de revisiÃ³n (cada punto requiere evidencia, no basta la presencia de este archivo)

### Integridad y autoridad

- [ ] H04 tiene aprobaciÃ³n humana y el contenido coincide con las reglas superiores.
- [ ] Se analizÃ³ el PDF docente v3 y el catÃ¡logo V6.0.2 sin asumir GCP/MongoDB.
- [ ] Se mantiene el alcance de Backend, no se exige validaciÃ³n Mobile.
- [ ] No se crearon recursos ni artefactos de cÃ³digo/EP antes de H08/H06.

### Decisiones tÃ©cnicas

- [ ] `INF-060-01` **Identidades:** diseÃ±o justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-060-02` **SCP:** diseÃ±o justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-060-03` **Secretos:** diseÃ±o justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-060-04` **Least privilege:** diseÃ±o justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-060-05` **GitHub OIDC:** diseÃ±o justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-060-06` **AuditorÃ­a:** diseÃ±o justificado, referencias, supuestos y evidencia trazable.

### Pruebas y cobertura de diseÃ±o

- [ ] Matriz principalâ†’acciÃ³nâ†’recurso con IAM mÃ­nimo.
- [ ] SCP detectado no se elude: decisiÃ³n responsable.
- [ ] OIDC trust polÃ­tica restringe repo/branch/environment.
- [ ] No hay claves persistentes o secretos GitHub para AWS.
- [ ] Retiro de credenciales y revocaciÃ³n de emergencia documentados.

### Riesgos, economÃ­a y aprobaciones

- [ ] Resuelto o escalado formalmente: SCP bloquea IAM/EC2/ALB.
- [ ] Resuelto o escalado formalmente: GitHub OIDC no permitido.
- [ ] Resuelto o escalado formalmente: Servicios secretos con tarifa.
- [ ] Resuelto o escalado formalmente: Estado Terraform contiene secretos.
- [ ] Costos potenciales (incluido apagado) identificados por servicio en I-000; no se afirma $0 por usar crÃ©ditos.
- [ ] SCP y disponibilidad de servicios relevantes documentados (no simulados).
- [ ] Gate I-GO tiene resoluciÃ³n expresa del responsable, o esta spec continÃºa DRAFT/BLOCKED.

### EjecuciÃ³n futura H08 (NO ejecutada en este lote)

- [ ] Pruebas AWS/CI/HTTP y sus evidencias ejecutadas realmente tras aprobaciÃ³n y release; no cambiar a PASS antes.

### Gate de implementaciÃ³n local I-060 (2026-11-07)

- [x] `APRUEBO_GATE_I060_LOCAL` concedido explÃ­citamente y registrado.
- [x] Rol EC2 runtime + instance profile declarados y conectados a I-020.
- [x] PolÃ­tica DynamoDB exacta por tabla/Ã­ndices con `LeadingKeys`; sin Scan ni administraciÃ³n.
- [x] Trust OIDC separado con subjects inmutables y `aud` exactos; roles OIDC no activos.
- [x] `fmt -check`, `init -backend=false` y `validate` ejecutados con salida real (Success).
- [x] 31 pruebas de contrato IAM en verde (196 en la suite), con Query sobre tabla, Null:false y casos negativos.
- [ ] VerificaciÃ³n AWS real de SCP, OIDC provider, environments y permisos efectivos: `NOT_VERIFIED`.
- [ ] Secretos y state resueltos: **bloqueado por RFC-004** y por el riesgo de contraseÃ±a Redis en state.
- [ ] PR Ãºnico revisado; merge y despliegue requieren autorizaciÃ³n humana independiente.
