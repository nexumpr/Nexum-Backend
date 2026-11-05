# I-150-publicacion-y-compatibilidad-api — Publicación y compatibilidad de contratos API

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-040, A-110, E001..E075, I-030, I-090 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## 1. Estrategia de diseño documental de H05

Publicar una API HTTPS accesible con contratos OpenAPI verificables independientemente, sin incluir ni depender de aprobación/pruebas del equipo Mobile.

## Autoridad y restricciones

- `constitution.md` (GOV-01..18, R-001..060) y el Gate H04 aprobado explícitamente el **2026-11-05** tienen precedencia; H05 diseña infraestructura, no la ejecuta.
- Arquitectura ratificada: Go (Clean Architecture), AWS, DynamoDB, **Redis real**, **2 instancias simultáneas**, balanceador HTTPS y CI/CD/DevSecOps. Ninguno es demostración opcional.
- El PDF del docente v3 exige Backend cloud con costo cero; el catálogo V6.0.2 enumera 75 rutas propuestas E001..E075 y utiliza stack histórico GCP/MongoDB **descartado**.
- No crear código Go, archivos Terraform `.tf`, pipelines ni endpoints H06 durante H05; `terraform apply/destroy`, `git push`, cambios AWS y facturación están prohibidos hasta fases y aprobaciones autorizadas.
- **Regla de evidencia:** documentos pueden alcanzar DRAFT completo sin acceso cloud; Gate I-GO requiere demostrar **viabilidad operacional y económica**, no asumirla. Campos pendientes no se cambian a PASS ni se solventan con créditos.

## 2. Plan secuencial y verificaciones

### P-150-01 — Contrato

**Entrada:** H04 aprobado, requisitos `A-040, A-110, E001..E075, I-030, I-090`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** OpenAPI 3.1 estable como fuente de verdad; versionar path `/api/v1`, schemas, errores con traceId, scopes JWT, page cursor, idempotency, moneda decimal/string y enums; todo EP formalizado solo en H06.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-150-02 — Descubrimiento

**Entrada:** H04 aprobado, requisitos `A-040, A-110, E001..E075, I-030, I-090`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Definir URL de backend pública HTTPS y ruta de especificación OpenAPI o artefacto publicado con checksum, con documentación de ejemplos sin credenciales. Política CORS permite orígenes aprobados sin abrir * con credenciales.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-150-03 — Compatibilidad

**Entrada:** H04 aprobado, requisitos `A-040, A-110, E001..E075, I-030, I-090`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Matriz E001-E075: método, ruta, status, esquema, auth, trazabilidad UI, prueba negativa, persistencia; clasificación M/S/E/O no exime requisitos obligatorios adicionales del profesor (push, ML/IA si el backend interviene).

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-150-04 — Evidencia HTTP

**Entrada:** H04 aprobado, requisitos `A-040, A-110, E001..E075, I-030, I-090`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Desde cliente independiente: curl/HTTP GET live/ready, registro/login/logout, cuatro endpoints adicionales de la rúbrica, 401/403/404, HTTPS CA, persistencia Dynamo y paso por dos nodos. Tests contrato ejecutados por Backend.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-150-05 — Cambio

**Entrada:** H04 aprobado, requisitos `A-040, A-110, E001..E075, I-030, I-090`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Compatibilidad semver y deprecation de `/api/v1`, revisión de breaking changes en CI; ningún cambio sin actualizar schemas/tareas/historias.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-150-06 — Alcance

**Entrada:** H04 aprobado, requisitos `A-040, A-110, E001..E075, I-030, I-090`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** La aplicación Android, Play Store, pantallas, pruebas de integración Mobile y landing se entregan por otros equipos. No bloquear una fase de Backend hasta que móvil confirme; no atribuirles gates de nuestro API.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-150-07 — CDN/moderación

**Entrada:** H04 aprobado, requisitos `A-040, A-110, E001..E075, I-030, I-090`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Si algún EP recibe texto/imagen/video, diseñar moderación pre-storage mediante servicio cloud permitido y CDN de AWS según requisito, con costos y manejo de contenidos inapropiados; no crear EP artificiosos para archivos no presentes.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

## 3. Orden de ejecución futura H08 (no ejecutar ahora)

1. Auditar plan AWS/credenciales/precios y límites SCP sin cambios.
2. Preparar revisión de cambios, artefactos IaC y análisis económico en PR autorizado.
3. Ejecutar validaciones (fmt/validate/plan/pruebas), someter al gate humano.
4. Aprovisionar o modificar únicamente después de autorización de release y protección de datos.
5. Ejecutar pruebas reales, registrar métricas, costos y rollback, y no declarar DONE antes de las evidencias.

## 4. Dependencias cruzadas

I-030, I-060, I-080, I-090, H06

## 5. Riesgos y tratamiento

1. `I-150-publicacion-y-compatibilidad-api-R01`: Rutas opcionales asumidas obligatorias sin evidencia. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.
2. `I-150-publicacion-y-compatibilidad-api-R02`: Equipo Mobile convertido indebidamente en gate. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.
3. `I-150-publicacion-y-compatibilidad-api-R03`: Función de upload futura sin moderación/CDN. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.

## 6. Puerta de entrada a H06

Gate H05/I-GO NO se logra con instalación de Markdown: requiere pruebas de viabilidad económica/cuenta, trazabilidad física Dynamo/Redis, controles IAM y aceptación expresa del responsable. Los contratos detallados E001..E075 se formalizan exclusivamente en H06.
