# I-090-cd-backend — CD, versionamiento, autorizaciones y rollback

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-110, I-020, I-030, I-060, I-080 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## 1. Objetivo y definición de terminado

Diseñar promoción de un artefacto Go verificado mediante release autorizado y evidencia HTTP real, sin crear aún pipeline ni publicar código.

## Autoridad y restricciones

- `constitution.md` (GOV-01..18, R-001..060) y el Gate H04 aprobado explícitamente el **2026-11-05** tienen precedencia; H05 diseña infraestructura, no la ejecuta.
- Arquitectura ratificada: Go (Clean Architecture), AWS, DynamoDB, **Redis real**, **2 instancias simultáneas**, balanceador HTTPS y CI/CD/DevSecOps. Ninguno es demostración opcional.
- El PDF del docente v3 exige Backend cloud con costo cero; el catálogo V6.0.2 enumera 75 rutas propuestas E001..E075 y utiliza stack histórico GCP/MongoDB **descartado**.
- No crear código Go, archivos Terraform `.tf`, pipelines ni endpoints H06 durante H05; `terraform apply/destroy`, `git push`, cambios AWS y facturación están prohibidos hasta fases y aprobaciones autorizadas.
- **Regla de evidencia:** documentos pueden alcanzar DRAFT completo sin acceso cloud; Gate I-GO requiere demostrar **viabilidad operacional y económica**, no asumirla. Campos pendientes no se cambian a PASS ni se solventan con créditos.

## 2. Contrato de infraestructura propuesto

### INF-090-01 — Matriz versiones

Antes del primer release fijar semver proyecto, versión API `/api/v1`, secuencia backend (p.ej. v0.1.0) y release-id inmutable, vínculo SHA commit/SBOM/artefacto.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-090-02 — Aprobación

Promotion solo desde artifacts CI verdes, GitHub Environment protegido, responsable autoriza por release y por función nueva; no `main`→deploy automático. Plan Terraform con revisión separada de código de app.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-090-03 — Despliegue seguro

Rollout EC2 ASG rolling/blue-green según precio con `maxUnavailable` y mínimo2 healthy cuando está encendido; si faltan 2 instancias sanas, abortar. Verificar health y rollback a digest previo.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-090-04 — Pruebas HTTP

Postdeploy GET health, HTTPS certificado, autorizar endpoints, registrar método/path/status/schema y persistencia real Dynamo para función nueva. Un código 200 sin operación persistente no basta.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-090-05 — Fallos y rollback

Rollback app se puede ejecutar independientemente de rollback de esquema Dynamo: migraciones backwards compatible; TTL y GSI versionados; confirmar plan de reversión.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-090-06 — Cierre

Registro de despliegue, artefacto/version, tiempo, autorizador, 2 targets sanos, costos, errores. No integrar ni aprobar Android.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

## 3. Dependencias y secuencia

I-020, I-030, I-060, I-080, I-100, I-140, I-150

## 4. Criterios de aceptación de diseño (Gate H05)

- Release enlaza commit + SHA de binario + OpenAPI + pruebas.
- GitHub environment humano controla deploy.
- Siempre dos nodos reales durante release activo, con health.
- Prueba HTTP y datos persistentes por cada función nueva.
- Rollback ensayado y documentado antes de producción.

## 5. Riesgos que impiden una aprobación sin evidencia

- **Riesgo:** Rollout reduce nodos por debajo 2. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.
- **Riesgo:** Fallos HTTP durante cutover. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.
- **Riesgo:** Falta permiso OIDC/deploy. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.
- **Riesgo:** Presupuesto impide doble capacidad transitoria. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.

## 6. Entregables y referencias

Los siguientes nombres identifican evidencias/artefactos **futuros** de I-GO y de implementación; no son archivos creados en H05: `POLITICA_RELEASES.md`, `PLAN_ROLLBACK.md`, `EVIDENCIA_DEPLOY.md`.

## 7. Criterio de bloqueo y aprobación

**BLOCKED** si falta una obligación técnica, contradice alguna spec A aprobada, no puede comprobarse la viabilidad Free/SCP o no hay aceptación humana del riesgo. **DRAFT** significa que el diseño está documentado; nunca equivale a infraestructura aplicada.
