# I-100-devsecops — DevSecOps integral y amenaza por superficie

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-050, A-100, I-060, I-080 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## 1. Objetivo y definición de terminado

Definir pruebas de seguridad automatizadas y manuales autorizadas para CI, IaC, HTTP, datos financieros y tokenización con tolerancia cero a secretos expuestos.

## Autoridad y restricciones

- `constitution.md` (GOV-01..18, R-001..060) y el Gate H04 aprobado explícitamente el **2026-11-05** tienen precedencia; H05 diseña infraestructura, no la ejecuta.
- Arquitectura ratificada: Go (Clean Architecture), AWS, DynamoDB, **Redis real**, **2 instancias simultáneas**, balanceador HTTPS y CI/CD/DevSecOps. Ninguno es demostración opcional.
- El PDF del docente v3 exige Backend cloud con costo cero; el catálogo V6.0.2 enumera 75 rutas propuestas E001..E075 y utiliza stack histórico GCP/MongoDB **descartado**.
- No crear código Go, archivos Terraform `.tf`, pipelines ni endpoints H06 durante H05; `terraform apply/destroy`, `git push`, cambios AWS y facturación están prohibidos hasta fases y aprobaciones autorizadas.
- **Regla de evidencia:** documentos pueden alcanzar DRAFT completo sin acceso cloud; Gate I-GO requiere demostrar **viabilidad operacional y económica**, no asumirla. Campos pendientes no se cambian a PASS ni se solventan con créditos.

## 2. Contrato de infraestructura propuesto

### INF-100-01 — Threat model

Superficies: Android→HTTPS, ALB→Go, Go→Dynamo/Redis, CI→AWS OIDC, logs/backup, recuperación/cambio contraseña, delete-account. STRIDE y datos regulados; priorizar IDOR, token replay, money manipulation y SSRF.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-100-02 — SAST/SCA

gosec, govulncheck, staticcheck y SCA/GitHub Dependency Review/Dependabot según disponibilidad; bloquear CVE critical/high explotable con RFC precisa para excepciones.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-100-03 — Secrets/SBOM

Gitleaks o equivalente en PR + historial permitido; generar SBOM CycloneDX o SPDX por build; firmado SHA y control supply chain de actions pinned.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-100-04 — DAST

ZAP/API scan únicamente contra entorno aislado autorizado con datos sintéticos y alcance/ventana explícitos, rate limits; jamás escanear terceros ni producción sin autorización.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-100-05 — IaC

Checkov/tfsec o equivalente, terraform fmt/validate, SG públicos, buckets cifrados, IAM wildcards, leaks en Terraform state y dependencias de roles.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-100-06 — Pruebas adversariales

JWT inválido/expirado/revocado, cambio de usuario entre payload/ID, participación en metas ajenas, carrera de idempotencia, rate limit entre nodos, CORS/headers, DoS validaciones/bodysize.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

## 3. Dependencias y secuencia

I-060, I-080, I-090, I-140, I-150

## 4. Criterios de aceptación de diseño (Gate H05)

- Modelo amenazas cubre 5 superficies y 75 contratos.
- CI marca y bloquea vulnerabilidades según severidad/alcance.
- Pruebas authz Cross-user con 401/403/404 apropiados.
- DAST autorizado con datos falsos y evidencias.
- No ejecutar herramientas ofensivas en H05.

## 5. Riesgos que impiden una aprobación sin evidencia

- **Riesgo:** Riesgo de pruebas que ataquen producción. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.
- **Riesgo:** Falsos negativos de escáner. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.
- **Riesgo:** Secrets en artefactos/logs. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.

## 6. Entregables y referencias

Los siguientes nombres identifican evidencias/artefactos **futuros** de I-GO y de implementación; no son archivos creados en H05: `MODELO_AMENAZAS.md`, `MATRIZ_ESCANEOS.md`, `CRITERIOS_VULNERABILIDADES.md`.

## 7. Criterio de bloqueo y aprobación

**BLOCKED** si falta una obligación técnica, contradice alguna spec A aprobada, no puede comprobarse la viabilidad Free/SCP o no hay aceptación humana del riesgo. **DRAFT** significa que el diseño está documentado; nunca equivale a infraestructura aplicada.
