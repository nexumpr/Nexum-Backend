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


### Implementacion H08 (2026-11-08)

- T-100-01 Threat model: IMPLEMENTADO_LOCAL en MODELO_AMENAZAS.md con 6 superficies, STRIDE, IDOR/replay/money/SSRF y riesgos residuales.
- T-100-02 SAST/SCA: IMPLEMENTADO_LOCAL. gosec, govulncheck y staticcheck en CI; criterios HIGH/CRITICAL y RFC en CRITERIOS_VULNERABILIDADES.md. Dependency Review PENDIENTE_DE_DISPONIBILIDAD.
- T-100-03 Secrets/SBOM: IMPLEMENTADO_LOCAL. gitleaks con baseline acotado, SBOM SPDX por SHA y acciones fijadas.
- T-100-04 DAST: NOT_RUN. Procedimiento y limites documentados; requiere entorno aislado, datos sinteticos y autorizacion.
- T-100-05 IaC: IMPLEMENTADO_LOCAL. Trivy HIGH/CRITICAL + herramientas/verificar_iac_devsecops.py (SG publico, IAM wildcard, S3 sin cifrado, force_destroy, ACL publica, state rastreado) en el job iac.
- T-100-06 Pruebas adversariales: IMPLEMENTADO_LOCAL. tests/security/adversarial_test.go con JWT, sesion/replay, idempotencia concurrente, CORS, cabeceras, body y rate limit; incluido en go test y -race.
- Pruebas: 13 de contrato DevSecOps y 7 del escaner IaC. AWS_NOT_RUN.

- Ampliacion de revision (2026-11-08): MODELO_AMENAZAS con matriz verificable T-01..T-22 (superficie, amenaza, spec/ruta, control, prueba P/N, evidencia, severidad, responsable, estado) y relacion E001-E075 (75 documentados, 0 endpoints implementados; casos futuros NOT_IMPLEMENTED/NOT_RUN).
- SAST: gosec limitado a codigo de produccion y prueba negativa con fixture math/rand (G404) + verificar_fixture_gosec.py en CI. Prueba local del fixture NOT_RUN por bloqueo de Application Control; CI la ejecuta.
- DAST: harness preparar_dast.py valida objetivo aislado, autorizacion por host, datos sinteticos, ventana <=4h, RPS <=5 y alcance; sin APRUEBO_GATE_DAST devuelve BLOCKED_AUTHORIZATION y no genera plan.
- Supply chain: se documenta checksum SHA-256 vs firma criptografica (no hay firma); sustitucion prevenida por release inmutable + If-None-Match + digest en LT.
- Pruebas: 15 de contrato DevSecOps, 7 IaC, 10 DAST/gosec.

- Correccion de escaner IaC (2026-11-08): cifrado evaluado por bucket (no indicador global), SG con cidr_blocks/ipv6_cidr_blocks/cidr_ipv4/cidr_ipv6, IAM wildcard escalar y en listas, y GIT_CHECK_FAILED si git no puede verificar. Expresiones dinamicas -> S3_CIFRADO_NO_VERIFICABLE (no PASS). 12 pruebas.
- Rate limiting (2026-11-08): se caracteriza el defecto A-030 (middleware local por instancia y XFF confiado) y se prueba coordinacion simulada de A-080 con cache en memoria. El limitador distribuido no esta cableado; Redis real y 2 nodos NOT_RUN. D-100-01/D-100-02 registrados; no se declara RATE_LIMIT_DISTRIBUTED_VERIFIED.
- DAST (2026-11-08): el harness exige evidencia de Gate dentro de sdd/_control/H08 con frase, host y caducidad; --permitir-remoto eliminado. Sin evidencia -> BLOCKED_AUTHORIZATION. 13 pruebas.
- Pruebas: 18 de contrato DevSecOps, 12 IaC y 13 DAST/gosec.

- Correccion definitiva (2026-11-08): analisis IaC por bloques con emparejamiento de llaves; cifrado solo si el bloque SSE referencia el bucket y declara sse_algorithm valido; asociaciones ajenas y expresiones dinamicas no protegen. Ingress/egress separados por tipo y sub-bloque; comentarios ignorados. Fixtures de secure/exposed y ingress privado/egress publico. 15 pruebas.
- DAST: la evidencia local es candidata, no autorizacion; sin verificacion de procedencia por Gates -> BLOCKED_AUTHORIZATION. --fecha solo en modo prueba con NEXUM_TEST_MODE=1; plan tecnico PLAN_TECNICO_PREPARADO_NO_AUTORIZADO. 15 pruebas.
- Pruebas: 18 de contrato DevSecOps, 15 IaC y 15 DAST/gosec.
