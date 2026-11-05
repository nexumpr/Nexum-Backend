# A-060 — Plan técnico de arquitectura

**Estado:** DRAFT H04. **Entrada obligatoria:** `spec.md` y fuentes de A-060. **Salida:** decisión lista para implementar después de H07; este plan NO ejecuta infraestructura ni código.

## 1. Estrategia de construcción

1. **Tablas y modo.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Propuesta técnica: tabla NEXUM_CORE con PK (S) y SK (S) y GSI1PK/GSI1SK (sparse); modo on-demand en diseño, capacidad/costo a validar H05. Tabla auxiliar NEXUM_IDENTITY_INDEX solo si unicidad de email no cabe en la tabla principal; preferir índices únicos mediante items en NEXUM_CORE y ConditionExpression.
2. **Identidad global.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. USER#<uuidv7> / PROFILE; EMAIL#<sha256(normalized-email)> / OWNER => userId reservado transaccionalmente, no email claro como PK; AUTH#<hash-token> / SESSION o LOOKUP#SESSION#... para renovar sin usuarioId. Evitar exponer hashes crudos; secreto pepper según caso.
3. **Partición usuario.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. PK=USER#<id> SK=PROFILE|INCOME|ONBOARDING|PREFERENCES|RECURRING#<id>|GOAL#<id>|BUDGET#<period>#<category>|MOVEMENT#<RFC3339-sortable>#<uuid>|CONTRIBUTION#<id>|SESSION#<id>|CONSENT#<version>|REMINDER#<id>; no Scan.
4. **Índices y acceso.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. GSI1PK=USER#id#TYPE#<type>; GSI1SK=timestamp#uuid para listados filtrados; GSI sparse para status/status-due pendientes cuando H06 confirme acceso. `Query` sobre partition y begin/end SK, cursores LastEvaluatedKey asociados a identidad y filtros.
5. **Movimientos e integridad.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Registro de movimiento guarda amountCents int64, type ant|fixed|saving, categoryId, occurredAt UTC, timezone, contributionId/goalId/recurringPaymentId opcionales, createdAt, version, tombstone/reversal. Proyección de sumarios con versión o lectura calculada, prohibido doble cargo fixed.
6. **Aportación compuesta.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. TransactWriteItems escribe identificador idempotente, group contribution, allocations/movements y actualiza cada goal mediante ADD/condition, en una transacción (<100 acciones y <4MB). Para grandes lotes limitar max metas por request o fragmentar solo con saga especificada; nunca escribir parcialmente bajo apariencia de éxito.
7. **Límites y concurrencia.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. ConditionExpression `version=:old`, uniqueness, transaction cancellation mapping 409, retry con jitter solo condiciones transitorias; no reintentar conflicto de negocio ciegamente. DynamoDB TransactWriteItems hasta 100 acciones/4MB, medir consumo y límites.
8. **TTL y eliminación.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. TTL epoch seconds solo limpieza eventual de tokens/ítems no críticos; expirados se rechazan en lectura inmediatamente. Borrado de cuenta requiere enumerar por PK usuario y proceso auditable con job; backups/retención resueltos H05.
9. **Patrones de acceso obligatorios.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. E027 movimientos por rango+tipo, E038 metas por estado, E032 recurrentes activos, E048 budgets por periodo, E056 racha, E051 dashboard sin Scan, E008 sesiones activas, E059 push por dispositivo, E014 borrar cuenta. Matriz especifica para cada endpoint en A060 anexo.
10. **Validación de diseño.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Simular cardinalidades 1, 100, 2000, 50000 movimientos; Query con filtros y cursor, concurrencia multi-node, skew PK, consumo de GSI, atomicidad; costo estimado antes de H05 APPROVED.

## 2. Orden interno de diseño y dependencias

1. Revisar `constitution.md` y `AGENTS.md` antes de aceptar cambios; cotejar los PDF/prototipo cuyos nombres y hashes se registran en H04.
2. Identificar invariantes y entradas/salidas de cada módulo; mapear al catálogo E001..075 sin crear contratos endpoints H06.
3. Verificar acoplamiento con A-020 (capas), A-040 (DTOs), A-050 (auth), A-060 (persistencia), A-070 (consistencia), A-080 (Redis), A-090 (telemetría) y A-100 (QA).
4. Mantener como *blocker de H05* capacidad/costo de 2 instancias + balanceador + Redis, restricciones SCP/IAM, TLS y mecanismos de on/off; no inferir disponibilidad de AWS Free.
5. Registrar la decisión humana y conflictos si alguna regla altera el negocio del prototipo; no solicitar gate Mobile.

## 3. Estrategia de pruebas posteriores por capa

| Nivel | Diseño a comprobar | Evidencia futura obligatoria |
|---|---|---|
| Dominio | Invariantes y money/identidad | go test, race detector, entradas límite |
| Aplicación | Orquestación con mocks y transacciones | unit tests, casos negativos, retries |
| Infraestructura | DynamoDB/Redis reales según fase | integración, latencia, permisos mínimos, gasto |
| HTTP | Schemas OpenAPI 3.1 y respuestas | pruebas contractuales y HTTP reales |
| CI/CD | Seguridad y releases | PR checks, escaneo, autorización humana |

## 4. Riesgos y mitigación explícita

- Alta concentración de movimientos en un solo PK para usuarios extremos: evaluar sharding lógico si datos lo demandan, con pruebas de consultas. **Mitigación:** resolver antes de aprobar su dependencia; si requiere cambio de constitución emitir RFC.
- El límite 100 acciones obliga a fijar número máximo de metas en contribución dividida para atomicidad. **Mitigación:** resolver antes de aprobar su dependencia; si requiere cambio de constitución emitir RFC.
- La unicidad de email requiere reserva transaccional no un GSI eventualmente consistente. **Mitigación:** resolver antes de aprobar su dependencia; si requiere cambio de constitución emitir RFC.

## 5. Criterio de cierre

Se considera listo para revisión cuando los 10 ejes técnicos de `spec.md` tienen trazabilidad a pruebas en `checklist.md`, los elementos futuros permanecen `NOT_RUN`, la matriz de alcance está intacta y las decisiones pendientes tienen dueño/fase, sin `APPROVED` automático.
