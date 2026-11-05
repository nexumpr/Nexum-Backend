# I-090-cd-backend — CD, versionamiento, autorizaciones y rollback

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-110, I-020, I-030, I-060, I-080 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## 1. Estrategia de diseño documental de H05

Diseñar promoción de un artefacto Go verificado mediante release autorizado y evidencia HTTP real, sin crear aún pipeline ni publicar código.

## Autoridad y restricciones

- `constitution.md` (GOV-01..18, R-001..060) y el Gate H04 aprobado explícitamente el **2026-11-05** tienen precedencia; H05 diseña infraestructura, no la ejecuta.
- Arquitectura ratificada: Go (Clean Architecture), AWS, DynamoDB, **Redis real**, **2 instancias simultáneas**, balanceador HTTPS y CI/CD/DevSecOps. Ninguno es demostración opcional.
- El PDF del docente v3 exige Backend cloud con costo cero; el catálogo V6.0.2 enumera 75 rutas propuestas E001..E075 y utiliza stack histórico GCP/MongoDB **descartado**.
- No crear código Go, archivos Terraform `.tf`, pipelines ni endpoints H06 durante H05; `terraform apply/destroy`, `git push`, cambios AWS y facturación están prohibidos hasta fases y aprobaciones autorizadas.
- **Regla de evidencia:** documentos pueden alcanzar DRAFT completo sin acceso cloud; Gate I-GO requiere demostrar **viabilidad operacional y económica**, no asumirla. Campos pendientes no se cambian a PASS ni se solventan con créditos.

## 2. Plan secuencial y verificaciones

### P-090-01 — Matriz versiones

**Entrada:** H04 aprobado, requisitos `A-110, I-020, I-030, I-060, I-080`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Antes del primer release fijar semver proyecto, versión API `/api/v1`, secuencia backend (p.ej. v0.1.0) y release-id inmutable, vínculo SHA commit/SBOM/artefacto.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-090-02 — Aprobación

**Entrada:** H04 aprobado, requisitos `A-110, I-020, I-030, I-060, I-080`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Promotion solo desde artifacts CI verdes, GitHub Environment protegido, responsable autoriza por release y por función nueva; no `main`→deploy automático. Plan Terraform con revisión separada de código de app.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-090-03 — Despliegue seguro

**Entrada:** H04 aprobado, requisitos `A-110, I-020, I-030, I-060, I-080`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Rollout EC2 ASG rolling/blue-green según precio con `maxUnavailable` y mínimo2 healthy cuando está encendido; si faltan 2 instancias sanas, abortar. Verificar health y rollback a digest previo.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-090-04 — Pruebas HTTP

**Entrada:** H04 aprobado, requisitos `A-110, I-020, I-030, I-060, I-080`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Postdeploy GET health, HTTPS certificado, autorizar endpoints, registrar método/path/status/schema y persistencia real Dynamo para función nueva. Un código 200 sin operación persistente no basta.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-090-05 — Fallos y rollback

**Entrada:** H04 aprobado, requisitos `A-110, I-020, I-030, I-060, I-080`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Rollback app se puede ejecutar independientemente de rollback de esquema Dynamo: migraciones backwards compatible; TTL y GSI versionados; confirmar plan de reversión.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-090-06 — Cierre

**Entrada:** H04 aprobado, requisitos `A-110, I-020, I-030, I-060, I-080`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Registro de despliegue, artefacto/version, tiempo, autorizador, 2 targets sanos, costos, errores. No integrar ni aprobar Android.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

## 3. Orden de ejecución futura H08 (no ejecutar ahora)

1. Auditar plan AWS/credenciales/precios y límites SCP sin cambios.
2. Preparar revisión de cambios, artefactos IaC y análisis económico en PR autorizado.
3. Ejecutar validaciones (fmt/validate/plan/pruebas), someter al gate humano.
4. Aprovisionar o modificar únicamente después de autorización de release y protección de datos.
5. Ejecutar pruebas reales, registrar métricas, costos y rollback, y no declarar DONE antes de las evidencias.

## 4. Dependencias cruzadas

I-020, I-030, I-060, I-080, I-100, I-140, I-150

## 5. Riesgos y tratamiento

1. `I-090-cd-backend-R01`: Rollout reduce nodos por debajo 2. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.
2. `I-090-cd-backend-R02`: Fallos HTTP durante cutover. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.
3. `I-090-cd-backend-R03`: Falta permiso OIDC/deploy. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.
4. `I-090-cd-backend-R04`: Presupuesto impide doble capacidad transitoria. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.

## 6. Puerta de entrada a H06

Gate H05/I-GO NO se logra con instalación de Markdown: requiere pruebas de viabilidad económica/cuenta, trazabilidad física Dynamo/Redis, controles IAM y aceptación expresa del responsable. Los contratos detallados E001..E075 se formalizan exclusivamente en H06.
