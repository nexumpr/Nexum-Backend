# I-140-operacion — Runbooks, incidentes y verificación operacional

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-090, I-020, I-030, I-070, I-090 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## 1. Estrategia de diseño documental de H05

Definir operación sostenible, respuesta a fallos, pruebas de salud y control de cambios para Backend con dos instancias y dependencia Dynamo/Redis.

## Autoridad y restricciones

- `constitution.md` (GOV-01..18, R-001..060) y el Gate H04 aprobado explícitamente el **2026-11-05** tienen precedencia; H05 diseña infraestructura, no la ejecuta.
- Arquitectura ratificada: Go (Clean Architecture), AWS, DynamoDB, **Redis real**, **2 instancias simultáneas**, balanceador HTTPS y CI/CD/DevSecOps. Ninguno es demostración opcional.
- El PDF del docente v3 exige Backend cloud con costo cero; el catálogo V6.0.2 enumera 75 rutas propuestas E001..E075 y utiliza stack histórico GCP/MongoDB **descartado**.
- No crear código Go, archivos Terraform `.tf`, pipelines ni endpoints H06 durante H05; `terraform apply/destroy`, `git push`, cambios AWS y facturación están prohibidos hasta fases y aprobaciones autorizadas.
- **Regla de evidencia:** documentos pueden alcanzar DRAFT completo sin acceso cloud; Gate I-GO requiere demostrar **viabilidad operacional y económica**, no asumirla. Campos pendientes no se cambian a PASS ni se solventan con créditos.

## 2. Plan secuencial y verificaciones

### P-140-01 — Runbooks

**Entrada:** H04 aprobado, requisitos `A-090, I-020, I-030, I-070, I-090`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Crear procedimientos para deployment, health fail, Dynamo throttle/timeout, Redis unavailable, certificados, secrets rotation, 401/403 no esperados, contención de presupuesto, incidentes y eliminación IaC segura.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-140-02 — Prioridades

**Entrada:** H04 aprobado, requisitos `A-090, I-020, I-030, I-070, I-090`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** S1 pérdida/filtración datos o servicio, S2 degradación parcial, S3 warnings; escalamiento humano, tiempos de notificación y rollback.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-140-03 — Drills

**Entrada:** H04 aprobado, requisitos `A-090, I-020, I-030, I-070, I-090`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Aislar instancia A y validar B, reemplazo target, fallo Redis con degradación definida, cuotas Dynamo y timeout, rotación JWT; solo H08 con autorización y datos sintéticos.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-140-04 — Prueba y aceptación

**Entrada:** H04 aprobado, requisitos `A-090, I-020, I-030, I-070, I-090`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Comparar implementación contra 16 specs I, 12 specs A y contratos E; URL HTTPS/HTTP real, log/trace y Dynamo persistente. Registrar evidencia por escenario.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-140-05 — Acceso

**Entrada:** H04 aprobado, requisitos `A-090, I-020, I-030, I-070, I-090`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Operación segura con identidades temporales; no shell de root, no abrir SSH público o cambiar SCP a ciegas.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-140-06 — Inventario final

**Entrada:** H04 aprobado, requisitos `A-090, I-020, I-030, I-070, I-090`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Lista de recursos/owners/tags, servicios 24×7 vs efímeros y facturación residual. Nombres de recursos no hacen prueba de existencia.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

## 3. Orden de ejecución futura H08 (no ejecutar ahora)

1. Auditar plan AWS/credenciales/precios y límites SCP sin cambios.
2. Preparar revisión de cambios, artefactos IaC y análisis económico en PR autorizado.
3. Ejecutar validaciones (fmt/validate/plan/pruebas), someter al gate humano.
4. Aprovisionar o modificar únicamente después de autorización de release y protección de datos.
5. Ejecutar pruebas reales, registrar métricas, costos y rollback, y no declarar DONE antes de las evidencias.

## 4. Dependencias cruzadas

I-020, I-030, I-050, I-090, I-110, I-120, I-130

## 5. Riesgos y tratamiento

1. `I-140-operacion-R01`: No hay acceso de recuperación a nodos. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.
2. `I-140-operacion-R02`: Inconsistencia tras retry. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.
3. `I-140-operacion-R03`: Ausencia de registros para incidentes. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.

## 6. Puerta de entrada a H06

Gate H05/I-GO NO se logra con instalación de Markdown: requiere pruebas de viabilidad económica/cuenta, trazabilidad física Dynamo/Redis, controles IAM y aceptación expresa del responsable. Los contratos detallados E001..E075 se formalizan exclusivamente en H06.
