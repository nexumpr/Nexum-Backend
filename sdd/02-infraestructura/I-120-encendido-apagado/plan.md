# I-120-encendido-apagado — Encendido, apagado y preservación de datos

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | I-000, I-020, I-030, I-040, I-050 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## 1. Estrategia de diseño documental de H05

Permitir ventanas de prueba con dos instancias simultáneas y runtime desconectado fuera de uso, sin falsificar cero costo ni borrar los datos de DynamoDB.

## Autoridad y restricciones

- `constitution.md` (GOV-01..18, R-001..060) y el Gate H04 aprobado explícitamente el **2026-11-05** tienen precedencia; H05 diseña infraestructura, no la ejecuta.
- Arquitectura ratificada: Go (Clean Architecture), AWS, DynamoDB, **Redis real**, **2 instancias simultáneas**, balanceador HTTPS y CI/CD/DevSecOps. Ninguno es demostración opcional.
- El PDF del docente v3 exige Backend cloud con costo cero; el catálogo V6.0.2 enumera 75 rutas propuestas E001..E075 y utiliza stack histórico GCP/MongoDB **descartado**.
- No crear código Go, archivos Terraform `.tf`, pipelines ni endpoints H06 durante H05; `terraform apply/destroy`, `git push`, cambios AWS y facturación están prohibidos hasta fases y aprobaciones autorizadas.
- **Regla de evidencia:** documentos pueden alcanzar DRAFT completo sin acceso cloud; Gate I-GO requiere demostrar **viabilidad operacional y económica**, no asumirla. Campos pendientes no se cambian a PASS ni se solventan con créditos.

## 2. Plan secuencial y verificaciones

### P-120-01 — Estados

**Entrada:** H04 aprobado, requisitos `I-000, I-020, I-030, I-040, I-050`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** `OFF` runtime efímero deshabilitado; `STARTING`, `ON_2_HEALTHY` (único estado operativo validable), `DRAINING`, `ERROR`, `COST_REVIEW`. Toda transición requiere actor, timestamp y runbook; datos persisten.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-120-02 — Encender

**Entrada:** H04 aprobado, requisitos `I-000, I-020, I-030, I-040, I-050`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Aprobación humana, check plan/credits, desiredCapacity=2 y healthcheck de targets y Redis; no exponer URL hasta 2 healthy; pruebas HTTP y auditoría de costo inicial.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-120-03 — Apagar

**Entrada:** H04 aprobado, requisitos `I-000, I-020, I-030, I-040, I-050`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Drenar requests, bloquear trabajos, recoger logs, bajar o destruir runtime efímero autorizado; detener EC2 no equivale a eliminar ALB/IPv4/Redis/EBS/billing. Persistir Dynamo y Terraform state; registrar recursos residuales con precios.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-120-04 — DNS/ALB

**Entrada:** H04 aprobado, requisitos `I-000, I-020, I-030, I-040, I-050`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Evaluar si ALB puede permanecer y cuánto factura; modo más económico puede requerir destruir ALB y reprovisionar posteriormente (cambio DNS y TLS), previa autorización. No eliminar tablas por defecto.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-120-05 — Seguridad

**Entrada:** H04 aprobado, requisitos `I-000, I-020, I-030, I-040, I-050`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Nunca utilizar destroy-all en un script de apagado. Separar explícitamente `destroy-runtime` de `destroy-data` y `destroy-state` con protección fuerte.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-120-06 — Prueba

**Entrada:** H04 aprobado, requisitos `I-000, I-020, I-030, I-040, I-050`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Tras OFF, obtener Resource Inventory y Billing, verificar datos en Dynamo intactos y restaurar ON con 2 nodos; comparar costos reales.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

## 3. Orden de ejecución futura H08 (no ejecutar ahora)

1. Auditar plan AWS/credenciales/precios y límites SCP sin cambios.
2. Preparar revisión de cambios, artefactos IaC y análisis económico en PR autorizado.
3. Ejecutar validaciones (fmt/validate/plan/pruebas), someter al gate humano.
4. Aprovisionar o modificar únicamente después de autorización de release y protección de datos.
5. Ejecutar pruebas reales, registrar métricas, costos y rollback, y no declarar DONE antes de las evidencias.

## 4. Dependencias cruzadas

I-000, I-020, I-030, I-040, I-050, I-070, I-130

## 5. Riesgos y tratamiento

1. `I-120-encendido-apagado-R01`: Apagado incompleto mantiene ALB/Redis facturados. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.
2. `I-120-encendido-apagado-R02`: Destroy accidental de datos. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.
3. `I-120-encendido-apagado-R03`: Gastos mientras runtime OFF. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.

## 6. Puerta de entrada a H06

Gate H05/I-GO NO se logra con instalación de Markdown: requiere pruebas de viabilidad económica/cuenta, trazabilidad física Dynamo/Redis, controles IAM y aceptación expresa del responsable. Los contratos detallados E001..E075 se formalizan exclusivamente en H06.
