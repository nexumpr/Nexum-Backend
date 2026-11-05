# I-080-ci-backend — CI Backend GitHub Actions y quality gates

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-100, A-110, I-060, I-070 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## Gestión de tareas H05 (solo especificación y aceptación de diseño)

Estado inicial de todos los trabajos: `TODO`, salvo evidencia local de integridad H04 documentada por el responsable. No marcar como completado por generar archivos.

### T-080-01 — Cerrar eventos

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-100, A-110, I-060, I-070.
- **Trabajo:** Pull_request a release/main: gofmt, go vet, govulncheck, go test -race, coverage (umbral propuesto 80% dominio), staticcheck, SAST, lint, análisis dependencias, API schema validation, secretos e IaC scan. Tags y versiones pinneadas por SHA.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-080-02 — Cerrar pr

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-100, A-110, I-060, I-070.
- **Trabajo:** Branch protection con checks requeridos, revisión obligatoria, autor y rastro de cambio; artifacts por SHA/source revision; no subir SDD a GitHub antes de H07 conforme constitución vigente.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-080-03 — Cerrar seguridad ci

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-100, A-110, I-060, I-070.
- **Trabajo:** `permissions: contents: read` por defecto; sin privilegios cloud en jobs untrusted. PR de forks no recibe secretos; OIDC sólo para release autorizado.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-080-04 — Cerrar pruebas

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-100, A-110, I-060, I-070.
- **Trabajo:** Unit dominio con money/timezone; integración Dynamo (local o aislada con permisos), Redis aislado, API HTTP de extremo a extremo y contratos OpenAPI. No Docker local obligatorio; servicios cloud/test controlados según entorno y presupuesto.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-080-05 — Cerrar fallos

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-100, A-110, I-060, I-070.
- **Trabajo:** Cualquier hallazgo alto/crítico o test roto bloquea merge; excepciones únicamente RFC firmada con vigencia y riesgo, nunca green forzado.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-080-06 — Cerrar versiones

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-100, A-110, I-060, I-070.
- **Trabajo:** Go toolchain fijado por `go.mod`/setup-go exact, `-trimpath`, `-buildvcs`, reproducibilidad, SBOM e integridad de artefactos.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-080-90 — Revisión de riesgos y dependencias

- **Estado:** TODO.
- **Trabajo:** resolver o escalar: CI no puede usar toolchains por plan limitado, Pipeline costo/tiempo alto, Escaneo IoC inseguro con falsos PASS.
- **Criterio DoD:** cada riesgo clasificado RESOLVED/BLOCKED/ACCEPTED con firma/fecha; no usar ACCEPTED para incumplir requisito docente sin aclaración.

### T-080-99 — Ratificación de la especificación

- **Estado:** REVIEW pendiente.
- **Entrada:** `spec.md`, `plan.md`, `tasks.md`, `checklist.md` actualizados.
- **Salida:** decisión explícita del responsable y vínculo a evidencia en informe I-GO.
- **Bloqueo:** no crear Terraform/endpoints ni ejecutar AWS.
