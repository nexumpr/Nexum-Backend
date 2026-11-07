# I-050-redis-distribuido — Redis compartido real y seguridad

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-080, I-000, I-010, I-020 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## Gestión de tareas H05 (solo especificación y aceptación de diseño)

Estado inicial de todos los trabajos: `TODO`, salvo evidencia local de integridad H04 documentada por el responsable. No marcar como completado por generar archivos.

### T-050-01 — Cerrar servicio

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-080, I-000, I-010, I-020.
- **Trabajo:** Evaluar ElastiCache Redis OSS compatible con plan/costo; si no está permitido, evaluar Redis OSS autogestionado en una tercera instancia aprobada, aislada y presupuestada. Valkey no debe reemplazar Redis sin ADR humano explícito si la exigencia es Redis.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-050-02 — Cerrar topología

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-080, I-000, I-010, I-020.
- **Trabajo:** Una única instancia Redis compartida representa punto único de fallo: registrar RTO y degradación. Para HA, nodos adicionales + réplica/sentinel y costo adicional; no afirmar alta disponibilidad sin prueba ni financiación.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-050-03 — Cerrar red

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-080, I-000, I-010, I-020.
- **Trabajo:** Redis privado, SG solo Go, TLS habilitado end-to-end, ACL/usuario dedicado y credenciales rotables; no puertos públicos, no claves de usuario en texto claro.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-050-04 — Cerrar objetivos

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-080, I-000, I-010, I-020.
- **Trabajo:** Cachear catálogos 1h, dashboard 20-60s, analytics 1-5min (semillas); claves con versión + HMAC usuario, TTL, invalidación after commit; atomizar rate-limits login/requests con Lua o primitivas seguras.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-050-05 — Cerrar fallo

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-080, I-000, I-010, I-020.
- **Trabajo:** DynamoDB mantiene fuente de verdad; caché no puede devolver saldo antiguo después de una mutación. Auth/rate limit sensibles fallan cerrado según A-080; lecturas públicas pueden degradarse.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-050-06 — Cerrar prueba compartida

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-080, I-000, I-010, I-020.
- **Trabajo:** Nodo A escribe clave temporal; nodo B la ve; invalidación realizada por nodo A se observa en B; reinicio de nodo verifica impacto y se detecta caída con métricas.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-050-07 — Cerrar costo

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-080, I-000, I-010, I-020.
- **Trabajo:** Comparar ElastiCache, instancia EC2 dedicada y transferencia/almacenamiento/backup; preferir servicio soportado, no dejar Redis para «después». Evidencia de capacidad real antes de aprobar I-GO.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-050-90 — Revisión de riesgos y dependencias

- **Estado:** TODO.
- **Trabajo:** resolver o escalar: ElastiCache no habilitado, Autogestión agrega tercera instancia con costo, Redis sin TLS/ACL, Pérdida de caché o throttle genera inconsistencia.
- **Criterio DoD:** cada riesgo clasificado RESOLVED/BLOCKED/ACCEPTED con firma/fecha; no usar ACCEPTED para incumplir requisito docente sin aclaración.

### T-050-99 — Ratificación de la especificación

- **Estado:** **IMPLEMENTADO_LOCAL 2026-11-07** (Gate `APRUEBO_GATE_I050_LOCAL`); falta el Gate de despliegue AWS.
- **Entrada:** `spec.md`, `plan.md`, `tasks.md`, `checklist.md` actualizados.
- **Salida:** decisión explícita del responsable y vínculo a evidencia en informe I-GO.
- **Bloqueo:** no crear Terraform/endpoints ni ejecutar AWS.

## Implementación H08-I (2026-11-07) — no reescribe el estado H05

Gate `APRUEBO_GATE_I050_LOCAL`; registro en `sdd/_control/H08/GATE_H08I_I050.md` y ADR de proveedor en
`sdd/_control/H08/ADR_REDIS_PROVEEDOR.md`. Rama `feat/h08i-i050-redis`. `AWS_NOT_RUN`.

| Tarea | Estado real | Evidencia |
|---|---|---|
| T-050-01 Servicio | **IMPLEMENTADO_LOCAL** | `aws_elasticache_replication_group` engine `redis` 7.1; decisión heredada de I-000 (SKU `VERIFIED_AWS`), no elegida en I-050 |
| T-050-02 Topología | **IMPLEMENTADO_LOCAL** | 1 nodo `cache.t4g.micro`, sin réplicas, sin Multi-AZ; precondition impide failover con un nodo; salida `alta_disponibilidad = false` |
| T-050-03 Red | **IMPLEMENTADO_LOCAL** | Subnet group con las 2 subredes de I-010; SG de I-010 (6379 sólo desde backend); TLS + cifrado en reposo obligatorios; RBAC dedicado con contraseña sensible; **usuario `default` sustituido por uno deshabilitado (`off ~* -@all`)**; ACL con categorías oficiales (`@transaction`, no `@transactions`) y denegación de `KEYS`/`SCAN`/`RANDOMKEY`/`RESTORE`/`MIGRATE`/`SORT` |
| T-050-04 Objetivos | **APP (A-080)** | TTL/versión/HMAC en claves y rate limiting atómico viven en la aplicación; la caché aporta política `volatile-lru` |
| T-050-05 Fallo | **APP (A-080) + DISEÑO** | DynamoDB es la verdad; sin snapshots; fail-close/degradación por operación en el cliente |
| T-050-06 Prueba compartida | **NOT_RUN** | Requiere AWS; el endpoint único y el SG compartido hacen posible la prueba desde ambos nodos |
| T-050-07 Costo | **PRESUPUESTADO** | 17.2800 USD/45 d (1080 × 0.016), sin partidas nuevas; réplicas exigirían recotización |
| T-050-90 Riesgos | **CLASIFICADOS** | R01 mitigado parcialmente (motor/precio verificados, creación `NOT_RUN`), R02 no aplicable, R03 resuelto en diseño, R04 en A-080; SPOF declarado |
| T-050-99 Ratificación | **IMPLEMENTADO_LOCAL** | Gate registrado; despliegue y merge requieren autorización específica |

**Bloqueo de despliegue registrado:** la contraseña RBAC debe entregarse por un mecanismo aprobado; RFC-004 sigue
PROPUESTO y el endpoint `secretsmanager` de I-010 sólo existe en ventana de mantenimiento.
