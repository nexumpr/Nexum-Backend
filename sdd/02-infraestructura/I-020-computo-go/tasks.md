# I-020-computo-go — Dos instancias Go reales, sistema y hardening

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-020, A-030, A-090, I-000, I-010 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## Gestión de tareas H05 (solo especificación y aceptación de diseño)

Estado inicial de todos los trabajos: `TODO`, salvo evidencia local de integridad H04 documentada por el responsable. No marcar como completado por generar archivos.

### T-020-01 — Cerrar cómputo

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-020, A-030, A-090, I-000, I-010.
- **Trabajo:** Evaluar EC2+Launch Template/Auto Scaling Group con desired=2,min=2,max=2 mientras el servicio esté encendido. Dos procesos en la misma VM o un contenedor duplicado en un host NO satisfacen el requisito. Tipo/AMI/arquitectura se eligen según precio, capacidad y disponibilidad real.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-020-02 — Cerrar binario

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-020, A-030, A-090, I-000, I-010.
- **Trabajo:** Build Go con dependencias fijadas y hash SHA256 por release, servicio systemd de usuario no privilegiado, read-only filesystem en lo posible, umask, restart bounded, graceful shutdown de HTTP con timeout, límites de memoria/archivos.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-020-03 — Cerrar bootstrap

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-020, A-030, A-090, I-000, I-010.
- **Trabajo:** User-data mínimo, no secreto en scripts ni logs, artefactos verificados por digest, identidad de instancia via IAM role, config por entorno y secret retrieval seguro si el servicio está permitido.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-020-04 — Cerrar salud

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-020, A-030, A-090, I-000, I-010.
- **Trabajo:** GET /health/live solo proceso; /health/ready comprueba condiciones esenciales con timeout. ALB retira destino no ready; error de Redis se degrada según A-080 sin servir datos incorrectos; dependencia Dynamo crítica hace no-ready.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-020-05 — Cerrar simultaneidad comprobable

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-020, A-030, A-090, I-000, I-010.
- **Trabajo:** Capturar dos instance-ids, dos IPs privadas distintas, ambos targets healthy concurrentemente, contar solicitudes a cada nodo mediante trace/nodo-ID seguro, aislar un nodo y demostrar servicio del otro sin perder estado.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-020-06 — Cerrar capacidad

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-020, A-030, A-090, I-000, I-010.
- **Trabajo:** Plan de carga básico: RAM/CPU p95, conexiones Dynamo/Redis, timeout, tamaño de binario y carga a 2 nodos; no cambiar a un solo nodo por ahorro.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-020-90 — Revisión de riesgos y dependencias

- **Estado:** TODO.
- **Trabajo:** resolver o escalar: Plan Free limita cantidad/tipo EC2, Dos instancias agotan presupuesto, Inicio fallido por egress/descarga de artefactos, Desplegar nueva versión en un solo nodo deja incompatibilidad.
- **Criterio DoD:** cada riesgo clasificado RESOLVED/BLOCKED/ACCEPTED con firma/fecha; no usar ACCEPTED para incumplir requisito docente sin aclaración.

### T-020-99 — Ratificación de la especificación

- **Estado:** REVIEW pendiente.
- **Entrada:** `spec.md`, `plan.md`, `tasks.md`, `checklist.md` actualizados.
- **Salida:** decisión explícita del responsable y vínculo a evidencia en informe I-GO.
- **Bloqueo:** no crear Terraform/endpoints ni ejecutar AWS.
