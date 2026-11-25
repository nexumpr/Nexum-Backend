# Residuos de E014 por familia — inventario y límites

**Fecha:** 2026-11-25 · **Rama:** `fix/h08e-oleada5-regularizacion` (PR #145) · **Estado:** inventario verificado por lectura de código; purga local probada con dobles estrictos; **AWS NOT_RUN**.

## 1. Qué borra la purga de E014

`DELETE /api/v1/users/me` marca el perfil `DELETION_PENDING`, incrementa `sessionEpoch`, crea el tombstone `DELETION#<requestId>` y encola el trabajo. El worker recorre por fases:

| Familia | Ubicación | Tratamiento |
|---|---|---|
| `PROFILE`, `SESSION#`, `REFRESH#`, `NOTIFICATION#`, `READALL#`, `READALLPTR#`, `REMINDER#`, `REMINDERPTR#`, `SCHEDULEPTR#`, `OUTBOXPTR#`, `DEVICE#`, `NOTIFICATION_PREFERENCES`, `IDEM#` (usuario), `IDEM#PUSH#`, `IDEM#REMINDER#`, `GOAL#`, `MOVEMENT#`, `MOVEMENTPTR#`, `RECURRING#`, `BUDGETSET#`, `CONSENT#`, `ONBOARDING`, `INCOME` | `USER#<sub>` | Barrido por `Query` de la partición con `LastSK` (reanudable), sin `Scan`. |
| `OUTBOX#GLOBAL/DUE#…#<id>` y `SCHEDULE#GLOBAL/DUE#…#<id>` | partición de servicio | Se localizan **solo** por el puntero `OUTBOXPTR#`/`SCHEDULEPTR#` del titular; se borra primero el global (condición de lease) y después el puntero. Un global con envío en curso deja el trabajo en `RETRY` hasta vencer el lease. |
| `EMAIL#<sha256>` (reserva de correo) | partición de servicio | Se borra con la clave derivada del correo del perfil, sin listar la tabla. |
| `QUEUE#PWRESET` | partición de servicio | Se localiza por `userId` en los ítems de la cola (Query por prefijo) y se retira. |
| `QUEUE#READALL/REQ#…` y `READALLPTR#` | servicio + titular | El puntero se borra con la partición; la cola la retira el cierre del trabajo o `FinishReadAll`. |

Tras `PURGED` se verifica la partición y las referencias conocidas; si queda algo, el estado es `RETRY`, nunca `PURGED` prematuro.

## 2. Residuos declarados (no enumerables sin `Scan` o con TTL)

| Familia | Contenido | Clasificación | Límite / mitigación |
|---|---|---|---|
| `OUTBOX#GLOBAL/IDEM#<sha256(userID\|eventId)>` | `userID`, `eventId`, `notificationID` | **Identificadores vinculables** (no contenido) | Ahora lleva `expiresAt` a 90 días (TTL best-effort de DynamoDB). Antes quedaba permanente. |
| `ONE_USE#<sha256(token)>` (E010/E011) | hash del token, `userId`, correo, propósito | Dato personal indirecto | TTL de 30 min; no enumerable por usuario sin `Scan`. |
| `RESET#<sha256(token)>` (E005/E006) | hash del token, `userId`, TTL | Dato personal indirecto | TTL de 30 min; el token en claro nunca se persiste. |
| Claves Redis de cuotas y agregados | HMAC del identificador (correlacionable) | Dato personal indirecto | TTL corto (30 s agregados; cuotas con su ventana). `DeletePrefix` requiere `SCAN`, denegado por ACL: se tolera `NOPERM` de `SCAN` y la clave expira por TTL. No se habilita `SCAN`. |
| Histórico sin puntero (si existiera de releases previos) | podría no ser enumerable | Riesgo declarado | No se usa `Scan` global. Estrategia propuesta: job de reconciliación acotado por ventana temporal y lotes, con Gate propio; no se ejecutó. |

**Regla:** un hash derivado de `userID` no se considera anónimo. Se trata como identificador vinculable y se elimina o caduca.

## 3. Pruebas ejecutadas (locales, dobles estrictos)

Purga de A sin afectar B; 0 residuos en `USER#`, punteros y globales; purga durante envío activo (lease vigente ⇒ `RETRY`); purga durante scheduler; lease vencido; worker reiniciado; dos nodos; operación tardía; puntero corrupto/ajeno (no borra datos de terceros); ausencia de recreación post-tombstone (productor/worker comprueban `PROFILE`); `PURGED` solo tras verificación durable.

## 4. Límites que no se ocultan

- Un push **ya entregado** a un dispositivo externo no puede recuperarse; se borra el dato del backend, no la copia en el terminal.
- El TTL de DynamoDB es best-effort: el ítem puede sobrevivir horas o días tras `expiresAt`.
- La verificación real en AWS (`PURGED` con una cuenta sintética) sigue `NOT_RUN`.
