# I-080-ci-backend — CI Backend GitHub Actions y quality gates

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-100, A-110, I-060, I-070 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## 1. Estrategia de diseño documental de H05

Definir CI de Go y documentación/infraestructura con pruebas, análisis y calidad medibles, sin ejecutar workflows ni crear archivos de GitHub durante H05.

## Autoridad y restricciones

- `constitution.md` (GOV-01..18, R-001..060) y el Gate H04 aprobado explícitamente el **2026-11-05** tienen precedencia; H05 diseña infraestructura, no la ejecuta.
- Arquitectura ratificada: Go (Clean Architecture), AWS, DynamoDB, **Redis real**, **2 instancias simultáneas**, balanceador HTTPS y CI/CD/DevSecOps. Ninguno es demostración opcional.
- El PDF del docente v3 exige Backend cloud con costo cero; el catálogo V6.0.2 enumera 75 rutas propuestas E001..E075 y utiliza stack histórico GCP/MongoDB **descartado**.
- No crear código Go, archivos Terraform `.tf`, pipelines ni endpoints H06 durante H05; `terraform apply/destroy`, `git push`, cambios AWS y facturación están prohibidos hasta fases y aprobaciones autorizadas.
- **Regla de evidencia:** documentos pueden alcanzar DRAFT completo sin acceso cloud; Gate I-GO requiere demostrar **viabilidad operacional y económica**, no asumirla. Campos pendientes no se cambian a PASS ni se solventan con créditos.

## 2. Plan secuencial y verificaciones

### P-080-01 — Eventos

**Entrada:** H04 aprobado, requisitos `A-100, A-110, I-060, I-070`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Pull_request a release/main: gofmt, go vet, govulncheck, go test -race, coverage (umbral propuesto 80% dominio), staticcheck, SAST, lint, análisis dependencias, API schema validation, secretos e IaC scan. Tags y versiones pinneadas por SHA.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-080-02 — PR

**Entrada:** H04 aprobado, requisitos `A-100, A-110, I-060, I-070`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Branch protection con checks requeridos, revisión obligatoria, autor y rastro de cambio; artifacts por SHA/source revision; no subir SDD a GitHub antes de H07 conforme constitución vigente.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-080-03 — Seguridad CI

**Entrada:** H04 aprobado, requisitos `A-100, A-110, I-060, I-070`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** `permissions: contents: read` por defecto; sin privilegios cloud en jobs untrusted. PR de forks no recibe secretos; OIDC sólo para release autorizado.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-080-04 — Pruebas

**Entrada:** H04 aprobado, requisitos `A-100, A-110, I-060, I-070`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Unit dominio con money/timezone; integración Dynamo (local o aislada con permisos), Redis aislado, API HTTP de extremo a extremo y contratos OpenAPI. No Docker local obligatorio; servicios cloud/test controlados según entorno y presupuesto.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-080-05 — Fallos

**Entrada:** H04 aprobado, requisitos `A-100, A-110, I-060, I-070`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Cualquier hallazgo alto/crítico o test roto bloquea merge; excepciones únicamente RFC firmada con vigencia y riesgo, nunca green forzado.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-080-06 — Versiones

**Entrada:** H04 aprobado, requisitos `A-100, A-110, I-060, I-070`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Go toolchain fijado por `go.mod`/setup-go exact, `-trimpath`, `-buildvcs`, reproducibilidad, SBOM e integridad de artefactos.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

## 3. Orden de ejecución futura H08 (no ejecutar ahora)

1. Auditar plan AWS/credenciales/precios y límites SCP sin cambios.
2. Preparar revisión de cambios, artefactos IaC y análisis económico en PR autorizado.
3. Ejecutar validaciones (fmt/validate/plan/pruebas), someter al gate humano.
4. Aprovisionar o modificar únicamente después de autorización de release y protección de datos.
5. Ejecutar pruebas reales, registrar métricas, costos y rollback, y no declarar DONE antes de las evidencias.

## 4. Dependencias cruzadas

I-060, I-070, I-090, I-100

## 5. Riesgos y tratamiento

1. `I-080-ci-backend-R01`: CI no puede usar toolchains por plan limitado. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.
2. `I-080-ci-backend-R02`: Pipeline costo/tiempo alto. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.
3. `I-080-ci-backend-R03`: Escaneo IoC inseguro con falsos PASS. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.

## 6. Puerta de entrada a H06

Gate H05/I-GO NO se logra con instalación de Markdown: requiere pruebas de viabilidad económica/cuenta, trazabilidad física Dynamo/Redis, controles IAM y aceptación expresa del responsable. Los contratos detallados E001..E075 se formalizan exclusivamente en H06.
