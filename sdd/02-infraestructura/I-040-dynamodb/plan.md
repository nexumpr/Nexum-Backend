# I-040-dynamodb — DynamoDB: tablas, claves, GSI, TTL y datos

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-060, A-070, E001..E075, I-000 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## 1. Estrategia de diseño documental de H05

Traducir el modelo aprobado de A-060 en definiciones IaC físicas exhaustivas, operaciones y pruebas para los patrones de acceso del prototipo sin usar MongoDB Atlas.

## Autoridad y restricciones

- `constitution.md` (GOV-01..18, R-001..060) y el Gate H04 aprobado explícitamente el **2026-11-05** tienen precedencia; H05 diseña infraestructura, no la ejecuta.
- Arquitectura ratificada: Go (Clean Architecture), AWS, DynamoDB, **Redis real**, **2 instancias simultáneas**, balanceador HTTPS y CI/CD/DevSecOps. Ninguno es demostración opcional.
- El PDF del docente v3 exige Backend cloud con costo cero; el catálogo V6.0.2 enumera 75 rutas propuestas E001..E075 y utiliza stack histórico GCP/MongoDB **descartado**.
- No crear código Go, archivos Terraform `.tf`, pipelines ni endpoints H06 durante H05; `terraform apply/destroy`, `git push`, cambios AWS y facturación están prohibidos hasta fases y aprobaciones autorizadas.
- **Regla de evidencia:** documentos pueden alcanzar DRAFT completo sin acceso cloud; Gate I-GO requiere demostrar **viabilidad operacional y económica**, no asumirla. Campos pendientes no se cambian a PASS ni se solventan con créditos.

## 2. Plan secuencial y verificaciones

### P-040-01 — Tabla principal

**Entrada:** H04 aprobado, requisitos `A-060, A-070, E001..E075, I-000`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Candidata NEXUM_CORE_<env>: PK(S), SK(S); atributos GSI1PK/GSI1SK, GSI2PK/GSI2SK solo si su patrón de acceso está justificado; modo PAY_PER_REQUEST inicialmente sujeto a costos reales; SSE, deletion protection según ambiente.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-040-02 — Entidades y claves

**Entrada:** H04 aprobado, requisitos `A-060, A-070, E001..E075, I-000`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** USER#<id>/PROFILE, USER#<id>/INCOME, USER#<id>/MOVEMENT#timestamp#id, USER#<id>/GOAL#id, USER#<id>/RECURRING#id, USER#<id>/BUDGET#month#cat, USER#<id>/CONTRIBUTION#id, USER#<id>/CONSENT#version, USER#<id>/SESSION#id; reservas email únicas como EMAIL#<hash-normalizado>/OWNER mediante ConditionExpression.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-040-03 — Rutas de acceso

**Entrada:** H04 aprobado, requisitos `A-060, A-070, E001..E075, I-000`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Query por propietario (userId derivado de JWT), fecha y tipo, GSI sparsely indexed; cursores LastEvaluatedKey firmados; nunca Scan sobre datos personales en request. Construir matriz E001-E075→operaciones (GetItem/Query/UpdateItem/TransactWriteItems).

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-040-04 — Unicidad y consistencia

**Entrada:** H04 aprobado, requisitos `A-060, A-070, E001..E075, I-000`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Email único sin scan, registro/activación condicionado; todas mutaciones financieras versionadas con optimistic concurrency, transacciones idempotentes; up to 100 actions, max 4MB y no dos acciones sobre mismo ítem. Limitar nº metas/allocations por operación.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-040-05 — Dinero y tiempo

**Entrada:** H04 aprobado, requisitos `A-060, A-070, E001..E075, I-000`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** amountCents int64; instante occurredAt UTC y timezone IANA del usuario, fecha date-only separada; no float; no duplicar descuentos de recurrentes fijos.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-040-06 — TTL y privacidad

**Entrada:** H04 aprobado, requisitos `A-060, A-070, E001..E075, I-000`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** TTL aplicado a token de recuperación, claves temporales y sesiones caducadas cuando sea legalmente válido; TTL es eventual, nunca reemplaza comprobación de expiración en app. Backups/retención y borrado real de cuenta requieren orquestación auditable y pruebas.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-040-07 — Índices concretos

**Entrada:** H04 aprobado, requisitos `A-060, A-070, E001..E075, I-000`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** GSI1: GSI1PK/SK en movements filtrados por usuario+tipo/tiempo; GSI2: índices sparse de notificaciones/reminders por estado/próxima fecha solo con consulta justificable. Prohibido crear GSI especulativos sin medición/uso.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-040-08 — Infra Terraform futuro

**Entrada:** H04 aprobado, requisitos `A-060, A-070, E001..E075, I-000`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** aws_dynamodb_table, TTL, SSE, tags, PITR condicionado a presupuesto y riesgo (no descartable sin aprobación), y role policy recursos acotados. Los GSI/tables deben enumerarse en plan y variables sin omisiones.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

## 3. Orden de ejecución futura H08 (no ejecutar ahora)

1. Auditar plan AWS/credenciales/precios y límites SCP sin cambios.
2. Preparar revisión de cambios, artefactos IaC y análisis económico en PR autorizado.
3. Ejecutar validaciones (fmt/validate/plan/pruebas), someter al gate humano.
4. Aprovisionar o modificar únicamente después de autorización de release y protección de datos.
5. Ejecutar pruebas reales, registrar métricas, costos y rollback, y no declarar DONE antes de las evidencias.

## 4. Dependencias cruzadas

A-060, I-000, I-060, I-070, I-130, H06 contratos E001..E075 sin anticipar archivos

## 5. Riesgos y tratamiento

1. `I-040-dynamodb-R01`: GSI incorrecto obliga a Scan. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.
2. `I-040-dynamodb-R02`: Retención/PITR costosa. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.
3. `I-040-dynamodb-R03`: Borrado de cuenta inconcluso. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.
4. `I-040-dynamodb-R04`: Transacciones sobre 100 elementos superan límites. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.

## 6. Puerta de entrada a H06

Gate H05/I-GO NO se logra con instalación de Markdown: requiere pruebas de viabilidad económica/cuenta, trazabilidad física Dynamo/Redis, controles IAM y aceptación expresa del responsable. Los contratos detallados E001..E075 se formalizan exclusivamente en H06.
