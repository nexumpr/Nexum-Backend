# Diseño de E014 — eliminación real de cuenta con purga durable

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/DISENO_E014_PURGA.md` |
| Versión | 1.1.0 (corrige recuperación con fencing, cola y retención) |
| Fecha | 2026-11-18 |
| Estado | **DISEÑO — no implementado** |
| Contrato | `DELETE /api/v1/users/me` → `202 DeletionReceipt` |
| Autorización | Gate `GATE_H08E_OLEADA_2.md` §12 frase 1 |

## 1. Objetivo verificable

`202 scheduled` es válido **solo** si existe un proceso real, persistente y recuperable capaz de
completar la supresión con la infraestructura ya desplegada: DynamoDB `NEXUM_CORE_H08I`, Redis, el
binario Go existente y los permisos IAM vigentes. No se crea ningún servicio, tabla, índice,
cola externa ni permiso nuevo. No se usa `Scan`.

## 2. Flujo de la petición

1. Middleware `AuthenticateBearerWithSession` → `sub`, `sid`, época.
2. Binding estricto de `DeleteAccountRequest` (`additionalProperties: false`):
   `confirmation = DELETE_MY_ACCOUNT`; `currentPassword` no vacía.
3. Lectura consistente de `USER#<id>/PROFILE`. Ausente o `status != ACTIVE` → 401/404 sin
   revelar estado de otras cuentas.
4. Verificación Argon2id. Contraseña incorrecta → **422** `BUSINESS_RULE_VIOLATION` (criterio de
   E007), nunca 401 por contraseña.
5. `TransactWriteItems` de **3 ítems**:

| Ítem | Acción | Condición |
|---|---|---|
| `USER#<id>/PROFILE` | `SET status=DELETION_PENDING, deletedAt=:now, sessionEpoch=if_not_exists(sessionEpoch,:zero)+:one, version=if_not_exists(version,:zero)+:one` | `status = :active` |
| `USER#<id>/DELETION#<requestId>` | `Put` con `state=PENDING`, `phase=COLLECT`, `fence=0`, `owner=""`, `attempts=0`, `dueAt=:now`, `emailHash`, `catalogVersion`, `deletedCount=0` | `attribute_not_exists(PK)` |
| `QUEUE#DELETION/<requestId>` | `Put` con `state=PENDING`, `dueAt=:now`, `userId` | `attribute_not_exists(PK)` |

6. Respuesta **202**: `requestId`, `status="scheduled"`, `requestedAt`, `sessionsRevoked=true`,
   `deletionRequestId`.

**`sessionsRevoked=true` es verificable:** `AuthorizeSession`
(`internal/adapters/dynamo/logout_store.go:201`) exige `PROFILE.status = ACTIVE` con
`ConsistentRead` y `sessionEpoch` igual al claim `epo`. El cambio de estado y la elevación de
época invalidan todo access token y todo refresh en **ambos** nodos al confirmarse la
transacción. No se borran las sesiones en ese instante: se **revocan**.

**Idempotencia:** una segunda llamada autenticada recibe **401** porque la cuenta ya no está
`ACTIVE`. No se emite un segundo 202 ni se crea otro trabajo.

## 3. Descubrimiento sin `Scan` y actualización de `dueAt`

- Cola: partición fija `QUEUE#DELETION` (prefijo `QUEUE#*` ya autorizado).
- **`SK = <requestId>`** (estable). `dueAt`, `state`, `fence` y `userId` son atributos.
  Se elige `SK` estable precisamente para que un reintento pueda **cambiar `dueAt` con un
  `UpdateItem`**, sin borrar y recrear la referencia (en DynamoDB la clave no es mutable).
- Descubrimiento del worker: `Query` sobre `PK = QUEUE#DELETION` con
  `FilterExpression: state IN (:pending,:retry,:slow) AND dueAt <= :now`, `Limit` 50,
  `ExclusiveStartKey`. Es una consulta de partición, **no** un `Scan`; la cardinalidad es una
  entrada por cuenta eliminada, no por usuario activo.
- El ítem `USER#<id>/DELETION#<requestId>` es la **fuente de verdad** del progreso. La cola es
  solo un índice de vencimiento y puede repararse a partir del control.

## 4. Máquina de estados y fencing

### 4.1 Estados

| Estado | Significado | Reclamable |
|---|---|---|
| `PENDING` | Recién creado | Sí, si `dueAt <= now` |
| `RUNNING` | Un nodo lo posee con `owner` + `fence` + `leaseUntil` | Sí, **si `leaseUntil < now`** |
| `RETRY` | Fallo transitorio; `nextDueAt` con backoff | Sí, si `dueAt <= now` |
| `SLOW_RETRY` | Reintentos agotados; se reprograma cada 24 h | Sí, si `dueAt <= now` |
| `PURGED` | Verificación superada; tombstone | No |

`RETRY` y `SLOW_RETRY` **no** son terminales: un trabajo nunca queda abandonado. Tras 8
intentos se pasa a `SLOW_RETRY` con `dueAt = now + 24 h`; el contador sigue creciendo y el
trabajo se reintenta indefinidamente hasta completarse.

### 4.2 Reclamación atómica (incluye lease vencido)

Un único `UpdateItem` sobre el control:

```text
ConditionExpression:
  (#s = :pending AND #due <= :now)
  OR (#s = :running AND #lease < :now)
  OR (#s = :retry   AND #due <= :now)
  OR (#s = :slow    AND #due <= :now)

UpdateExpression:
  SET #s = :running, #owner = :me, #lease = :until, #attempts = if_not_exists(#attempts,:zero) + :one
  ADD #fence :one
ReturnValues: ALL_NEW
```

- `fence` se incrementa en **cada** reclamación. El nodo guarda el valor devuelto.
- Un trabajo `RUNNING` con `leaseUntil` vencido se recupera por esta misma condición: no hace
  falta un barrido aparte ni un cambio de estado previo.
- `leaseUntil = now + 60 s`, renovado por latido tras cada lote.

### 4.3 Fencing: el propietario anterior no puede completar ni sobrescribir

Toda escritura de progreso, latido, transición a `RETRY` y transición a `PURGED` lleva:

```text
ConditionExpression: #owner = :me AND #fence = :myFence
```

Si otro nodo reclamó el trabajo (`fence` distinto), el propietario anterior recibe
`ConditionalCheckFailed` y **aborta sin escribir**. No puede marcar `PURGED`, ni pisar el cursor,
ni reprogramar `dueAt`. Es un fencing token verificable por DynamoDB, no una convención.

### 4.4 Transiciones

```text
PENDING --claim--> RUNNING
RUNNING --latido (owner+fence)--> RUNNING            (leaseUntil renovado)
RUNNING --lote ok (owner+fence)--> RUNNING           (lastSK, deletedCount)
RUNNING --fallo transitorio (owner+fence)--> RETRY   (dueAt=now+backoff, owner="", lease=0)
RETRY|SLOW_RETRY --claim--> RUNNING                  (fence++)
RUNNING --lease vencido--> reclamable por otro nodo  (fence++)
RUNNING --8 intentos--> SLOW_RETRY                   (dueAt=now+24h)
RUNNING --verificación OK (owner+fence)--> PURGED     (+ borrado de la entrada de cola)
```

## 5. Fases de la purga

El control guarda `phase`, `lastSK`, `refreshDigests`, `emailHash`, `deletedCount`,
`unknownKeys` y `catalogVersion`. Cada lote persiste el avance antes de continuar: una
interrupción en cualquier punto se reanuda desde `phase`/`lastSK`.

| Fase | Acción | Por qué en este orden |
|---|---|---|
| `COLLECT` | Recorrer `USER#<id>` y recopilar los `refreshHash` de cada `SESSION#<sid>`; contar ítems | Se necesita la información de localización **antes** de borrar nada |
| `EXTERNAL` | Borrar `REFRESH#<digest>`, `EMAIL#<emailHash>` y las entradas de `QUEUE#PWRESET` con `userId` del titular | Referencias fuera de la partición que contienen datos personales (`destino`) |
| `PARTITION` | Recorrer `USER#<id>` y borrar ítem por ítem **excepto** `DELETION#<requestId>` | El control debe sobrevivir al recorrido |
| `VERIFY` | Releer partición (debe quedar solo el control), comprobar los `REFRESH#` recopilados y `EMAIL#`; consultar `QUEUE#PWRESET` filtrado | `PURGED` solo tras comprobación |
| `TERMINAL` | Marcar `PURGED`, limpiar PII del control, borrar la entrada de cola e invalidar caché | Cierre atómico y verificable |

Detalles vinculantes:

- **Protección del control:** el recorrido de `PARTITION` excluye explícitamente cualquier `SK`
  con prefijo `DELETION#`. Una clave desconocida se cuenta en `unknownKeys` y se borra, **salvo**
  ese prefijo. Nunca se puede borrar el registro de progreso por accidente.
- **Cursor recuperable:** `lastSK` se actualiza tras cada lote con condición de `owner`+`fence`;
  al reanudar se continúa con `ExclusiveStartKey = lastSK`.
- **Caché:** `UserCachePrefix(env, userID)` + `DeletePrefix` en Redis, después de `VERIFY`.
- **`PURGED` solo con verificación superada:** si queda un solo ítem distinto del control, o un
  `REFRESH#`/`EMAIL#`, o una entrada de cola de recuperación con datos del titular, el trabajo
  vuelve a `RETRY`. Nunca se declara `PURGED` con datos personales localizables pendientes.

## 6. Catálogo de purga (ampliable)

| Grupo | Claves | Fase |
|---|---|---|
| Perfil, preferencias, onboarding, ingreso | `USER#<id>` / `PROFILE`, `PREFERENCES`, `ONBOARDING`, `INCOME` | `PARTITION` |
| Consentimientos | `USER#<id>` / `CONSENT#*` | `PARTITION` |
| Movimientos | `USER#<id>` / `MOVEMENT#*` | `PARTITION` |
| Metas | `USER#<id>` / `GOAL#*` | `PARTITION` |
| Recurrentes | `USER#<id>` / `RECURRING#*` | `PARTITION` |
| Presupuestos (futuro) | `USER#<id>` / `BUDGET#*` | `PARTITION` |
| Idempotencia nueva | `USER#<id>` / `IDEM#*` | `PARTITION` |
| Sesiones | `USER#<id>` / `SESSION#<sid>` | `COLLECT` + `PARTITION` |
| Localizadores de refresh | `REFRESH#<digest>` | `EXTERNAL` |
| Reserva de correo | `EMAIL#<sha256(email)>` | `EXTERNAL` |
| Cola de recuperación de contraseña | `QUEUE#PWRESET` filtrado por `userId` | `EXTERNAL` |
| Caché | `UserCachePrefix` | `TERMINAL` |
| Control y cola | `DELETION#`, `QUEUE#DELETION/<requestId>` | `TERMINAL` |

`catalogVersion` permite que las Oleadas 3–5 añadan entidades sin reescribir el worker.

## 7. Retención: vencimiento lógico ≠ eliminación física

| Concepto | Definición en este diseño |
|---|---|
| **Vencimiento lógico** | El registro deja de ser válido para su finalidad en el instante calculado (`expiresAt`/`dueAt`). Lo garantiza la aplicación. |
| **Invalidación de acceso** | La cuenta y sus tokens dejan de autorizar en el acto (estado `DELETION_PENDING` + época). Verificable por HTTP 401 en ambos nodos. |
| **Eliminación física confirmada** | El proceso ejecutó `DeleteItem` y la lectura posterior devuelve el ítem ausente. Solo esto se cuenta en `deletedCount` y solo esto habilita `PURGED`. |
| **Eliminación asíncrona del proveedor** | DynamoDB TTL elimina ítems vencidos de forma asíncrona, típicamente en horas o días. **No se promete plazo.** |
| **Borrado pendiente** | Estado observable del trabajo: `PENDING`, `RETRY`, `SLOW_RETRY`. La API no lo presenta como completado. |
| **Retención documentada** | Tombstone mínimo tras `PURGED`, con política explícita (abajo). |

**Residuos no localizables.** `RESET#<sha256(token)>` y `ONE_USE#<sha256(token)>` se indexan por
el hash del token, que el servidor no conserva: no son enumerables por usuario sin un índice
nuevo (prohibido). Lo mismo ocurre con `IDEM#<sha256(operación:clave)>` de E001 (24 h). Contienen
`userId` y, en el flujo de recuperación, el destinatario. Su tratamiento declarado:

- vencimiento lógico a los 30 minutos;
- quedan **inutilizables** porque los flujos que los consumen exigen `PROFILE.status = ACTIVE`
  con lectura consistente (`email_verification_confirm.go:172`) y la cuenta ya no existe;
- eliminación física asíncrona por TTL, **sin plazo prometido**;
- no se declaran borrados.

Los endpoints de la Oleada 2 no crean datos de este tipo: usan idempotencia con partición
`USER#<id>` (§ `GATE_H08E_OLEADA_2.md` §5.2), localizable por el catálogo. Convertir los flujos
heredados a referencias localizables requiere RFC propia y no forma parte de esta oleada.

## 8. Tombstone final

Tras `PURGED` el control conserva solo: `state=PURGED`, `requestId`, `completedAt`,
`deletedCount`, `catalogVersion`, `version`. Se eliminan `emailHash`, `refreshDigests`, `lastSK`,
`owner`, `unknownKeys` y cualquier otro valor personal.

- Contiene el `userId` implícito en su `PK` (`USER#<id>`), necesario para permanecer en la
  partición (no hay prefijo IAM alternativo). Es el único identificador retenido.
- Retención: **90 días**, con `expiresAt` para vencimiento lógico.
- La eliminación física de ese tombstone la realiza el TTL del servicio de forma asíncrona; no se
  promete una fecha exacta.
- No se conserva nombre, correo, hash de contraseña ni dato financiero alguno.

## 9. Pruebas exigidas

1. Petición válida → 202 con cinco campos y control `PENDING` persistido.
2. `confirmation` inválida, campo desconocido, JSON malformado → 400/422.
3. Contraseña incorrecta → 422; cuenta ajena → 404; sin token → 401.
4. Tras el 202: login 401, refresh 401 y ruta protegida 401, en el mismo nodo y en un segundo
   nodo lógico sobre el mismo almacén.
5. **Fallo de instancia:** trabajo `RUNNING` con `leaseUntil` vencido → otro nodo lo reclama con
   `fence` incrementado y continúa desde `phase`/`lastSK`.
6. **Propietario anterior:** tras la reclamación, el antiguo dueño intenta latido, avance y
   `PURGED` → `ConditionalCheckFailed` en las tres; no escribe nada.
7. **Dos workers concurrentes:** solo uno obtiene la reclamación; el otro no duplica borrados ni
   efectos.
8. **Reinicio:** se simula caída a mitad de `PARTITION`; al reanudar no se pierde ningún registro
   ni se repite un efecto observable.
9. **Purga parcial:** se inyecta fallo en `EXTERNAL`; el trabajo queda `RETRY`, no `PURGED`, y al
   reintentar completa.
10. **Protección del control:** el recorrido de `PARTITION` no borra `DELETION#<requestId>`;
    una clave desconocida se cuenta y se borra sin afectar al control.
11. **Verificación:** si queda un ítem o un `REFRESH#`/`EMAIL#`, el estado vuelve a `RETRY`.
12. **Reintentos agotados:** 8 fallos → `SLOW_RETRY` con `dueAt=+24 h`; un noveno intento lo
    vuelve a tomar (no queda abandonado).
13. **`dueAt`:** reprogramar un reintento actualiza el atributo de la misma entrada de cola
    (`SK=requestId`) sin crear duplicados.
14. **Ausencia de residuos no autorizados:** al terminar, la partición contiene solo el
    tombstone permitido y ninguna clave con datos financieros o de contacto.
15. Prohibido usar cuentas reales. Solo cuentas sintéticas creadas para el ensayo.

## 10. Dependencias y Gates separados

| Dependencia | Estado | Gate |
|---|---|---|
| `GetItem`/`PutItem`/`UpdateItem`/`DeleteItem`/`Query` sobre `USER#*`, `EMAIL#*`, `REFRESH#*`, `QUEUE#*` | Ya autorizado | — |
| `QUEUE#DELETION` como nueva partición | Dentro de `QUEUE#*` | Solo Gate de la oleada |
| `BatchWriteItem` | **No autorizado** | No se usa |
| Índice para tokens no localizables | Requiere GSI nuevo (prohibido) | RFC + Gate propios; fuera de alcance |
| Texto de E015 | `DRAFT` | Gate con hash del texto nuevo |
| Purga real en AWS | Requiere cuenta sintética | Gate de verificación AWS |

## 11. Riesgos

| Riesgo | Tratamiento |
|---|---|
| Purga lenta en cuentas grandes | Lotes, cursor persistido, reanudable |
| Worker sin liderazgo Redis | Trabajo queda `PENDING`/`RETRY`; el 202 no promete inmediatez |
| TTL asíncrono | Se declara vencimiento lógico, no plazo físico |
| Clave nueva de una oleada futura | `unknownKeys` + barrido por partición (con protección de `DELETION#`) |
| Borrado accidental de cuenta real | Solo cuentas sintéticas |
