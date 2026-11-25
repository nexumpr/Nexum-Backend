# RFC-024 — Ampliación aditiva de `UserExport` (E020) para las familias de Oleadas 4 y 5

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/RFC-024-EXPORT_E020_FAMILIAS_NUEVAS.md` |
| Versión | 1.0.0 |
| Fecha | 2026-11-24 |
| Estado | **APPROVED** por `APRUEBO_TRANSVERSAL_E014_E020` (Scrum Master, 2026-11-24) |
| Autoridad | Orden ejecutiva §10.2 (E020) y §2 (autorización transversal) |
| Instrumento | Enmienda aditiva de contrato; no elimina campos ni cambia la semántica existente |

## 1. Problema

`GET /api/v1/users/me/export` (E020) exporta perfil, movimientos, metas, consentimientos, ingreso, recurrentes, aportaciones, presupuestos, onboarding y preferencias. La auditoría del 2026-11-24 confirmó que **no exporta** las familias introducidas por Oleadas 4 y 5:

- preferencias de notificaciones (`NOTIFICATION_PREFERENCES`),
- dispositivos push (`DEVICE#`),
- notificaciones persistidas (`NOTIFICATION#`),
- recordatorios (`REMINDER#`).

La omisión produce una copia incompleta de los datos del titular.

## 2. Cambio aplicado (aditivo)

Se añaden cuatro propiedades **opcionales** a `UserExport`:

| Propiedad | Tipo | Contenido | Excluye |
|---|---|---|---|
| `notifications` | array de `Notification` | `id`, `title`, `message`, `isRead`, `createdAt` | `unreadCount`, tokens, PII de terceros |
| `reminders` | array de `ExportReminder` | `id`, `title`, `dueAt`, `timezone`, `frequency`, `active`, `createdAt`, `updatedAt` | `version` interna, `IDEM#` |
| `pushDevices` | array de `ExportPushDevice` | `id`, `provider`, `deviceLabel`, `registeredAt`, `active` | `token`, `tokenCipher`, `tokenFingerprint`, `revokedAt` |
| `notificationPreferences` | `NotificationPreferences` o `null` | los diez campos ya definidos por E057 | credenciales, secretos |

Se añaden dos schemas nuevos:

- `ExportPushDevice` (`additionalProperties: false`; requiere los cinco campos).
- `ExportReminder` (`additionalProperties: false`; requiere los siete campos; `frequency` admite `null`).

No se modifica `required` de `UserExport`: perfil, `exportedAt`, movimientos, metas y consentimientos siguen siendo obligatorios. Las cuatro propiedades nuevas son opcionales para no romper clientes existentes.

## 3. Privacidad y seguridad

- Nunca se exportan `passwordHash`, JWT, refresh tokens, `tokenCipher`, `tokenFingerprint`, credenciales, leases (`owner`, `leaseUntil`), claves `IDEM#`, `OUTBOX#` ni `SCHEDULE#`.
- `pushDevices.active` es un booleano de estado, no un token.
- `notifications` no incluye datos de otros usuarios: la consulta está particionada por `USER#<sub>`.
- El cursor existente se conserva; las colecciones nuevas se anexan al orden fijo de la página.

## 4. Compatibilidad

- Cambio **aditivo**: no se elimina ninguna propiedad, respuesta ni operación.
- `additionalProperties: false` se mantiene; las propiedades nuevas están declaradas.
- El comparador de I-150 debe devolver `COMPATIBLE`; se ejecutó sobre el contrato resultante.
- Android puede ignorar las propiedades nuevas; el cliente que las consuma debe actualizarse en una fase coordinada.

## 5. Verificación ejecutada

| Control | Resultado |
|---|---|
| `validar_h06.py` | `H06_CONTRACT_PASS` (75 operaciones, 300 specs, 33M+23S+12E+7O) |
| Paridad JSON/YAML | `PARIDAD_OK` (75 operaciones, 55 rutas, **104 esquemas**) |
| Comparador contra `main` | `COMPATIBLE`, 0 roturas |
| Pruebas Go de exportación | `TestE020Transversal*` PASS (A/B, paginación, sin secretos) |
| Pruebas de purga E014 | `TestE014*` PASS (punteros globales, lease, residuos) |

## 6. Aprobación

`APRUEBO_TRANSVERSAL_E014_E020` cubre expresamente “implementar purga/exportación de todas las familias nuevas, con RFC contractual cuando corresponda y sin operaciones destructivas en producción”. **No se ha inventado una firma nueva.**
