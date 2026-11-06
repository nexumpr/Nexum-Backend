# I-020-computo-go — Dos instancias Go reales, sistema y hardening

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-020, A-030, A-090, I-000, I-010 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## Checklist de revisión (cada punto requiere evidencia, no basta la presencia de este archivo)

### Integridad y autoridad

- [ ] H04 tiene aprobación humana y el contenido coincide con las reglas superiores.
- [ ] Se analizó el PDF docente v3 y el catálogo V6.0.2 sin asumir GCP/MongoDB.
- [ ] Se mantiene el alcance de Backend, no se exige validación Mobile.
- [ ] No se crearon recursos ni artefactos de código/EP antes de H08/H06.

### Decisiones técnicas

- [ ] `INF-020-01` **Cómputo:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-020-02` **Binario:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-020-03` **Bootstrap:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-020-04` **Salud:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-020-05` **Simultaneidad comprobable:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-020-06` **Capacidad:** diseño justificado, referencias, supuestos y evidencia trazable.

### Pruebas y cobertura de diseño

- [ ] Diseño Launch Template/ASG deseado2 y roles sin hardcoded AMI.
- [ ] Demostración posterior de 2 instance-id simultáneos y healthy en target group.
- [ ] Prueba posterior de reinicio de un nodo y continuidad con otro.
- [ ] Endpoint health sin secretos y logs correlacionados.
- [ ] Costo real de los dos nodos incluido en 45 días y ventanas.

### Riesgos, economía y aprobaciones

- [ ] Resuelto o escalado formalmente: Plan Free limita cantidad/tipo EC2.
- [ ] Resuelto o escalado formalmente: Dos instancias agotan presupuesto.
- [ ] Resuelto o escalado formalmente: Inicio fallido por egress/descarga de artefactos.
- [ ] Resuelto o escalado formalmente: Desplegar nueva versión en un solo nodo deja incompatibilidad.
- [ ] Costos potenciales (incluido apagado) identificados por servicio en I-000; no se afirma $0 por usar créditos.
- [ ] SCP y disponibilidad de servicios relevantes documentados (no simulados).
- [ ] Gate I-GO tiene resolución expresa del responsable, o esta spec continúa DRAFT/BLOCKED.

### Ejecución futura H08 (NO ejecutada en este lote)

- [ ] Pruebas AWS/CI/HTTP y sus evidencias ejecutadas realmente tras aprobación y release; no cambiar a PASS antes.

### Verificacion H08-I — 2026-11-06

- [x] Launch Template con AMI oficial por parametro SSM y sin id fijado.
- [x] `t4g.micro` ARM64 y `cpu_credits = standard` explicito.
- [x] IMDSv2 obligatorio (`http_tokens = required`, hop limit 1).
- [x] EBS cifrado, gp3, 15 GB por instancia.
- [x] Sin clave SSH y sin IPv4 publica.
- [x] SG del backend proveniente de I-010; instance profile por variable (I-060).
- [x] ASG min=desired=max=2, dos subredes/2 AZ, balanced-only, instance refresh 50 %.
- [x] user-data sin secretos, descarga de S3 con SHA-256 e instalacion atomica.
- [x] Unidad systemd endurecida (usuario no root, ProtectSystem=strict, MemoryMax=512M).
- [x] Compilacion real linux/arm64 con manifiesto y SHA-256.
- [ ] Garantia estricta de una instancia por AZ — depende de capacidad AWS (NOT_RUN_AWS).
- [ ] Endpoint ELB / target group — corresponde a I-030.
- [ ] **BLOQUEO DE DESPLIEGUE**: RFC-004 (secretos en reemplazo automatico de instancias).

### Correccion de cierre

- [x] La unidad systemd se instala desde S3 con SHA-256 verificado.
- [x] Binario y directorio propiedad de root; el usuario del servicio no puede modificarlos.
- [x] El bootstrap falla si falta unidad, binario o `/etc/nexum/backend.env`.
- [x] Comprueba servicio activo y liveness local; si no, `exit 1`.
- [x] Instance refresh con `min_healthy_percentage = 100`.
- [x] Validacion estricta de `sg_backend_id`.
- [x] Script de build restaura el entorno.
- [ ] **D-01** Rutas de salud no registradas en el router Go — bloquea I-030/despliegue.
- [ ] **RFC-004** pendiente de aprobacion humana — bloquea despliegue.
- [ ] NOT_RUN_AWS / NOT_RUN_LINUX: sin instancias ni systemd real.
