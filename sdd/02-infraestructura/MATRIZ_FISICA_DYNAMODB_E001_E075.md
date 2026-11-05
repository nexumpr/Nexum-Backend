# H05 — Diseño físico DynamoDB y acceso para las 75 rutas del catálogo V6.0.2

**Estado: DRAFT**. Este anexo define operaciones de persistencia y claves físicas candidatas para I-040; NO formaliza DTO, HTTP/OpenAPI ni crea archivos H06. Los 75 endpoints son **propuestas**, no API desplegada. Origen: catálogo 27 páginas elaborado sobre Java/WebView; proveedor histórico MongoDB/GCP sustituido por AWS+DynamoDB.

## Tabla principal y atributos

- `NEXUM_CORE_<env>`: `PK` (S), `SK` (S), `GSI1PK/GSI1SK` (opcionales sparse), `GSI2PK/GSI2SK` (opcionales sparse), `expiresAt` (N TTL). `PAY_PER_REQUEST` sujeto a costo/Free real en I-000. Encryption at rest, tags, snapshot/PITR según ADR y presupuestos.
- Items usuario: `PK=USER#uuid` y `SK=PROFILE|INCOME|ONBOARDING|PREFERENCES|CONSENT#<ver>|SESSION#<sid>|GOAL#<id>|RECURRING#<id>|BUDGET#<month>#<cat>|MOVEMENT#<UTC-sortable>#<id>|CONTRIBUTION#<id>|NOTIFICATION#<ts>#<id>|REMINDER#<id>|DEVICE#<id>`. Todos Get/Query restringidos al userId de JWT, no supplied `userId` de API.
- Reservas únicas: `PK=EMAIL#<sha256(normalized-lowercase-email)>; SK=OWNER` con `attribute_not_exists` en la misma transacción del registro; también uniqueness secundaria para tokens de sesión hashes que deben recuperarse sin `userId`, con `PK=SESSIONHASH#<tokenhash>; SK=LOOKUP`. Nunca indexar password/refresh en claro.
- `GSI1PK=USER#<id>#TYPE#<movType>`, `GSI1SK=<occurredAtUTC>#<movementId>` para lista por tipo y rango (alternativa `SK` directamente para ALL). `GSI2PK=DUE#<yyyy-mm-dd>#<shard>`, `GSI2SK=<nextDueAt>#<reminderId>` únicamente para workers programados confirmados; status/tipo de notificaciones se resuelven por Query usuario sin Scan o GSI sparse justificado.
- Registro dinero: `amountCents` int64, moneda `MXN` inicial, `version` para compare-and-swap, `occurredAt` instante UTC y zona horaria IANA del usuario. No usar float para saldo.
- Aportación dividida: transacción con idempotency item, contributions, movement(s), Update de GOAL(s), condiciones no-exceed/version. **Hasta 100 acciones, máximo 4 MB; no más de una acción al mismo item dentro de una transacción**; limitar cantidad de metas antes de aceptar petición. En conflictos rollback completo automático de la transacción.
- TTL expira eventualmente, no sustituye control funcional de expiración; borrado de cuenta, consentimientos y logs deben cumplir política de privacidad y respaldo.

## Matriz de operaciones por ruta (propuesta de persistencia, pendiente de ratificación contractual H06)

| ID | Método/ruta del catálogo | DynamoDB / índice / operación candidata |
|---|---|---|
| E001 | `POST /api/v1/auth/register` | TransactWriteItems CORE: EMAIL#hash uniqueness + USER#id PROFILE/INCOME/CONSENT; idempotencia por registro |
| E002 | `POST /api/v1/auth/login` | Get/Query identidad EMAIL#hash o SESSION#hash; Put/Update condicional de session y revocación |
| E003 | `POST /api/v1/auth/refresh` | Get/Query identidad EMAIL#hash o SESSION#hash; Put/Update condicional de session y revocación |
| E004 | `POST /api/v1/auth/logout` | Get/Query identidad EMAIL#hash o SESSION#hash; Put/Update condicional de session y revocación |
| E005 | `POST /api/v1/auth/password/forgot` | Put/Get/Update token de un solo uso con expiración y condición used=false; nunca Scan |
| E006 | `POST /api/v1/auth/password/reset` | Put/Get/Update token de un solo uso con expiración y condición used=false; nunca Scan |
| E007 | `POST /api/v1/auth/password/change` | Get/Query/Update USER#id/SESSION#id con ownership |
| E008 | `GET /api/v1/auth/sessions` | Get/Query/Update USER#id/SESSION#id con ownership |
| E009 | `DELETE /api/v1/auth/sessions/{sessionId}` | Get/Query/Update USER#id/SESSION#id con ownership |
| E010 | `POST /api/v1/auth/email-verification/request` | Put/Get/Update token de un solo uso con expiración y condición used=false; nunca Scan |
| E011 | `POST /api/v1/auth/email-verification/confirm` | Put/Get/Update token de un solo uso con expiración y condición used=false; nunca Scan |
| E012 | `GET /api/v1/users/me` | Get/Update/TransactWriteItems USER#id/PROFILE, CONSENT#version, ONBOARDING, PREFERENCES; DELETE cuenta en flujo auditable |
| E013 | `PATCH /api/v1/users/me` | Get/Update/TransactWriteItems USER#id/PROFILE, CONSENT#version, ONBOARDING, PREFERENCES; DELETE cuenta en flujo auditable |
| E014 | `DELETE /api/v1/users/me` | Get/Update/TransactWriteItems USER#id/PROFILE, CONSENT#version, ONBOARDING, PREFERENCES; DELETE cuenta en flujo auditable |
| E015 | `GET /api/v1/privacy/notices/current` | Get/Update/TransactWriteItems USER#id/PROFILE, CONSENT#version, ONBOARDING, PREFERENCES; DELETE cuenta en flujo auditable |
| E016 | `GET /api/v1/users/me/consents` | Get/Update/TransactWriteItems USER#id/PROFILE, CONSENT#version, ONBOARDING, PREFERENCES; DELETE cuenta en flujo auditable |
| E017 | `POST /api/v1/users/me/consents` | Get/Update/TransactWriteItems USER#id/PROFILE, CONSENT#version, ONBOARDING, PREFERENCES; DELETE cuenta en flujo auditable |
| E018 | `GET /api/v1/users/me/onboarding` | Get/Update/TransactWriteItems USER#id/PROFILE, CONSENT#version, ONBOARDING, PREFERENCES; DELETE cuenta en flujo auditable |
| E019 | `PATCH /api/v1/users/me/onboarding` | Get/Update/TransactWriteItems USER#id/PROFILE, CONSENT#version, ONBOARDING, PREFERENCES; DELETE cuenta en flujo auditable |
| E020 | `GET /api/v1/users/me/export` | Get/Update/TransactWriteItems USER#id/PROFILE, CONSENT#version, ONBOARDING, PREFERENCES; DELETE cuenta en flujo auditable |
| E021 | `GET /api/v1/bootstrap` | Get catalog #global y configuración pública con cache Redis (E021 agregador Query por PK del usuario) |
| E022 | `GET /api/v1/catalogs/categories` | Get catalog #global y configuración pública con cache Redis (E021 agregador Query por PK del usuario) |
| E023 | `GET /api/v1/catalogs/frequencies` | Get catalog #global y configuración pública con cache Redis (E021 agregador Query por PK del usuario) |
| E024 | `GET /api/v1/income` | Get/Update USER#id/INCOME, Query MOVEMENT#mes y REC#id + GOAL#id; sumar centavos sin Scan |
| E025 | `PUT /api/v1/income` | Get/Update USER#id/INCOME, Query MOVEMENT#mes y REC#id + GOAL#id; sumar centavos sin Scan |
| E026 | `GET /api/v1/financial-summary` | Get/Update USER#id/INCOME, Query MOVEMENT#mes y REC#id + GOAL#id; sumar centavos sin Scan |
| E027 | `GET /api/v1/movements` | Query USER#id MOVEMENT#UTC#id; GSI1 por tipo con rango; Get/Update/DELETE condicional y proyecciones versionadas |
| E028 | `POST /api/v1/movements` | Query USER#id MOVEMENT#UTC#id; GSI1 por tipo con rango; Get/Update/DELETE condicional y proyecciones versionadas |
| E029 | `GET /api/v1/movements/{movementId}` | Query USER#id MOVEMENT#UTC#id; GSI1 por tipo con rango; Get/Update/DELETE condicional y proyecciones versionadas |
| E030 | `PATCH /api/v1/movements/{movementId}` | Query USER#id MOVEMENT#UTC#id; GSI1 por tipo con rango; Get/Update/DELETE condicional y proyecciones versionadas |
| E031 | `DELETE /api/v1/movements/{movementId}` | Query USER#id MOVEMENT#UTC#id; GSI1 por tipo con rango; Get/Update/DELETE condicional y proyecciones versionadas |
| E032 | `GET /api/v1/recurring-payments` | Query/Update USER#id/RECURRING#id; pago real TransactWriteItems movimiento + idempotencia + vínculo recurrente |
| E033 | `POST /api/v1/recurring-payments` | Query/Update USER#id/RECURRING#id; pago real TransactWriteItems movimiento + idempotencia + vínculo recurrente |
| E034 | `GET /api/v1/recurring-payments/{recurringPaymentId}` | Query/Update USER#id/RECURRING#id; pago real TransactWriteItems movimiento + idempotencia + vínculo recurrente |
| E035 | `PATCH /api/v1/recurring-payments/{recurringPaymentId}` | Query/Update USER#id/RECURRING#id; pago real TransactWriteItems movimiento + idempotencia + vínculo recurrente |
| E036 | `DELETE /api/v1/recurring-payments/{recurringPaymentId}` | Query/Update USER#id/RECURRING#id; pago real TransactWriteItems movimiento + idempotencia + vínculo recurrente |
| E037 | `POST /api/v1/recurring-payments/{recurringPaymentId}/payments` | Query/Update USER#id/RECURRING#id; pago real TransactWriteItems movimiento + idempotencia + vínculo recurrente |
| E038 | `GET /api/v1/goals` | Query/Get/TransactWriteItems USER#id/GOAL#id y estado/counters; condición de versión y objetivo |
| E039 | `POST /api/v1/goals` | Query/Get/TransactWriteItems USER#id/GOAL#id y estado/counters; condición de versión y objetivo |
| E040 | `GET /api/v1/goals/{goalId}` | Query/Get/TransactWriteItems USER#id/GOAL#id y estado/counters; condición de versión y objetivo |
| E041 | `PATCH /api/v1/goals/{goalId}` | Query/Get/TransactWriteItems USER#id/GOAL#id y estado/counters; condición de versión y objetivo |
| E042 | `DELETE /api/v1/goals/{goalId}` | Query/Get/TransactWriteItems USER#id/GOAL#id y estado/counters; condición de versión y objetivo |
| E043 | `POST /api/v1/goal-contributions/preview` | TransactWriteItems USER#id/CONTRIBUTION#id + GOAL#id + MOVEMENT#id + idem; límite 100 actions/4MB |
| E044 | `POST /api/v1/goal-contributions` | TransactWriteItems USER#id/CONTRIBUTION#id + GOAL#id + MOVEMENT#id + idem; límite 100 actions/4MB |
| E045 | `GET /api/v1/goal-contributions` | TransactWriteItems USER#id/CONTRIBUTION#id + GOAL#id + MOVEMENT#id + idem; límite 100 actions/4MB |
| E046 | `GET /api/v1/goal-contributions/{contributionId}` | TransactWriteItems USER#id/CONTRIBUTION#id + GOAL#id + MOVEMENT#id + idem; límite 100 actions/4MB |
| E047 | `POST /api/v1/goal-contributions/{contributionId}/reverse` | TransactWriteItems USER#id/CONTRIBUTION#id + GOAL#id + MOVEMENT#id + idem; límite 100 actions/4MB |
| E048 | `GET /api/v1/budgets` | Query USER#id/BUDGET#month#category y MOVEMENT#period; update atómico limits |
| E049 | `PUT /api/v1/budgets` | Query USER#id/BUDGET#month#category y MOVEMENT#period; update atómico limits |
| E050 | `GET /api/v1/budgets/usage` | Query USER#id/BUDGET#month#category y MOVEMENT#period; update atómico limits |
| E051 | `GET /api/v1/dashboard` | Query rango de MOVEMENT + GOAL/CONTRIBUTION del usuario; agregar en Go; Redis cache versionada |
| E052 | `GET /api/v1/analytics/insights` | Query rango de MOVEMENT + GOAL/CONTRIBUTION del usuario; agregar en Go; Redis cache versionada |
| E053 | `GET /api/v1/analytics/categories` | Query rango de MOVEMENT + GOAL/CONTRIBUTION del usuario; agregar en Go; Redis cache versionada |
| E054 | `GET /api/v1/analytics/spending-patterns` | Query rango de MOVEMENT + GOAL/CONTRIBUTION del usuario; agregar en Go; Redis cache versionada |
| E055 | `GET /api/v1/analytics/recommendations` | Query rango de MOVEMENT + GOAL/CONTRIBUTION del usuario; agregar en Go; Redis cache versionada |
| E056 | `GET /api/v1/analytics/savings-streak` | Query rango de MOVEMENT + GOAL/CONTRIBUTION del usuario; agregar en Go; Redis cache versionada |
| E057 | `GET /api/v1/notification-preferences` | Get/Query/Update NOTIFICATION/REMINDER/DEVICE por usuario; GSI2 sparse por dueBucket#date para scheduler si aprobado |
| E058 | `PUT /api/v1/notification-preferences` | Get/Query/Update NOTIFICATION/REMINDER/DEVICE por usuario; GSI2 sparse por dueBucket#date para scheduler si aprobado |
| E059 | `POST /api/v1/devices/push-tokens` | Get/Query/Update NOTIFICATION/REMINDER/DEVICE por usuario; GSI2 sparse por dueBucket#date para scheduler si aprobado |
| E060 | `DELETE /api/v1/devices/push-tokens/{deviceRegistrationId}` | Get/Query/Update NOTIFICATION/REMINDER/DEVICE por usuario; GSI2 sparse por dueBucket#date para scheduler si aprobado |
| E061 | `GET /api/v1/notifications` | Get/Query/Update NOTIFICATION/REMINDER/DEVICE por usuario; GSI2 sparse por dueBucket#date para scheduler si aprobado |
| E062 | `PATCH /api/v1/notifications/{notificationId}/read` | Get/Query/Update NOTIFICATION/REMINDER/DEVICE por usuario; GSI2 sparse por dueBucket#date para scheduler si aprobado |
| E063 | `POST /api/v1/notifications/read-all` | Get/Query/Update NOTIFICATION/REMINDER/DEVICE por usuario; GSI2 sparse por dueBucket#date para scheduler si aprobado |
| E064 | `GET /api/v1/reminders` | Get/Query/Update NOTIFICATION/REMINDER/DEVICE por usuario; GSI2 sparse por dueBucket#date para scheduler si aprobado |
| E065 | `POST /api/v1/reminders` | Get/Query/Update NOTIFICATION/REMINDER/DEVICE por usuario; GSI2 sparse por dueBucket#date para scheduler si aprobado |
| E066 | `PATCH /api/v1/reminders/{reminderId}` | Get/Query/Update NOTIFICATION/REMINDER/DEVICE por usuario; GSI2 sparse por dueBucket#date para scheduler si aprobado |
| E067 | `DELETE /api/v1/reminders/{reminderId}` | Get/Query/Update NOTIFICATION/REMINDER/DEVICE por usuario; GSI2 sparse por dueBucket#date para scheduler si aprobado |
| E068 | `GET /api/v1/users/me/preferences` | Get/Update USER#id/PREFERENCES; si decisión en H06 mantiene dato local, sin persistencia remota |
| E069 | `PATCH /api/v1/users/me/preferences` | Get/Update USER#id/PREFERENCES; si decisión en H06 mantiene dato local, sin persistencia remota |
| E070 | `GET /api/v1/help/faq` | Get PUBLIC#FAQ/TIP (o recursos estáticos versionados) y Redis; no datos financieros privados |
| E071 | `GET /api/v1/education/tips` | Get PUBLIC#FAQ/TIP (o recursos estáticos versionados) y Redis; no datos financieros privados |
| E072 | `GET /api/v1/health/live` | No query/Scan Dynamo; health de proceso y ping acotado de dependencia crítica para readiness |
| E073 | `GET /api/v1/health/ready` | No query/Scan Dynamo; health de proceso y ping acotado de dependencia crítica para readiness |
| E074 | `GET /api/v1/demo/bootstrap` | Demo local opcional; si se implementa, JSON estático sintético sin cuenta real ni Dynamo |
| E075 | `POST /api/v1/voice/parse` | Parseo texto local opcional: sin persistencia; requiere confirmación antes de POST /movements |

## Validación y bloqueo de I-040

1. Construir en H08 los recursos Terraform para tabla PK/SK, GSI1, GSI2 únicamente si su uso se ratifica, TTL y backup. Mantener hash de IaC por release.
2. Ejercitar casos registro email único, sesiones revocables, idempotencia bajo doble reintento, 2 nodos, meta dividida, reversión y borrado total sin datos ajenos.
3. Medir `ConsumedCapacity`, tamaño de items y throttles; sin DynamoDB `Scan` en endpoints del usuario.
4. **Gate I-GO bloqueado** si alguna operación del catálogo requiere `Scan`, índice no definido, patrón de tiempo no estable o componente que la cuenta Free/SCP no pueda desplegar. Si H06 descubre cambio, RFC a I-040 antes de H08.
