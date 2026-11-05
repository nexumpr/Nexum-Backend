# I-030-balanceo-https — Balanceador real, TLS, DNS, health y failover

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-030, I-000, I-010, I-020 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## Gestión de tareas H05 (solo especificación y aceptación de diseño)

Estado inicial de todos los trabajos: `TODO`, salvo evidencia local de integridad H04 documentada por el responsable. No marcar como completado por generar archivos.

### T-030-01 — Cerrar alternativa candidata

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-030, I-000, I-010, I-020.
- **Trabajo:** Application Load Balancer + target group EC2 con 2 targets y listener HTTPS:443 usando ACM público, reglas mínimas; opción concreta requiere precio/servicio permitido. No agregar API Gateway redundante sin ADR.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-030-02 — Cerrar certificado

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-030, I-000, I-010, I-020.
- **Trabajo:** ACM público para nombre de dominio bajo control del proyecto, validación DNS, SNI/TLS 1.2+ y renovación; sin certificado autofirmado en URL pública. DNS o proveedor externo se costea y documenta.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-030-03 — Cerrar health

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-030, I-000, I-010, I-020.
- **Trabajo:** Health interval/timeout/threshold y matcher 200 parametrizados, liveness separado de readiness. Estado healthy por instancia consultable; detectar 5xx y target unhealth.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-030-04 — Cerrar redirección

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-030, I-000, I-010, I-020.
- **Trabajo:** HTTP:80 solo si hay decisión de redirección permanente a HTTPS y costo/SG permitido; de otra forma bloquear puerto 80. HSTS donde dominio/operación lo permitan, forwarded headers solo de proxies confiables.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-030-05 — Cerrar failover

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-030, I-000, I-010, I-020.
- **Trabajo:** En prueba autorizada H08, impedir servicio de un nodo, health lo marca unhealthy y tráfico continúa al otro. Recuperar y comprobar retorno de ambos healthy; no demostrar solo round-robin.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-030-06 — Cerrar evidencias

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-030, I-000, I-010, I-020.
- **Trabajo:** curl HTTPS con validación de certificado, cabeceras, status/latencia y requestId; listado de target health con IDs distintos; nombres de dominio/ALB reales y factura/costos de ALB.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-030-90 — Revisión de riesgos y dependencias

- **Estado:** TODO.
- **Trabajo:** resolver o escalar: ALB no habilitado en plan Free concreto, Certificado/dominio no disponibles, ALB mantiene cobro cuando los nodos están apagados.
- **Criterio DoD:** cada riesgo clasificado RESOLVED/BLOCKED/ACCEPTED con firma/fecha; no usar ACCEPTED para incumplir requisito docente sin aclaración.

### T-030-99 — Ratificación de la especificación

- **Estado:** REVIEW pendiente.
- **Entrada:** `spec.md`, `plan.md`, `tasks.md`, `checklist.md` actualizados.
- **Salida:** decisión explícita del responsable y vínculo a evidencia en informe I-GO.
- **Bloqueo:** no crear Terraform/endpoints ni ejecutar AWS.
