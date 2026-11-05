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

- **Estado:** REVIEW pendiente.
- **Entrada:** `spec.md`, `plan.md`, `tasks.md`, `checklist.md` actualizados.
- **Salida:** decisión explícita del responsable y vínculo a evidencia en informe I-GO.
- **Bloqueo:** no crear Terraform/endpoints ni ejecutar AWS.
