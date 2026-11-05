# I-100-devsecops — DevSecOps integral y amenaza por superficie

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-050, A-100, I-060, I-080 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## 1. Estrategia de diseño documental de H05

Definir pruebas de seguridad automatizadas y manuales autorizadas para CI, IaC, HTTP, datos financieros y tokenización con tolerancia cero a secretos expuestos.

## Autoridad y restricciones

- `constitution.md` (GOV-01..18, R-001..060) y el Gate H04 aprobado explícitamente el **2026-11-05** tienen precedencia; H05 diseña infraestructura, no la ejecuta.
- Arquitectura ratificada: Go (Clean Architecture), AWS, DynamoDB, **Redis real**, **2 instancias simultáneas**, balanceador HTTPS y CI/CD/DevSecOps. Ninguno es demostración opcional.
- El PDF del docente v3 exige Backend cloud con costo cero; el catálogo V6.0.2 enumera 75 rutas propuestas E001..E075 y utiliza stack histórico GCP/MongoDB **descartado**.
- No crear código Go, archivos Terraform `.tf`, pipelines ni endpoints H06 durante H05; `terraform apply/destroy`, `git push`, cambios AWS y facturación están prohibidos hasta fases y aprobaciones autorizadas.
- **Regla de evidencia:** documentos pueden alcanzar DRAFT completo sin acceso cloud; Gate I-GO requiere demostrar **viabilidad operacional y económica**, no asumirla. Campos pendientes no se cambian a PASS ni se solventan con créditos.

## 2. Plan secuencial y verificaciones

### P-100-01 — Threat model

**Entrada:** H04 aprobado, requisitos `A-050, A-100, I-060, I-080`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Superficies: Android→HTTPS, ALB→Go, Go→Dynamo/Redis, CI→AWS OIDC, logs/backup, recuperación/cambio contraseña, delete-account. STRIDE y datos regulados; priorizar IDOR, token replay, money manipulation y SSRF.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-100-02 — SAST/SCA

**Entrada:** H04 aprobado, requisitos `A-050, A-100, I-060, I-080`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** gosec, govulncheck, staticcheck y SCA/GitHub Dependency Review/Dependabot según disponibilidad; bloquear CVE critical/high explotable con RFC precisa para excepciones.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-100-03 — Secrets/SBOM

**Entrada:** H04 aprobado, requisitos `A-050, A-100, I-060, I-080`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Gitleaks o equivalente en PR + historial permitido; generar SBOM CycloneDX o SPDX por build; firmado SHA y control supply chain de actions pinned.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-100-04 — DAST

**Entrada:** H04 aprobado, requisitos `A-050, A-100, I-060, I-080`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** ZAP/API scan únicamente contra entorno aislado autorizado con datos sintéticos y alcance/ventana explícitos, rate limits; jamás escanear terceros ni producción sin autorización.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-100-05 — IaC

**Entrada:** H04 aprobado, requisitos `A-050, A-100, I-060, I-080`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Checkov/tfsec o equivalente, terraform fmt/validate, SG públicos, buckets cifrados, IAM wildcards, leaks en Terraform state y dependencias de roles.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-100-06 — Pruebas adversariales

**Entrada:** H04 aprobado, requisitos `A-050, A-100, I-060, I-080`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** JWT inválido/expirado/revocado, cambio de usuario entre payload/ID, participación en metas ajenas, carrera de idempotencia, rate limit entre nodos, CORS/headers, DoS validaciones/bodysize.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

## 3. Orden de ejecución futura H08 (no ejecutar ahora)

1. Auditar plan AWS/credenciales/precios y límites SCP sin cambios.
2. Preparar revisión de cambios, artefactos IaC y análisis económico en PR autorizado.
3. Ejecutar validaciones (fmt/validate/plan/pruebas), someter al gate humano.
4. Aprovisionar o modificar únicamente después de autorización de release y protección de datos.
5. Ejecutar pruebas reales, registrar métricas, costos y rollback, y no declarar DONE antes de las evidencias.

## 4. Dependencias cruzadas

I-060, I-080, I-090, I-140, I-150

## 5. Riesgos y tratamiento

1. `I-100-devsecops-R01`: Riesgo de pruebas que ataquen producción. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.
2. `I-100-devsecops-R02`: Falsos negativos de escáner. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.
3. `I-100-devsecops-R03`: Secrets en artefactos/logs. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.

## 6. Puerta de entrada a H06

Gate H05/I-GO NO se logra con instalación de Markdown: requiere pruebas de viabilidad económica/cuenta, trazabilidad física Dynamo/Redis, controles IAM y aceptación expresa del responsable. Los contratos detallados E001..E075 se formalizan exclusivamente en H06.
