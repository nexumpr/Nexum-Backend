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

## Registro de implementacion H08-I — 2026-11-06

- INF-020-01: `infra/terraform/modules/computo` con Launch Template (AMI por parametro publico SSM, ARM64) y ASG min=desired=max=2 sobre las dos subredes de I-010, `availability_zone_distribution = balanced-only`, `protect_from_scale_in`.
- INF-020-02: `scripts/build-backend.ps1` compila `GOOS=linux GOARCH=arm64`, `CGO_ENABLED=0`, `-trimpath`, emite SHA-256 y manifiesto. Ejecutado: binario 2,752,672 bytes, SHA-256 `ff2cc54f...eff1df5`.
- INF-020-03: `user-data.sh.tftpl` descarga el artefacto desde S3 (gateway endpoint), verifica SHA-256, instala atomicamente e inicia systemd; sin secretos y sin descargas de Internet.
- INF-020-04: la unidad publica el puerto 8080; se exponen las rutas `/api/v1/health/live` y `/ready` para el target group de I-030.
- INF-020-05: capacidad fija 2 con preconditions (instancia, capacidad, subredes, SHA, instance profile). La garantia estricta por AZ depende de capacidad real: documentado.
- INF-020-06: t4g.micro (2 vCPU / 1 GiB), `MemoryMax=512M`, disco 15 GB por instancia (30 GB presupuestados).
- Correccion critica: `credit_specification { cpu_credits = "standard" }` explicito (T4g usa unlimited por defecto).
- Validacion: `terraform fmt` PASS, `init -backend=false` OK, `terraform validate` Success, 65 pruebas estaticas Terraform + 11 systemd.
- Bloqueo de despliegue: RFC-004 (secretos en reemplazo automatico). NO_RUN_AWS: no hay instancias.

### Correccion de cierre — 2026-11-06

- Bootstrap completo: descarga binario **y unidad systemd** desde S3, verifica SHA-256 de ambos, instala el binario como `root:root 0755`, la unidad como `root:root 0644`, exige `/etc/nexum/backend.env`, y arranca comprobando `systemctl is-active` + liveness local. Cualquier fallo termina con `exit 1` (sin avisos que simulen exito).
- ASG: `min_healthy_percentage = 100` y `max_healthy_percentage = 200` (antes 50). El refresh usa una instancia temporal documentada en FinOps.
- `sg_backend_id`: validacion estricta `^sg-[0-9a-f]{8,17}$` (antes aceptaba cualquier cadena no vacia).
- `build-backend.ps1`: guarda y restaura `CGO_ENABLED/GOOS/GOARCH` incluso si el build falla.
- Defecto D-01 (no de I-020): las rutas `/api/v1/health/live` y `/ready` **no estan registradas** en el router Go; solo existe `health_handler.go`. Bloquea el health check de I-030 y el despliegue; se registra para A-090/H08-A.
- Pruebas: 76 Terraform (11 nuevas de bootstrap/script), 11 systemd, 167 FinOps, 18 permisos.
