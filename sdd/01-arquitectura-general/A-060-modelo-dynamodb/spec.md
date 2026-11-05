# A-060 — Modelo DynamoDB, consultas, índices y transacciones

| Campo | Valor |
|---|---|
| Fase | H04 — Arquitectura general (lote consolidado) |
| Versión | 0.2.0 |
| Estado | **DRAFT — Gate H04 pendiente** |
| Trazabilidad | E001-E075, A-070, GOV-08 |
| Proveedor vigente | AWS; DynamoDB NoSQL; Redis real; Backend Go |
| Fuente original | ZIP Android V6.0.2 `(3)`; PDF 75 rutas `(2)`; PDF docente v3 |

## 1. Objetivo y condiciones de aceptación

Esta especificación establece los **requisitos de diseño vinculantes propuestos**, no código implementado. Se rechazan decisiones sin trazabilidad, seguridad o viabilidad. Si contradice `constitution.md`, el Gate se bloquea y se tramita RFC. No crea recursos AWS, endpoints HTTP reales ni modifica cliente móvil.

## Base documental y autoridad

1. Autoridad superior: `constitution.md` v0.2.0, `AGENTS.md`, H03 humano APPROVED (2026-11-05). Ningún texto de esta entrega cambia permisos ni política.
2. Fuente académica: **Requerimientos para el desarrollo del proyecto Móvil v3(2).pdf**, cuatro páginas; requisitos de Backend y dependencias transversales. Se consultó íntegramente en esta entrega.
3. Fuente funcional: **Nexum_V6_0_2_AndroidStudio_Demo_Eliminar_Cuenta(3).zip**, 16 entradas ZIP. Se leyeron `app.js`, `index.html`, `MainActivity.java`, `AndroidManifest.xml`, `README.md` y se comprobó ausencia de `fetch(` en `app.js`. Código cliente no constituye Backend existente.
4. Catálogo: **Nexum_V6_0_2_Catalogo_Completo_Endpoints_Backend(2).pdf**, 27 páginas; 75 identificadores/rutas propuestas E001..E075. Etiquetas M/S/E/O preservadas. **No adoptar** su stack histórico Google Cloud/MongoDB; el stack vigente del proyecto es **AWS + DynamoDB + Redis**.
5. Evidencia H03: 54 decisiones del motor real (53 DENY, 1 ALLOW) y aprobación humana del Gate H03; ejecución en Windows aportada en Warp. En este lote NO se llama a OpenCode y sus permisos siguen restringidos.

**Trazabilidad:** la etiqueta M/S/E/O describe el origen de la ruta, no constituye por sí sola decisión de implementación ni reemplaza lo exigido por el docente. La formalización OpenAPI por ruta y su aprobación corresponde a H06. Las decisiones físicas de despliegue y costo cero corresponden a H05 y no están presumidas.


## 2. Contrato arquitectónico

### 1. Tablas y modo

Propuesta técnica: tabla NEXUM_CORE con PK (S) y SK (S) y GSI1PK/GSI1SK (sparse); modo on-demand en diseño, capacidad/costo a validar H05. Tabla auxiliar NEXUM_IDENTITY_INDEX solo si unicidad de email no cabe en la tabla principal; preferir índices únicos mediante items en NEXUM_CORE y ConditionExpression.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 2. Identidad global

USER#<uuidv7> / PROFILE; EMAIL#<sha256(normalized-email)> / OWNER => userId reservado transaccionalmente, no email claro como PK; AUTH#<hash-token> / SESSION o LOOKUP#SESSION#... para renovar sin usuarioId. Evitar exponer hashes crudos; secreto pepper según caso.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 3. Partición usuario

PK=USER#<id> SK=PROFILE|INCOME|ONBOARDING|PREFERENCES|RECURRING#<id>|GOAL#<id>|BUDGET#<period>#<category>|MOVEMENT#<RFC3339-sortable>#<uuid>|CONTRIBUTION#<id>|SESSION#<id>|CONSENT#<version>|REMINDER#<id>; no Scan.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 4. Índices y acceso

GSI1PK=USER#id#TYPE#<type>; GSI1SK=timestamp#uuid para listados filtrados; GSI sparse para status/status-due pendientes cuando H06 confirme acceso. `Query` sobre partition y begin/end SK, cursores LastEvaluatedKey asociados a identidad y filtros.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 5. Movimientos e integridad

Registro de movimiento guarda amountCents int64, type ant|fixed|saving, categoryId, occurredAt UTC, timezone, contributionId/goalId/recurringPaymentId opcionales, createdAt, version, tombstone/reversal. Proyección de sumarios con versión o lectura calculada, prohibido doble cargo fixed.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 6. Aportación compuesta

TransactWriteItems escribe identificador idempotente, group contribution, allocations/movements y actualiza cada goal mediante ADD/condition, en una transacción (<100 acciones y <4MB). Para grandes lotes limitar max metas por request o fragmentar solo con saga especificada; nunca escribir parcialmente bajo apariencia de éxito.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 7. Límites y concurrencia

ConditionExpression `version=:old`, uniqueness, transaction cancellation mapping 409, retry con jitter solo condiciones transitorias; no reintentar conflicto de negocio ciegamente. DynamoDB TransactWriteItems hasta 100 acciones/4MB, medir consumo y límites.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 8. TTL y eliminación

TTL epoch seconds solo limpieza eventual de tokens/ítems no críticos; expirados se rechazan en lectura inmediatamente. Borrado de cuenta requiere enumerar por PK usuario y proceso auditable con job; backups/retención resueltos H05.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 9. Patrones de acceso obligatorios

E027 movimientos por rango+tipo, E038 metas por estado, E032 recurrentes activos, E048 budgets por periodo, E056 racha, E051 dashboard sin Scan, E008 sesiones activas, E059 push por dispositivo, E014 borrar cuenta. Matriz especifica para cada endpoint en A060 anexo.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 10. Validación de diseño

Simular cardinalidades 1, 100, 2000, 50000 movimientos; Query con filtros y cursor, concurrencia multi-node, skew PK, consumo de GSI, atomicidad; costo estimado antes de H05 APPROVED.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

## 3. Límites del alcance

- Solamente Backend, datos, seguridad, contratos API e infraestructura gestionada por el equipo Backend; ninguna implementación de Android, landing, Play Store o aprobación del equipo móvil.
- H04 congela **arquitectura lógica**; H05 congela viabilidad y topología física; H06 congela OpenAPI detallado de cada operación; H08 implementa conforme a esas decisiones.
- No convertir opciones propuestas de un catálogo histórico en compromisos silenciosos; no hacer optativas las obligaciones docentes.

## 4. Riesgos específicos y decisiones que deberán sustentarse

- **R-A-060-01:** Alta concentración de movimientos en un solo PK para usuarios extremos: evaluar sharding lógico si datos lo demandan, con pruebas de consultas.
- **R-A-060-02:** El límite 100 acciones obliga a fijar número máximo de metas en contribución dividida para atomicidad.
- **R-A-060-03:** La unicidad de email requiere reserva transaccional no un GSI eventualmente consistente.

## 5. Condiciones de aceptación demostrables

- **AC-A-060-01:** Nunca Scan; solo Query o GetItem/BatchGetItem con índices justificados.
- **AC-A-060-02:** Transacción con falla de condición no deja goal.saved sin movement.
- **AC-A-060-03:** Diseño de índices cubre los 75 IDs incluidas rutas O como opciones no obligatorias.

## 6. Trazabilidad y desbloqueo

- H04: aprobar este diseño en conjunto con otros once, sin ocultar discrepancias de fuentes.
- H05: seleccionar recursos AWS permitidos, demostrar costo cero y capacidad para DOS instancias simultáneas + LB/HTTPS + Redis sin simulaciones.
- H06: ratificar las 75 rutas con contratos y categorías M/S/E/O y su cobertura contra pantallas; incluir las operaciones académicas.
- H07: comprobar contradicciones, seguridad, aceptación y que nada indispensable se pospone a implementación por falta de diseño.
- H08: cada caso se implementa y prueba de forma real, aislada, con autorización de release.

## 7. Inventario lógico de entidades y patrones de acceso

| Entidad | Clave propuesta | Access pattern | Invariante de integridad |
|---|---|---|---|
| User/Profile | USER#id / PROFILE | get por sub JWT | userId no tomado del cliente |
| Email reservation | EMAIL#digest / OWNER | GetItem por email normalizado + condición | unicidad transaccional |
| Session/token lookup | TOKEN#digest / REF; USER#id / SESSION#id | refresh por hash y sesiones del titular | rotación CAS/replay |
| Consent | USER#id / CONSENT#version#time | historial por usuario | append-only versionado |
| Income | USER#id / INCOME | Get/Put ingreso | máximo un ingreso vigente |
| Recurring | USER#id / RECURRING#id | Query por PK usuario y status sparse | regla ≠ pago realizado |
| Movement | USER#id / MOVEMENT#date#id | Query rangos y `GSI1` por tipo | inmutable vínculo de aporte |
| Goal | USER#id / GOAL#id | Query por PK y GSI status | saved=opening+sum(active allocations) |
| Contribution | USER#id / CONTRIBUTION#id | Get por id y Query por usuario | grupo y asignaciones atómicos |
| Budget | USER#id / BUDGET#month#category | Query por mes | límites >=0, cero desactiva |
| Notification/Reminder | USER#id / NOTIF#timestamp#id etc. | Query por usuario y estado | opt-in, privacidad |
| Idempotency | IDEM#user#op / key-digest | Get/Transact write | payload hash y respuesta estable |
| Catalog/Notice | PUBLIC#CATALOG / version | Get sin auth donde autorizado | datos y versión controlados |

**Reglas de implementación del adaptador:** establecer en H05 el diseño físico y los costos de las tablas/GSIs sobre los patrones lógicos de H04; en H06 completar la matriz por cada ruta con claves, cardinalidad, proyecciones, lectura/escritura y consistencia antes de aprobar H06. No exigir los contratos finales de H06 como precondición de iniciar H05. **No** usar Scan global. Si concurrencia y cuotas de transacciones no permiten cumplir el flujo de split, reducir número máximo de metas por request y documentar 422, no permitir éxito parcial.
