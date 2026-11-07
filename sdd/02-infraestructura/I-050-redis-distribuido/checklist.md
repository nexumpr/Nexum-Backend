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

- [x] `INF-050-01` **Servicio:** diseño justificado, referencias, supuestos y evidencia trazable.
  — ElastiCache Redis OSS `cache.t4g.micro` (decisión de I-000, ADR `ADR_REDIS_PROVEEDOR.md`); no es evidencia AWS.
- [x] `INF-050-02` **Topología:** diseño justificado, referencias, supuestos y evidencia trazable.
  — 1 nodo sin réplicas ni Multi-AZ; SPOF declarado y `alta_disponibilidad = false`.
- [x] `INF-050-03` **Red:** diseño justificado, referencias, supuestos y evidencia trazable.
  — subnet group privado de I-010, SG de I-010, TLS + cifrado en reposo, usuario RBAC dedicado y **`default` sustituido por uno deshabilitado (`off ~* -@all`)**.
- [ ] `INF-050-04` **Objetivos:** diseño justificado, referencias, supuestos y evidencia trazable.
  — TTL/versión/HMAC y rate limiting son de la aplicación (A-080); la caché aporta `volatile-lru`.
- [ ] `INF-050-05` **Fallo:** diseño justificado, referencias, supuestos y evidencia trazable.
  — DynamoDB como verdad y fail-close en A-080; sin snapshots por diseño.
- [ ] `INF-050-06` **Prueba compartida:** diseño justificado, referencias, supuestos y evidencia trazable.
  — `NOT_RUN`: requiere AWS; el endpoint y SG compartidos habilitan la prueba.
- [x] `INF-050-07` **Costo:** diseño justificado, referencias, supuestos y evidencia trazable.
  — 17.2800 USD/45 d (1080 × 0.016), sin partidas nuevas.

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

### Gate de implementación local I-050 (2026-11-07)

- [x] `APRUEBO_GATE_I050_LOCAL` concedido explícitamente.
- [x] ADR de proveedor (`ADR_REDIS_PROVEEDOR.md`) con alternativas y trazabilidad a I-000.
- [x] Módulo `infra/terraform/modules/redis` escrito y compuesto en `envs/h08i`.
- [x] `fmt -check`, `init -backend=false` y `validate` ejecutados con salida real (Success).
- [x] 27 pruebas de contrato + 8 de alineación FinOps en verde.
- [x] Usuario RBAC de aplicación con ACL restringida y contraseña fuera del repositorio.
- [x] **Usuario `default` de ElastiCache sustituido** por uno deshabilitado (`off ~* -@all`, `no-password-required`) y añadido al grupo; el grupo contiene sólo aplicación + default desactivado.
- [ ] PR único revisado; merge y despliegue requieren autorización humana independiente.
- [ ] Verificación real en AWS (nodo, TLS, RBAC, invalidation entre nodos): `NOT_RUN`.
- [ ] Entrega del secreto RBAC resuelta (RFC-004 + endpoint `secretsmanager` en ventana): **bloqueo de despliegue**.
