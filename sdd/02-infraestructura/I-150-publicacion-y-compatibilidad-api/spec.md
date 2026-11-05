# I-150-publicacion-y-compatibilidad-api — Publicación y compatibilidad de contratos API

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-040, A-110, E001..E075, I-030, I-090 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## 1. Objetivo y definición de terminado

Publicar una API HTTPS accesible con contratos OpenAPI verificables independientemente, sin incluir ni depender de aprobación/pruebas del equipo Mobile.

## Autoridad y restricciones

- `constitution.md` (GOV-01..18, R-001..060) y el Gate H04 aprobado explícitamente el **2026-11-05** tienen precedencia; H05 diseña infraestructura, no la ejecuta.
- Arquitectura ratificada: Go (Clean Architecture), AWS, DynamoDB, **Redis real**, **2 instancias simultáneas**, balanceador HTTPS y CI/CD/DevSecOps. Ninguno es demostración opcional.
- El PDF del docente v3 exige Backend cloud con costo cero; el catálogo V6.0.2 enumera 75 rutas propuestas E001..E075 y utiliza stack histórico GCP/MongoDB **descartado**.
- No crear código Go, archivos Terraform `.tf`, pipelines ni endpoints H06 durante H05; `terraform apply/destroy`, `git push`, cambios AWS y facturación están prohibidos hasta fases y aprobaciones autorizadas.
- **Regla de evidencia:** documentos pueden alcanzar DRAFT completo sin acceso cloud; Gate I-GO requiere demostrar **viabilidad operacional y económica**, no asumirla. Campos pendientes no se cambian a PASS ni se solventan con créditos.

## 2. Contrato de infraestructura propuesto

### INF-150-01 — Contrato

OpenAPI 3.1 estable como fuente de verdad; versionar path `/api/v1`, schemas, errores con traceId, scopes JWT, page cursor, idempotency, moneda decimal/string y enums; todo EP formalizado solo en H06.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-150-02 — Descubrimiento

Definir URL de backend pública HTTPS y ruta de especificación OpenAPI o artefacto publicado con checksum, con documentación de ejemplos sin credenciales. Política CORS permite orígenes aprobados sin abrir * con credenciales.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-150-03 — Compatibilidad

Matriz E001-E075: método, ruta, status, esquema, auth, trazabilidad UI, prueba negativa, persistencia; clasificación M/S/E/O no exime requisitos obligatorios adicionales del profesor (push, ML/IA si el backend interviene).

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-150-04 — Evidencia HTTP

Desde cliente independiente: curl/HTTP GET live/ready, registro/login/logout, cuatro endpoints adicionales de la rúbrica, 401/403/404, HTTPS CA, persistencia Dynamo y paso por dos nodos. Tests contrato ejecutados por Backend.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-150-05 — Cambio

Compatibilidad semver y deprecation de `/api/v1`, revisión de breaking changes en CI; ningún cambio sin actualizar schemas/tareas/historias.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-150-06 — Alcance

La aplicación Android, Play Store, pantallas, pruebas de integración Mobile y landing se entregan por otros equipos. No bloquear una fase de Backend hasta que móvil confirme; no atribuirles gates de nuestro API.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-150-07 — CDN/moderación

Si algún EP recibe texto/imagen/video, diseñar moderación pre-storage mediante servicio cloud permitido y CDN de AWS según requisito, con costos y manejo de contenidos inapropiados; no crear EP artificiosos para archivos no presentes.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

## 3. Dependencias y secuencia

I-030, I-060, I-080, I-090, H06

## 4. Criterios de aceptación de diseño (Gate H05)

- API accesible HTTPS y OpenAPI 3.1 con hash.
- Matriz endpoints E001..E075 categorizada y rastreable.
- Pruebas HTTP reales del Backend sin confirmación Mobile.
- 4 endpoints adicionales más registro/login/logout evaluables según PDF.
- Política CI de breaking changes y versionado de release.

## 5. Riesgos que impiden una aprobación sin evidencia

- **Riesgo:** Rutas opcionales asumidas obligatorias sin evidencia. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.
- **Riesgo:** Equipo Mobile convertido indebidamente en gate. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.
- **Riesgo:** Función de upload futura sin moderación/CDN. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.

## 6. Entregables y referencias

Los siguientes nombres identifican evidencias/artefactos **futuros** de I-GO y de implementación; no son archivos creados en H05: `MATRIZ_COMPATIBILIDAD_75.md`, `PLAN_PUB_OPENAPI.md`, `PLAN_EVIDENCIA_HTTP.md`.

## 7. Criterio de bloqueo y aprobación

**BLOCKED** si falta una obligación técnica, contradice alguna spec A aprobada, no puede comprobarse la viabilidad Free/SCP o no hay aceptación humana del riesgo. **DRAFT** significa que el diseño está documentado; nunca equivale a infraestructura aplicada.
