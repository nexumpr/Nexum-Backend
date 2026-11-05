# I-100-devsecops — DevSecOps integral y amenaza por superficie

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-050, A-100, I-060, I-080 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## Gestión de tareas H05 (solo especificación y aceptación de diseño)

Estado inicial de todos los trabajos: `TODO`, salvo evidencia local de integridad H04 documentada por el responsable. No marcar como completado por generar archivos.

### T-100-01 — Cerrar threat model

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-050, A-100, I-060, I-080.
- **Trabajo:** Superficies: Android→HTTPS, ALB→Go, Go→Dynamo/Redis, CI→AWS OIDC, logs/backup, recuperación/cambio contraseña, delete-account. STRIDE y datos regulados; priorizar IDOR, token replay, money manipulation y SSRF.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-100-02 — Cerrar sast/sca

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-050, A-100, I-060, I-080.
- **Trabajo:** gosec, govulncheck, staticcheck y SCA/GitHub Dependency Review/Dependabot según disponibilidad; bloquear CVE critical/high explotable con RFC precisa para excepciones.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-100-03 — Cerrar secrets/sbom

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-050, A-100, I-060, I-080.
- **Trabajo:** Gitleaks o equivalente en PR + historial permitido; generar SBOM CycloneDX o SPDX por build; firmado SHA y control supply chain de actions pinned.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-100-04 — Cerrar dast

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-050, A-100, I-060, I-080.
- **Trabajo:** ZAP/API scan únicamente contra entorno aislado autorizado con datos sintéticos y alcance/ventana explícitos, rate limits; jamás escanear terceros ni producción sin autorización.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-100-05 — Cerrar iac

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-050, A-100, I-060, I-080.
- **Trabajo:** Checkov/tfsec o equivalente, terraform fmt/validate, SG públicos, buckets cifrados, IAM wildcards, leaks en Terraform state y dependencias de roles.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-100-06 — Cerrar pruebas adversariales

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-050, A-100, I-060, I-080.
- **Trabajo:** JWT inválido/expirado/revocado, cambio de usuario entre payload/ID, participación en metas ajenas, carrera de idempotencia, rate limit entre nodos, CORS/headers, DoS validaciones/bodysize.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-100-90 — Revisión de riesgos y dependencias

- **Estado:** TODO.
- **Trabajo:** resolver o escalar: Riesgo de pruebas que ataquen producción, Falsos negativos de escáner, Secrets en artefactos/logs.
- **Criterio DoD:** cada riesgo clasificado RESOLVED/BLOCKED/ACCEPTED con firma/fecha; no usar ACCEPTED para incumplir requisito docente sin aclaración.

### T-100-99 — Ratificación de la especificación

- **Estado:** REVIEW pendiente.
- **Entrada:** `spec.md`, `plan.md`, `tasks.md`, `checklist.md` actualizados.
- **Salida:** decisión explícita del responsable y vínculo a evidencia en informe I-GO.
- **Bloqueo:** no crear Terraform/endpoints ni ejecutar AWS.
