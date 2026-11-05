# I-140-operacion — Runbooks, incidentes y verificación operacional

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-090, I-020, I-030, I-070, I-090 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## 1. Objetivo y definición de terminado

Definir operación sostenible, respuesta a fallos, pruebas de salud y control de cambios para Backend con dos instancias y dependencia Dynamo/Redis.

## Autoridad y restricciones

- `constitution.md` (GOV-01..18, R-001..060) y el Gate H04 aprobado explícitamente el **2026-11-05** tienen precedencia; H05 diseña infraestructura, no la ejecuta.
- Arquitectura ratificada: Go (Clean Architecture), AWS, DynamoDB, **Redis real**, **2 instancias simultáneas**, balanceador HTTPS y CI/CD/DevSecOps. Ninguno es demostración opcional.
- El PDF del docente v3 exige Backend cloud con costo cero; el catálogo V6.0.2 enumera 75 rutas propuestas E001..E075 y utiliza stack histórico GCP/MongoDB **descartado**.
- No crear código Go, archivos Terraform `.tf`, pipelines ni endpoints H06 durante H05; `terraform apply/destroy`, `git push`, cambios AWS y facturación están prohibidos hasta fases y aprobaciones autorizadas.
- **Regla de evidencia:** documentos pueden alcanzar DRAFT completo sin acceso cloud; Gate I-GO requiere demostrar **viabilidad operacional y económica**, no asumirla. Campos pendientes no se cambian a PASS ni se solventan con créditos.

## 2. Contrato de infraestructura propuesto

### INF-140-01 — Runbooks

Crear procedimientos para deployment, health fail, Dynamo throttle/timeout, Redis unavailable, certificados, secrets rotation, 401/403 no esperados, contención de presupuesto, incidentes y eliminación IaC segura.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-140-02 — Prioridades

S1 pérdida/filtración datos o servicio, S2 degradación parcial, S3 warnings; escalamiento humano, tiempos de notificación y rollback.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-140-03 — Drills

Aislar instancia A y validar B, reemplazo target, fallo Redis con degradación definida, cuotas Dynamo y timeout, rotación JWT; solo H08 con autorización y datos sintéticos.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-140-04 — Prueba y aceptación

Comparar implementación contra 16 specs I, 12 specs A y contratos E; URL HTTPS/HTTP real, log/trace y Dynamo persistente. Registrar evidencia por escenario.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-140-05 — Acceso

Operación segura con identidades temporales; no shell de root, no abrir SSH público o cambiar SCP a ciegas.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-140-06 — Inventario final

Lista de recursos/owners/tags, servicios 24×7 vs efímeros y facturación residual. Nombres de recursos no hacen prueba de existencia.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

## 3. Dependencias y secuencia

I-020, I-030, I-050, I-090, I-110, I-120, I-130

## 4. Criterios de aceptación de diseño (Gate H05)

- Plan de respuesta con diagnóstico, mitigación, rollback y cierre.
- Drills de caída por componente documentados.
- 2 targets saludables en modo ON.
- Procedimiento de eliminación IaC no toca datos sin aprobación.
- Matriz responsabilidades de operación solo Backend.

## 5. Riesgos que impiden una aprobación sin evidencia

- **Riesgo:** No hay acceso de recuperación a nodos. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.
- **Riesgo:** Inconsistencia tras retry. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.
- **Riesgo:** Ausencia de registros para incidentes. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.

## 6. Entregables y referencias

Los siguientes nombres identifican evidencias/artefactos **futuros** de I-GO y de implementación; no son archivos creados en H05: `RUNBOOK_OPERATIVO.md`, `MATRIZ_INCIDENTES.md`, `PLAN_PRUEBAS_OPERACION.md`.

## 7. Criterio de bloqueo y aprobación

**BLOCKED** si falta una obligación técnica, contradice alguna spec A aprobada, no puede comprobarse la viabilidad Free/SCP o no hay aceptación humana del riesgo. **DRAFT** significa que el diseño está documentado; nunca equivale a infraestructura aplicada.
