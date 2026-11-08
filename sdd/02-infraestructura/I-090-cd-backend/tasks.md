# I-090-cd-backend — CD, versionamiento, autorizaciones y rollback

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-110, I-020, I-030, I-060, I-080 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## Gestión de tareas H05 (solo especificación y aceptación de diseño)

Estado inicial de todos los trabajos: `TODO`, salvo evidencia local de integridad H04 documentada por el responsable. No marcar como completado por generar archivos.

### T-090-01 — Cerrar matriz versiones

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-110, I-020, I-030, I-060, I-080.
- **Trabajo:** Antes del primer release fijar semver proyecto, versión API `/api/v1`, secuencia backend (p.ej. v0.1.0) y release-id inmutable, vínculo SHA commit/SBOM/artefacto.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-090-02 — Cerrar aprobación

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-110, I-020, I-030, I-060, I-080.
- **Trabajo:** Promotion solo desde artifacts CI verdes, GitHub Environment protegido, responsable autoriza por release y por función nueva; no `main`→deploy automático. Plan Terraform con revisión separada de código de app.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-090-03 — Cerrar despliegue seguro

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-110, I-020, I-030, I-060, I-080.
- **Trabajo:** Rollout EC2 ASG rolling/blue-green según precio con `maxUnavailable` y mínimo2 healthy cuando está encendido; si faltan 2 instancias sanas, abortar. Verificar health y rollback a digest previo.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-090-04 — Cerrar pruebas http

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-110, I-020, I-030, I-060, I-080.
- **Trabajo:** Postdeploy GET health, HTTPS certificado, autorizar endpoints, registrar método/path/status/schema y persistencia real Dynamo para función nueva. Un código 200 sin operación persistente no basta.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-090-05 — Cerrar fallos y rollback

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-110, I-020, I-030, I-060, I-080.
- **Trabajo:** Rollback app se puede ejecutar independientemente de rollback de esquema Dynamo: migraciones backwards compatible; TTL y GSI versionados; confirmar plan de reversión.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-090-06 — Cerrar cierre

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-110, I-020, I-030, I-060, I-080.
- **Trabajo:** Registro de despliegue, artefacto/version, tiempo, autorizador, 2 targets sanos, costos, errores. No integrar ni aprobar Android.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-090-90 — Revisión de riesgos y dependencias

- **Estado:** TODO.
- **Trabajo:** resolver o escalar: Rollout reduce nodos por debajo 2, Fallos HTTP durante cutover, Falta permiso OIDC/deploy, Presupuesto impide doble capacidad transitoria.
- **Criterio DoD:** cada riesgo clasificado RESOLVED/BLOCKED/ACCEPTED con firma/fecha; no usar ACCEPTED para incumplir requisito docente sin aclaración.

### T-090-99 — Ratificación de la especificación

- **Estado:** REVIEW pendiente.
- **Entrada:** `spec.md`, `plan.md`, `tasks.md`, `checklist.md` actualizados.
- **Salida:** decisión explícita del responsable y vínculo a evidencia en informe I-GO.
- **Bloqueo:** no crear Terraform/endpoints ni ejecutar AWS.


### Implementacion H08 (2026-11-08)

- T-090-01 Matriz de versiones: IMPLEMENTADO_LOCAL en POLITICA_RELEASES.md (SemVer vX.Y.Z, /api/v1, release vX.Y.Z+sha12, digest y SBOM del mismo run).
- T-090-02 Aprobacion: IMPLEMENTADO_LOCAL. cd-backend.yml solo workflow_dispatch en main, entorno aws-deploy, CD_HABILITADO=true, frase AUTORIZO_DESPLIEGUE_<sha> y confirmar=SI; sin main->deploy.
- T-090-03 Despliegue seguro: IMPLEMENTADO_LOCAL. Instance refresh MinHealthyPercentage=100, aborta sin 2 InService y 2 healthy antes/despues, rollback a LT previa.
- T-090-04 Pruebas HTTP: IMPLEMENTADO_LOCAL. GET https /api/v1/health/live con curl --fail --proto https y exigencia de 200.
- T-090-05 Fallos y rollback: DOCUMENTADO en PLAN_ROLLBACK.md; rollback de app independiente del esquema, migraciones backwards compatible, no borrar state ni datos.
- T-090-06 Cierre: DOCUMENTADO en EVIDENCIA_DEPLOY.md con estado NOT_RUN y campos de release, autorizador, 2 targets, coste y errores.
- Cambio habilitador I-080: go-backend.yml publica binario-<sha> linux/arm64 con SHA-256; no altera checks ni umbrales.
- Pruebas: 21 de contrato CD y 10 de autorizacion; AWS_NOT_RUN.

- Verificacion de run de CI (2026-11-08): preflight consulta la API del run y exige status completed, conclusion success, head_sha y repositorio/rama autorizados (herramientas/verificar_run_ci.py).
- Entorno protegido (2026-11-08): ENVIRONMENT_PROTECTION_CONFIRMED=aws-deploy y lectura de la API del entorno; sin required_reviewers o sin permiso de lectura, falla cerrado antes de OIDC (herramientas/verificar_entorno_protegido.py).
- Capacidad transitoria: max_size 2->3 durante el rollout y restauracion a 2 en exito y rollback; coste ~0.0084 USD/h no medido; si no cabe, BLOCKED.
- Readiness: /api/v1/health/ready solo obligatoria con READINESS_REQUIRED=true; por defecto 503 documentado, no declarado.
- Simulador determinista herramientas/simular_despliegue.py (ABORTAR/ROLLBACK/DESPLEGAR, max_final=2) con 9 pruebas.
- Pruebas: 29 de contrato CD, 11 de autorizacion, 10 de run/entorno, 9 de simulador.
