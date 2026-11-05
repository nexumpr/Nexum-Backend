# I-010-red-vpc — Red VPC, zonas, seguridad y conectividad

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-020, A-030, A-050, I-000 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## Gestión de tareas H05 (solo especificación y aceptación de diseño)

Estado inicial de todos los trabajos: `TODO`, salvo evidencia local de integridad H04 documentada por el responsable. No marcar como completado por generar archivos.

### T-010-01 — Cerrar topología mínima

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-020, A-030, A-050, I-000.
- **Trabajo:** VPC dedicada no superpuesta; al menos dos subredes/AZ si ALB lo exige, SG de entrada ALB:443, backend solo desde SG del ALB, Redis solo desde SG del backend, egress por destino justificable. Revisar quotas por zona.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-010-02 — Cerrar puertos y flujos

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-020, A-030, A-050, I-000.
- **Trabajo:** Cliente→ALB TLS443; ALB→Go en puerto interno elegido para health + API; Go→DynamoDB vía AWS SDK con IAM, Go→Redis TCP TLS; nodos→logs/actualizaciones por ruta controlada. Documentar por matriz origen-destino.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-010-03 — Cerrar nat e ip

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-020, A-030, A-050, I-000.
- **Trabajo:** NAT Gateway, endpoints privados, IPv4 públicas y tráfico de salida tienen costos o requisitos específicos; comparar rutas sin NAT y con endpoints, sin forzar IP pública a las dos instancias ni ocultar costo de IPv4.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-010-04 — Cerrar identidades y datos

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-020, A-030, A-050, I-000.
- **Trabajo:** TLS del cliente termina en ALB; si políticas exigen cifrado tránsito ALB→Go, instrumentar TLS interno y certificados gestionados. Redis autenticado y cifrado; no exponer puertos administrativos públicamente.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-010-05 — Cerrar controles de seguridad

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-020, A-030, A-050, I-000.
- **Trabajo:** SG least privilege, no SSH público, acceso operativo temporal aprobado (SSM si autorizado) o canal compatible con SCP; registros de acceso con retención y PII mínima.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-010-06 — Cerrar alcance

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-020, A-030, A-050, I-000.
- **Trabajo:** Terraform futuro creará red/SG/rutas desde módulos; aquí únicamente definición. Evitar Default VPC como dependencia implícita o mezcla con redes de otros proyectos.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-010-90 — Revisión de riesgos y dependencias

- **Estado:** TODO.
- **Trabajo:** resolver o escalar: Costo NAT/endpoints e IPv4 inadvertido, AZ insuficientes en una cuenta Free, Rutas sin salida que dejan nodos irrecuperables.
- **Criterio DoD:** cada riesgo clasificado RESOLVED/BLOCKED/ACCEPTED con firma/fecha; no usar ACCEPTED para incumplir requisito docente sin aclaración.

### T-010-99 — Ratificación de la especificación

- **Estado:** REVIEW pendiente.
- **Entrada:** `spec.md`, `plan.md`, `tasks.md`, `checklist.md` actualizados.
- **Salida:** decisión explícita del responsable y vínculo a evidencia en informe I-GO.
- **Bloqueo:** no crear Terraform/endpoints ni ejecutar AWS.
