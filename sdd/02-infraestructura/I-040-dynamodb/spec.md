# I-040-dynamodb — DynamoDB: tablas, claves, GSI, TTL y datos

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-060, A-070, E001..E075, I-000 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## 1. Objetivo y definición de terminado

Traducir el modelo aprobado de A-060 en definiciones IaC físicas exhaustivas, operaciones y pruebas para los patrones de acceso del prototipo sin usar MongoDB Atlas.

## Autoridad y restricciones

- `constitution.md` (GOV-01..18, R-001..060) y el Gate H04 aprobado explícitamente el **2026-11-05** tienen precedencia; H05 diseña infraestructura, no la ejecuta.
- Arquitectura ratificada: Go (Clean Architecture), AWS, DynamoDB, **Redis real**, **2 instancias simultáneas**, balanceador HTTPS y CI/CD/DevSecOps. Ninguno es demostración opcional.
- El PDF del docente v3 exige Backend cloud con costo cero; el catálogo V6.0.2 enumera 75 rutas propuestas E001..E075 y utiliza stack histórico GCP/MongoDB **descartado**.
- No crear código Go, archivos Terraform `.tf`, pipelines ni endpoints H06 durante H05; `terraform apply/destroy`, `git push`, cambios AWS y facturación están prohibidos hasta fases y aprobaciones autorizadas.
- **Regla de evidencia:** documentos pueden alcanzar DRAFT completo sin acceso cloud; Gate I-GO requiere demostrar **viabilidad operacional y económica**, no asumirla. Campos pendientes no se cambian a PASS ni se solventan con créditos.

## 2. Contrato de infraestructura propuesto

### INF-040-01 — Tabla principal

Candidata NEXUM_CORE_<env>: PK(S), SK(S); atributos GSI1PK/GSI1SK, GSI2PK/GSI2SK solo si su patrón de acceso está justificado; modo PAY_PER_REQUEST inicialmente sujeto a costos reales; SSE, deletion protection según ambiente.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-040-02 — Entidades y claves

USER#<id>/PROFILE, USER#<id>/INCOME, USER#<id>/MOVEMENT#timestamp#id, USER#<id>/GOAL#id, USER#<id>/RECURRING#id, USER#<id>/BUDGET#month#cat, USER#<id>/CONTRIBUTION#id, USER#<id>/CONSENT#version, USER#<id>/SESSION#id; reservas email únicas como EMAIL#<hash-normalizado>/OWNER mediante ConditionExpression.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-040-03 — Rutas de acceso

Query por propietario (userId derivado de JWT), fecha y tipo, GSI sparsely indexed; cursores LastEvaluatedKey firmados; nunca Scan sobre datos personales en request. Construir matriz E001-E075→operaciones (GetItem/Query/UpdateItem/TransactWriteItems).

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-040-04 — Unicidad y consistencia

Email único sin scan, registro/activación condicionado; todas mutaciones financieras versionadas con optimistic concurrency, transacciones idempotentes; up to 100 actions, max 4MB y no dos acciones sobre mismo ítem. Limitar nº metas/allocations por operación.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-040-05 — Dinero y tiempo

amountCents int64; instante occurredAt UTC y timezone IANA del usuario, fecha date-only separada; no float; no duplicar descuentos de recurrentes fijos.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-040-06 — TTL y privacidad

TTL aplicado a token de recuperación, claves temporales y sesiones caducadas cuando sea legalmente válido; TTL es eventual, nunca reemplaza comprobación de expiración en app. Backups/retención y borrado real de cuenta requieren orquestación auditable y pruebas.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-040-07 — Índices concretos

GSI1: GSI1PK/SK en movements filtrados por usuario+tipo/tiempo; GSI2: índices sparse de notificaciones/reminders por estado/próxima fecha solo con consulta justificable. Prohibido crear GSI especulativos sin medición/uso.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-040-08 — Infra Terraform futuro

aws_dynamodb_table, TTL, SSE, tags, PITR condicionado a presupuesto y riesgo (no descartable sin aprobación), y role policy recursos acotados. Los GSI/tables deben enumerarse en plan y variables sin omisiones.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

## 3. Dependencias y secuencia

A-060, I-000, I-060, I-070, I-130, H06 contratos E001..E075 sin anticipar archivos

## 4. Criterios de aceptación de diseño (Gate H05)

- Inventario completo de tablas, PK/SK, GSI y entidades 75-ruta.
- Pruebas contractuales de ownership, email único y no Scan.
- Atomicidad/consistencia en contribuciones y pagos; prueba concurrencia.
- TTL/backup y costos estimados, incluso con tabla vacía.
- IaC futuro representa todas las claves e índices aprobados y pruebas Postdeploy.

## 5. Riesgos que impiden una aprobación sin evidencia

- **Riesgo:** GSI incorrecto obliga a Scan. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.
- **Riesgo:** Retención/PITR costosa. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.
- **Riesgo:** Borrado de cuenta inconcluso. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.
- **Riesgo:** Transacciones sobre 100 elementos superan límites. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.

## 6. Entregables y referencias

Los siguientes nombres identifican evidencias/artefactos **futuros** de I-GO y de implementación; no son archivos creados en H05: `MODELO_FISICO_DYNAMODB.md`, `MATRIZ_75_ACCESOS_DDB.md`, `INVENTARIO_TABLAS_INDICES_TTL.md`.

## 7. Criterio de bloqueo y aprobación

**BLOCKED** si falta una obligación técnica, contradice alguna spec A aprobada, no puede comprobarse la viabilidad Free/SCP o no hay aceptación humana del riesgo. **DRAFT** significa que el diseño está documentado; nunca equivale a infraestructura aplicada.
