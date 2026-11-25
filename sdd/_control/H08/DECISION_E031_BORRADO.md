# Decisión E031 — Borrado de movimientos: tombstone lógico y exclusión de agregados

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/DECISION_E031_BORRADO.md` |
| Versión | 1.1.0 |
| Fecha | 2026-11-24 |
| Estado | **APROBADA — implementada en local** |
| Autorizaciones humanas | `APRUEBO_DECISION_E031_BORRADO_LOGICO_TOMBSTONE` · `APRUEBO_E031_IDEMPOTENTE_204_CON_PUNTERO` · `APRUEBO_CIERRE_IMPLEMENTACION_E031_TOMBSTONE` |
| Alcance | Decisión de diseño y contrato interno de persistencia. **No** cambia OpenAPI (`204`/`404`/`409` ya existen), no toca AWS |
| Base verificada | `main` `f3bccca8b2437dc154bedbef257a3cabc7620844`; implementación en la rama `fix/h08e-deuda-historica` |
| Instrumentos relacionados | `sdd/03-endpoints/E031-movements-movementId/spec.md`; `sdd/01-arquitectura-general/A-060-modelo-dynamodb/spec.md`; RFC-022 §7.3; `MATRIZ_AUDITORIA_54_ENDPOINTS.md` (E031) |

> **Aviso.** La decisión está concedida. `APRUEBO_E031_IDEMPOTENTE_204_CON_PUNTERO` fija la
> variante vigente: el **segundo `DELETE` responde `204` idempotente** y el localizador
> `MOVEMENTPTR#<id>` **se conserva** marcado con `deleted=true`. Las secciones §3.2, §3.3, §4 y §6
> de la versión 1.0.0 que decían «puntero eliminado» y «segundo intento `404`» quedan **sustituidas**
> por este texto. La implementación local está cerrada; el despliegue AWS exige su Gate propio.

---

## 1. El conflicto, con evidencia

| Fuente | Texto / hecho | Línea |
|---|---|---|
| `E031/spec.md` §1 | «Soft delete o reverso auditable; no permitir borrar registro ligado a aportaciones confirmadas sin reversa.» | :18 |
| `E031/spec.md` §4 | «Invariante: Soft delete o reverso auditable; …» | :42 |
| `A-060/spec.md` §modelo | «Registro de movimiento guarda amountCents int64, type …, createdAt, version, **tombstone/reversal**.» | :55 |
| `A-060/spec.md` §TTL | «TTL epoch seconds solo limpieza eventual de tokens/ítems no críticos…» | :73 |
| Runtime `internal/application/movements/service.go` | `Delete` lee, valida y delega; **ya escribe tombstone** (v1.1.0) | :494-564 |
| Runtime `internal/adapters/dynamo/movements_store.go` | `DeleteMovement` ejecutaba **`Delete` físico** del ítem principal y de su localizador en una transacción (estado que motivó esta decisión) | v1.0.0, sustituido |
| Runtime `internal/adapters/dynamo/movements_store.go` | La escritura actual es un `Update` de tombstone con CAS de versión y filtro de borrado | §3.2 |
| Contrato | `DELETE /api/v1/movements/{movementId}` → `204` sin cuerpo; códigos `204, 400, 401, 403, 404, 409, 422, 429, 500, 503` | OpenAPI `nexum_e031` |

Conclusión de hecho: **la especificación y la arquitectura exigen soft delete/tombstone; el
runtime borra físicamente.** No es una ambigüedad: son dos comportamientos incompatibles. El
borrado físico, además, destruye la evidencia necesaria para detectar dobles conteos y hace
irreversible cualquier error de un cliente.

### 1.1 Precedente ya implementado en el mismo proyecto

E042 (metas) resolvió el mismo problema con un tombstone: `deleted=true`, `status=archived`,
`updatedAt`, `ADD version :one` y condición de versión (`internal/adapters/dynamo/goals_store.go:856-857`).
E040/E041/E038/E051 excluyen `deleted=true`. La decisión de E031 debe ser **coherente** con ese
patrón, no inventar uno nuevo.

## 2. Opciones evaluadas

### Opción A — Mantener el borrado físico y enmendar la documentación

- Ventaja: máxima minimización de datos; el ítem desaparece.
- Inconvenientes:
  1. Contradice dos documentos aprobados (E031 §1/§4 y A-060 §55) y obliga a una enmienda de
     arquitectura, no solo de endpoint.
  2. Elimina la trazabilidad de una operación financiera: si un cliente borra por error, no hay
     forma de auditar ni de detectar que un agregado cambió.
  3. Una carrera `PATCH`/`DELETE` puede perder la versión y dejar un 404 sin evidencia.
  4. No hay idempotencia natural: el segundo `DELETE` ya es 404, pero no se distingue «nunca
     existió» de «ya se borró».
- Veredicto: **rechazada** para un dominio financiero y para el texto vigente de A-060.

### Opción B — Tombstone lógico + exclusión de agregados (recomendada)

- El ítem `USER#<id>/MOVEMENT#<occurredAt>#<id>` **permanece** con los hechos financieros y un
  marcador de borrado; el localizador `MOVEMENTPTR#<id>` se elimina en la misma transacción.
- Todos los lectores de movimientos excluyen los tombstones de los agregados y listados.
- Ventajas:
  1. Cumple E031 §1/§4 y A-060 §55.
  2. Conserva la auditoría mínima sin conservar texto libre.
  3. Permite condiciones CAS claras y una carrera `PATCH`/`DELETE` demostrable.
  4. Coherente con E042 y con el mecanismo de purga de E014 (que sigue siendo la vía de borrado
     real al eliminar la cuenta).
- Inconvenientes: crecen los ítems (coste de almacenamiento marginal) y **todo lector debe
  filtrar**; un lector que olvide el filtro infla los totales. Se mitiga con pruebas dedicadas
  (§6) y con una lista exhaustiva de lectores (§4).

### Opción C — Borrado físico diferido con `expiresAt` (TTL) como tombstone

- Consistiría en marcar y dejar que DynamoDB borre por TTL.
- Veredicto: **rechazada en esta decisión**. El TTL es *best-effort* (puede tardar días), no es
  determinista y A-060 §73 lo limita a ítems no críticos. Además, el contrato no expone un
  periodo de gracia y prometerlo sin verificarlo sería falso. Puede evaluarse como optimización
  de retención posterior con su propio Gate.

## 3. Decisión recomendada (opción B)

### 3.1 Campos del tombstone

Sobre el ítem principal `MOVEMENT#<occurredAt>#<id>`:

| Campo | Tipo DynamoDB | Valor | Obligatorio |
|---|---|---|---|
| `deleted` | `BOOL` | `true` | Sí |
| `deletedAt` | `S` | RFC3339 UTC con milisegundos (mismo formato que `updatedAt`) | Sí |
| `deletedBy` | `S` | `USER` (autoservicio del titular). Nunca se guarda `sessionId`, `jti`, token ni correo | Sí |
| `version` | `N` | `ADD version :one` (la versión del tombstone es `versión_anterior + 1`) | Sí |

Campos que **se conservan** (hechos financieros mínimos para auditoría y para demostrar la
exclusión): `PK`, `SK`, `entityType`, `userId`, `movementId`, `type`, `amountCents`, `currency`,
`categoryId`, `occurredAt`, `createdAt`, `updatedAt`, y los vínculos `contributionId` /
`recurringPaymentId` **solo si existen** (un movimiento con `contributionId` no se puede borrar:
409).

Campos que **se eliminan** en la misma escritura:

| Campo | Motivo |
|---|---|
| `description` | Texto libre del usuario; no es necesario para la auditoría financiera ni para excluir el ítem de los agregados. Minimización de datos |
| `GSI1PK`, `GSI1SK` | El ítem no debe aparecer en consultas por tipo/fecha del índice; un tombstone no se lista |

No se escribe `deleted=false` al crear: **la ausencia del atributo significa «no borrado»**
(compatibilidad con todos los movimientos existentes y con E001/E028 ya publicados).

### 3.2 Escritura atómica (variante vigente: `204` idempotente con puntero conservado)

Una sola `TransactWriteItems` con dos acciones:

1. `Update` sobre `PK=USER#<id>`, `SK=MOVEMENT#<occurredAt>#<id>`:
   - `UpdateExpression`: `SET #deleted = :true, #deletedAt = :now, #deletedBy = :actor, #updatedAt = :now ADD #version :one REMOVE #description, #gsi1pk, #gsi1sk` (DynamoDB exige `SET` → `REMOVE` → `ADD`)
   - `ConditionExpression`: `attribute_exists(PK) AND #version = :expected AND (attribute_not_exists(#deleted) OR #deleted = :false)`
   - Todos los `ExpressionAttributeValues` declarados se usan y ninguno sobra: DynamoDB rechaza con
     `ValidationException` cualquier valor no referenciado (defecto que ya rompió E003/E042).
2. `Update` sobre `PK=USER#<id>`, `SK=MOVEMENTPTR#<id>` con
   `SET #deleted = :true, #updatedAt = :now` y `ConditionExpression: attribute_exists(PK)`.
   **El puntero NO se elimina** y conserva `targetSK` (autorización
   `APRUEBO_E031_IDEMPOTENTE_204_CON_PUNTERO`).

Reglas:

- `:expected` es la versión leída con `ConsistentRead` inmediatamente antes. Si la condición
  falla, se relee (máximo 3 intentos) y se distingue: ítem ausente → `404`; versión distinta →
  reintento; **ya borrado → `204` idempotente** (sin escritura y sin subir la versión).
- Si sólo falla la acción del puntero (cancelación en el índice 1: ausente o incoherente), se
  reintenta **únicamente** el tombstone del principal. La autoridad es el ítem principal; nunca se
  revierte un tombstone ya aplicado.
- **No** se toca el ítem de aportación (`CONTRIBUTION#`) ni la meta. Un movimiento con
  `contributionId` no vacío responde **409** antes de escribir (comportamiento actual, correcto).
- El `DELETE` es **idempotente**: repetirlo sobre un movimiento ya borrado devuelve `204` sin
  cuerpo, sin escritura y con la versión intacta.
- La transacción es la única escritura: no hay ventana en la que el puntero exista sin ítem ni
  viceversa.

### 3.3 Por qué el puntero se CONSERVA

- La autorización `APRUEBO_E031_IDEMPOTENTE_204_CON_PUNTERO` exige que la repetición del `DELETE`
  pueda resolver el mismo recurso y confirmar el tombstone sin una consulta de partición.
- `LoadMovement` ya lee el puntero y devuelve el principal; el servicio excluye los tombstones
  (`Deleted`) traduciéndolos a `404`/`204`, de modo que E029 y E030 siguen sin revelar recursos
  borrados.
- La alternativa de la v1.0.0 (eliminar el puntero) obligaba a distinguir «nunca existió» de «ya
  se borró» por tiempos y hacía imposible un `204` idempotente sin `Query`.
- El tombstone no aparece en `GSI1` (se retiran `GSI1PK`/`GSI1SK`) y E020 no lo exporta (§4.7).

## 4. Impacto por endpoint

| Endpoint | Operación afectada | Cambio exigido | Riesgo si se omite |
|---|---|---|---|
| **E031** `DELETE /movements/{id}` | `Service.Delete` + `MovementsStore.DeleteMovement` | Tombstone (§3) en lugar de `Delete` físico; `204` idempotente con puntero conservado | Sigue incumpliendo E031 §1/§4 |
| **E027** `GET /movements` | `MovementsStore.ListMovements` | `FilterExpression`: `(attribute_not_exists(#deleted) OR #deleted = :false)`; el bucle de páginas ya existe para filtros | El listado mostraría movimientos borrados |
| **E029** `GET /movements/{id}` | `LoadMovement` + `Service.Get` | El puntero se conserva: el servicio convierte un tombstone en `404` homogéneo | Devolvería un recurso borrado |
| **E030** `PATCH /movements/{id}` | `Service.Patch` + `ReplaceMovement` | Condición con `(attribute_not_exists(#deleted) OR #deleted = :false)` en ambas rutas; si el ítem está borrado → `404` | Permitiría editar un tombstone o devolvería 409/503 confusos |
| **E026** `GET /financial-summary` | `FinancialSummaryStore.AggregateMovements` (`:66-111`) | Saltar ítems con `deleted=true` antes de sumar | `income`/`expenses`/`savings`/`net` inflados |
| **E050** `GET /budgets/usage` | `BudgetUsageStore.AggregateExpenses` (`:78-130`) | Saltar `deleted=true` en el recorrido | `usedAmount` mayor que el real |
| **E051** `GET /dashboard` | `DashboardStore.ListLatestMovements` (`:98-126`) + resumen de E026 | `FilterExpression` de borrado y acumulación hasta 10 elementos (el `Limit` se aplica antes del filtro) | `latestMovements` podría mostrar borrados o quedarse corto |
| **E048/E049** `GET/PUT /budgets` | `BUDGETSET#` | **Sin impacto**: no leen movimientos | — |
| **E020** `GET /users/me/export` | `ExportStore.Movimientos` (`:162-174`) → `ListMovements` | Excluir tombstones (hereda el filtro de E027). Documentar que la exportación no incluye lo borrado | Exportaría datos que el usuario borró |
| **E014** `DELETE /users/me` | Purga por partición (`deletion_store.go`) | Sin cambio: la purga borra físicamente todos los ítems, tombstones incluidos. Recontar `deletedCount` | Residuos tras la purga |
| **E028** `POST /movements` | Idempotencia | Sin cambio. Un replay de una creación posteriormente borrada devuelve la respuesta original (semántica de idempotencia); documentarlo | Cambiar la semántica de replay sin decisión |
| **E037** `POST /recurring-payments/{id}/payments` | Duplicado por fecha (`RECURRINGPAY#`) | **Sin impacto**: la deduplicación no depende del ítem de movimiento | — |
| **E052–E056** analítica | Lectores de `MOVEMENT#` | Excluir tombstones en cualquier agregado | Insights y patrones sobre datos borrados |
| **E044/E047** aportaciones | Movimiento vinculado | Sin cambio: `contributionId` ⇒ `409` en E031; la reversión no borra el movimiento | — |

## 5. Migración

| Aspecto | Resultado |
|---|---|
| Ítems existentes sin `deleted` | **No borrados** (la condición es `attribute_not_exists(#deleted) OR #deleted = :false`). No hay backfill |
| Ítems ya borrados físicamente por el runtime anterior | No se recuperan; no hay nada que migrar |
| Índices | No se crean ni modifican. `GSI1` deja de proyectar el tombstone al eliminarse `GSI1PK`/`GSI1SK` |
| Terraform | **Sin cambio**: mismos atributos base, sin nuevo índice ni tabla |
| Coste | Marginal: el ítem pasa de 0 a ~300-600 bytes; con los volúmenes previstos, dentro del margen de I-000. No se declara una cifra no medida |
| Reversibilidad | Volver al borrado físico exigiría una purga de tombstones con Gate propio; no se contempla en esta decisión |

## 6. Pruebas exigibles (antes de declarar E031 conforme)

1. `Delete` sobre un movimiento propio responde `204` sin cuerpo y deja el ítem con `deleted=true`, `deletedAt` RFC3339, `deletedBy=USER` y `version` incrementada.
2. El ítem conserva `amountCents`, `type`, `categoryId`, `occurredAt` y **no** conserva `description`, `GSI1PK` ni `GSI1SK`.
3. El localizador `MOVEMENTPTR#` **se conserva** con `deleted=true` y el mismo `targetSK`; no reaparece si estaba ausente.
4. Segundo `DELETE` del mismo movimiento → `204` idempotente, sin escritura ni versión nueva.
5. `DELETE` de un movimiento ajeno → `404` homogéneo (no 403).
6. `DELETE` de un movimiento con `contributionId` → `409` y **ninguna** escritura (verificar versión intacta).
7. `E027` no lista el tombstone, ni con filtros `type`/`from`/`to`, ni en la última página.
8. `E029` devuelve `404` tras el borrado.
9. `E030` devuelve `404` tras el borrado y no incrementa la versión del tombstone.
10. `E026` calcula `expenses`/`savings`/`net` sin el movimiento borrado (comparación exacta en centavos).
11. `E050` no cuenta el gasto borrado en `usedAmount`.
12. `E051` no incluye el tombstone en `latestMovements` y pagina hasta llenar el límite.
13. `E020` no exporta el tombstone (hereda el filtro de E027).
14. Carrera `PATCH` ∥ `DELETE` en 25 iteraciones: o gana el PATCH y el DELETE reintenta, o gana el DELETE y el PATCH responde `404`; **nunca** un ítem sin `deleted` con versión retrocedida ni un `500`.
15. Dos instancias lógicas sobre la misma tabla: el borrado en A es visible en B (lectura consistente).
16. Cero `Scan` en todas las rutas; el tombstone no aparece en `GSI1` (`GSI1PK`/`GSI1SK` retirados).
17. Purga de cuenta (E014) elimina también los tombstones y termina en `PURGED`.

## 7. Riesgos

| ID | Riesgo | Mitigación |
|---|---|---|
| R-31-01 | Un lector olvida filtrar y los agregados se inflan | Lista exhaustiva §4 + pruebas §6.10-13; revisión de código obligatoria |
| R-31-02 | Retención de datos que el usuario cree eliminados | Se elimina `description` y los índices; se conserva solo el hecho financiero mínimo; la eliminación real de la cuenta (E014) purga todo; el aviso de privacidad debe explicarlo |
| R-31-03 | Crecimiento de almacenamiento | Marginal; se medirá con las métricas de I-110 si se desea |
| R-31-04 | `deletedBy` pudiera almacenar PII | Se fija el literal `USER`; prohibido guardar sesión, token o correo |
| R-31-05 | Cambio de comportamiento en endpoints publicados | E027/E026/E050/E051/E020 cambian lecturas; exige Gate de runtime y regresión HTTPS |
| R-31-06 | El `FilterExpression` altera la paginación | E027 ya itera para llenar página; E051 debe acumular hasta 10. Pruebas con tombstones intercalados |

## 8. Autorizaciones concedidas

```
APRUEBO_DECISION_E031_BORRADO_LOGICO_TOMBSTONE
APRUEBO_E031_IDEMPOTENTE_204_CON_PUNTERO
APRUEBO_CIERRE_IMPLEMENTACION_E031_TOMBSTONE
```

Alcance concedido:

1. Implementar la opción B en `internal/application/movements` y `internal/adapters/dynamo`
   (tombstone y filtros de lectura), con **`204` idempotente y puntero conservado**.
2. Añadir los filtros de exclusión en E026, E050, E051 y E020; los lectores de analítica/ML se
   filtran cuando se integren.
3. Ejecutar las pruebas del §6 y la regresión de los endpoints afectados.
4. **No** autoriza por sí sola release, despliegue, cambio de OpenAPI ni modificación de
   `spec.md`/`plan.md`. El contrato HTTP no cambia (`204`, `404`, `409` ya existen) y el
   despliegue AWS exige su propio Gate.

## 9. Qué no hace este documento

- No cambia el contrato HTTP ni Terraform; la implementación vive en `internal/**` y `tests/**`.
- No cambia `spec.md`/`plan.md` de E031 ni de A-060 (política de edición).
- No declara E031 desplegado ni verificado en AWS: el estado local es `IMPLEMENTED_LOCAL`.
- No fija retención ni TTL de tombstones; si se desea, será una decisión posterior con Gate.

## 10. Trazabilidad

- `sdd/03-endpoints/E031-movements-movementId/{spec,plan,tasks,checklist}.md`
- `sdd/01-arquitectura-general/A-060-modelo-dynamodb/spec.md:55,73`
- `sdd/_control/H08/RFC-022-CONTRATOS_OLEADA_3.md` §7.3
- `sdd/_control/H08/MATRIZ_AUDITORIA_54_ENDPOINTS.md` (fila E031)
- `sdd/_control/H08/RECONCILIACION_CONTRACTUAL_E002_E008_E009_E051.md` (lectores de movimientos)
- Runtime: `internal/application/movements/service.go:485`, `internal/adapters/dynamo/movements_store.go:320,570,745`, `financial_summary_store.go:66`, `budget_usage_store.go:78`, `dashboard_store.go:98`, `export_store.go:162`, `goals_store.go:856` (precedente E042), `deletion_store.go` (purga E014)
