# I-080-ci-backend — CI Backend GitHub Actions y quality gates

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-100, A-110, I-060, I-070 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## 1. Objetivo y definición de terminado

Definir CI de Go y documentación/infraestructura con pruebas, análisis y calidad medibles, sin ejecutar workflows ni crear archivos de GitHub durante H05.

## Autoridad y restricciones

- `constitution.md` (GOV-01..18, R-001..060) y el Gate H04 aprobado explícitamente el **2026-11-05** tienen precedencia; H05 diseña infraestructura, no la ejecuta.
- Arquitectura ratificada: Go (Clean Architecture), AWS, DynamoDB, **Redis real**, **2 instancias simultáneas**, balanceador HTTPS y CI/CD/DevSecOps. Ninguno es demostración opcional.
- El PDF del docente v3 exige Backend cloud con costo cero; el catálogo V6.0.2 enumera 75 rutas propuestas E001..E075 y utiliza stack histórico GCP/MongoDB **descartado**.
- No crear código Go, archivos Terraform `.tf`, pipelines ni endpoints H06 durante H05; `terraform apply/destroy`, `git push`, cambios AWS y facturación están prohibidos hasta fases y aprobaciones autorizadas.
- **Regla de evidencia:** documentos pueden alcanzar DRAFT completo sin acceso cloud; Gate I-GO requiere demostrar **viabilidad operacional y económica**, no asumirla. Campos pendientes no se cambian a PASS ni se solventan con créditos.

## 2. Contrato de infraestructura propuesto

### INF-080-01 — Eventos

Pull_request a release/main: gofmt, go vet, govulncheck, go test -race, coverage (umbral propuesto 80% dominio), staticcheck, SAST, lint, análisis dependencias, API schema validation, secretos e IaC scan. Tags y versiones pinneadas por SHA.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-080-02 — PR

Branch protection con checks requeridos, revisión obligatoria, autor y rastro de cambio; artifacts por SHA/source revision; no subir SDD a GitHub antes de H07 conforme constitución vigente.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-080-03 — Seguridad CI

`permissions: contents: read` por defecto; sin privilegios cloud en jobs untrusted. PR de forks no recibe secretos; OIDC sólo para release autorizado.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-080-04 — Pruebas

Unit dominio con money/timezone; integración Dynamo (local o aislada con permisos), Redis aislado, API HTTP de extremo a extremo y contratos OpenAPI. No Docker local obligatorio; servicios cloud/test controlados según entorno y presupuesto.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-080-05 — Fallos

Cualquier hallazgo alto/crítico o test roto bloquea merge; excepciones únicamente RFC firmada con vigencia y riesgo, nunca green forzado.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-080-06 — Versiones

Go toolchain fijado por `go.mod`/setup-go exact, `-trimpath`, `-buildvcs`, reproducibilidad, SBOM e integridad de artefactos.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

## 3. Dependencias y secuencia

I-060, I-070, I-090, I-100

## 4. Criterios de aceptación de diseño (Gate H05)

- Pipeline PR bloquea fallos y es reproducible.
- Consolidación logs/artifacts sin credenciales.
- Ejecuta pruebas Go unit/integration/security planificadas.
- Secreto encontrado bloquea job.
- Aún sin workflows ejecutados: estado DRAFT, no confundir diseño con CI verde.

## 5. Riesgos que impiden una aprobación sin evidencia

- **Riesgo:** CI no puede usar toolchains por plan limitado. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.
- **Riesgo:** Pipeline costo/tiempo alto. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.
- **Riesgo:** Escaneo IoC inseguro con falsos PASS. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.

## 6. Entregables y referencias

Los siguientes nombres identifican evidencias/artefactos **futuros** de I-GO y de implementación; no son archivos creados en H05: `MATRIZ_JOBS_CI.md`, `GATES_PR.md`, `REPORTE_EVIDENCIA_CI.md`.

## 7. Criterio de bloqueo y aprobación

**BLOCKED** si falta una obligación técnica, contradice alguna spec A aprobada, no puede comprobarse la viabilidad Free/SCP o no hay aceptación humana del riesgo. **DRAFT** significa que el diseño está documentado; nunca equivale a infraestructura aplicada.
