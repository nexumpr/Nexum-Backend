# Matriz de reconciliación preparatoria E005–E075

> **CANDIDATO / NO APROBADO.** No altera el SDD, la API productiva ni el contrato OpenAPI. El PDF es una auditoría de prototipo y declara expresamente no haber contrastado SDD vigente. Los campos candidatos pueden ser opcionales, derivados o futuros; requieren decisión humana por RFC antes de modificar specs aprobadas.

Base: ZIP main post-PR #64 y Auditoría V6.0.2 de 126 páginas. Se cotejaron 71 operaciones y sus spec.md; el método y la ruta coinciden en las 71.

| ID | Ruta / operación (SDD y auditoría) | HTTP SDD → PDF | Entrada candidata nueva | Salida candidata nueva | Observación |
|---|---|---|---|---|---|
| E005 | `POST /api/v1/auth/password/forgot` | 202 → 202 | `locale` | `accepted`, `message`, `retryAfterSeconds` | entrada candidata; salida candidata; persistencia obsoleta |
| E006 | `POST /api/v1/auth/password/reset` | 200 → 200 | `resetToken` | `reset`, `revokedSessions` | entrada candidata; salida candidata; persistencia obsoleta |
| E007 | `POST /api/v1/auth/password/change` | 200 → 200 | — | `changedAt`, `revokedOtherSessions` | salida candidata; persistencia obsoleta |
| E008 | `GET /api/v1/auth/sessions` | 200 → 200 | `limit`, `cursor` | `sessions[]`, `sessions[].id`, `sessions[].deviceName`, `sessions[].platform`, `sessions[].createdAt`, `sessions[].lastSeenAt`… | entrada candidata; salida candidata; persistencia obsoleta |
| E009 | `DELETE /api/v1/auth/sessions/{sessionId}` | 204 → 204 | `sessionId` | — | entrada candidata; persistencia obsoleta |
| E010 | `POST /api/v1/auth/email-verification/request` | 202 → 202 | `email`, `locale` | `accepted`, `retryAfterSeconds` | entrada candidata; salida candidata; persistencia obsoleta |
| E011 | `POST /api/v1/auth/email-verification/confirm` | 200 → 200 | `verificationToken` | `emailVerified`, `verifiedAt` | entrada candidata; salida candidata; persistencia obsoleta |
| E012 | `GET /api/v1/users/me` | 200 → 200 | — | `name`, `updatedAt`, `accountStatus` | salida candidata |
| E013 | `PATCH /api/v1/users/me` | 200 → 200 | `name` | `name`, `updatedAt` | entrada candidata; salida candidata |
| E014 | `DELETE /api/v1/users/me` | 202 → 202 | — | `deletionRequestId`, `message` | salida candidata |
| E015 | `GET /api/v1/privacy/notices/current` | 200 → 200 | `locale` | `noticeId`, `title`, `content`, `locale`, `requiresAcceptance` | entrada candidata; salida candidata |
| E016 | `GET /api/v1/users/me/consents` | 200 → 200 | — | `consents[]`, `consents[].noticeId`, `consents[].version`, `consents[].acceptedAt`, `consents[].status` | salida candidata |
| E017 | `POST /api/v1/users/me/consents` | 201 → 200 | `noticeId`, `version`, `accepted` | `noticeId`, `version`, `acceptedAt` | http exito; entrada candidata; salida candidata |
| E018 | `GET /api/v1/users/me/onboarding` | 200 → 200 | — | `microphone`, `notifications`, `configured`, `completedAt` | salida candidata |
| E019 | `PATCH /api/v1/users/me/onboarding` | 200 → 200 | `microphone`, `notifications`, `configured` | `microphone`, `notifications`, `configured`, `completedAt`, `updatedAt` | entrada candidata; salida candidata |
| E020 | `GET /api/v1/users/me/export` | 200 → 200 | `format` | `exportVersion`, `user`, `income`, `movements[]`, `recurringPayments[]`, `goals[]`… | entrada candidata; salida candidata |
| E021 | `GET /api/v1/bootstrap` | 200 → 200 | `include`, `period` | `user`, `income`, `recurringPayments[]`, `goals[]`, `budgets[]`, `onboarding`… | entrada candidata; salida candidata |
| E022 | `GET /api/v1/catalogs/categories` | 200 → 200 | `includeInactive` | `categories[]`, `categories[].id`, `categories[].name`, `categories[].emoji`, `categories[].budgetable`, `categories[].allowedExpenseTyp`… | entrada candidata; salida candidata |
| E023 | `GET /api/v1/catalogs/frequencies` | 200 → 200 | `context` | `frequencies[]`, `frequencies[].code`, `frequencies[].label`, `frequencies[].monthlyFactor`, `frequencies[].allowedContexts[]`, `version` | entrada candidata; salida candidata |
| E024 | `GET /api/v1/income` | 200 → 200 | — | `monthlyEquivalent`, `updatedAt` | salida candidata |
| E025 | `PUT /api/v1/income` | 200 → 200 | — | `monthlyEquivalent`, `updatedAt` | salida candidata |
| E026 | `GET /api/v1/financial-summary` | 200 → 200 | `period` | `incomeMonthly`, `recurringFixedMonthly`, `oneTimeFixedMonthly`, `fixedMonthly`, `antMonthly`, `reservedMonthly`… | entrada candidata; salida candidata |
| E027 | `GET /api/v1/movements` | 200 → 200 | `type`, `period`, `from`, `to`, `category`, `limit`… | `items[]`, `items[].id`, `items[].concept`, `items[].amount`, `items[].category`, `items[].type`… | entrada candidata; salida candidata |
| E028 | `POST /api/v1/movements` | 201 → 201 | `concept`, `category`, `frequency`, `source` | `concept`, `category`, `frequency`, `source`, `summary`, `summary.period`… | entrada candidata; salida candidata |
| E029 | `GET /api/v1/movements/{movementId}` | 200 → 200 | `movementId` | `concept`, `category`, `frequency`, `recurringPaymentId`, `goalId`, `contributionId` | entrada candidata; salida candidata |
| E030 | `PATCH /api/v1/movements/{movementId}` | 200 → 200 | `movementId`, `concept`, `amount`, `category`, `type` | `concept`, `category`, `updatedAt` | entrada candidata; salida candidata |
| E031 | `DELETE /api/v1/movements/{movementId}` | 204 → 200 | `movementId`, `reason` | `deletedMovementId`, `affectedGoalId`, `reversedContributionId`, `summary`, `summary.period`, `summary.currency`… | http exito; entrada candidata; salida candidata |
| E032 | `GET /api/v1/recurring-payments` | 200 → 200 | `type`, `limit`, `cursor` | `items[]`, `items[].id`, `items[].concept`, `items[].amount`, `items[].frequency`, `items[].category`… | entrada candidata; salida candidata |
| E033 | `POST /api/v1/recurring-payments` | 201 → 201 | `concept`, `category`, `type`, `startDate`, `dueDay` | `concept`, `category`, `type`, `monthlyEquivalent`, `startDate` | entrada candidata; salida candidata |
| E034 | `GET /api/v1/recurring-payments/{recurringPaymentId}` | 200 → 200 | `recurringPaymentId` | `concept`, `category`, `type`, `monthlyEquivalent`, `startDate` | entrada candidata; salida candidata |
| E035 | `PATCH /api/v1/recurring-payments/{recurringPaymentId}` | 200 → 200 | `recurringPaymentId`, `concept`, `frequency`, `category`, `type`, `startDate`… | `concept`, `category`, `type`, `monthlyEquivalent`, `updatedAt` | entrada candidata; salida candidata |
| E036 | `DELETE /api/v1/recurring-payments/{recurringPaymentId}` | 204 → 204 | `recurringPaymentId` | — | entrada candidata |
| E037 | `POST /api/v1/recurring-payments/{recurringPaymentId}/payments` | 200 → 201 | `recurringPaymentId`, `occurrenceDate` | `movement.id`, `movement.concept`, `movement.amount`, `movement.category`, `movement.type`, `movement.recurringPaymentId`… | http exito; entrada candidata; salida candidata |
| E038 | `GET /api/v1/goals` | 200 → 200 | `status`, `limit`, `cursor` | `items[]`, `items[].id`, `items[].name`, `items[].target`, `items[].openingSaved`, `items[].saved`… | entrada candidata; salida candidata |
| E039 | `POST /api/v1/goals` | 201 → 201 | `target`, `openingSaved`, `emoji` | `target`, `openingSaved`, `saved`, `emoji`, `remaining`, `percentage`… | entrada candidata; salida candidata |
| E040 | `GET /api/v1/goals/{goalId}` | 200 → 200 | `goalId` | `target`, `openingSaved`, `contributedNet`, `saved`, `emoji`, `remaining`… | entrada candidata; salida candidata |
| E041 | `PATCH /api/v1/goals/{goalId}` | 200 → 200 | `goalId`, `target`, `openingSaved`, `emoji` | `target`, `openingSaved`, `contributedNet`, `saved`, `emoji`, `remaining`… | entrada candidata; salida candidata |
| E042 | `DELETE /api/v1/goals/{goalId}` | 204 → 200 | `goalId` | `deletedGoalId`, `releasedAmount`, `reversedAllocations`, `affectedPeriods[]` | http exito; entrada candidata; salida candidata |
| E043 | `POST /api/v1/goal-contributions/preview` | 200 → 200 | `totalAmount`, `mode`, `splitMode`, `goalId`, `goalIds[]`, `allocations[]`… | `totalAmount`, `allocatedAmount`, `unallocatedAmount`, `allocations[]`, `allocations[].goalId`, `allocations[].goalName`… | entrada candidata; salida candidata |
| E044 | `POST /api/v1/goal-contributions` | 201 → 201 | `totalAmount`, `mode`, `splitMode`, `goalId`, `goalIds[]`, `allocations[]`… | `contribution.id`, `contribution.totalRequested`, `contribution.totalApplied`, `contribution.unallocated`, `contribution.mode`, `contribution.splitMode`… | entrada candidata; salida candidata |
| E045 | `GET /api/v1/goal-contributions` | 200 → 200 | `goalId`, `from`, `to`, `status`, `limit`, `cursor` | `items[]`, `items[].id`, `items[].totalRequested`, `items[].totalApplied`, `items[].unallocated`, `items[].mode`… | entrada candidata; salida candidata |
| E046 | `GET /api/v1/goal-contributions/{contributionId}` | 200 → 200 | `contributionId` | `totalRequested`, `totalApplied`, `unallocated`, `mode`, `splitMode`, `createdAt`… | entrada candidata; salida candidata |
| E047 | `POST /api/v1/goal-contributions/{contributionId}/reverse` | 200 → 200 | `contributionId` | `contributionId`, `reversedAmount`, `affectedGoals[]`, `affectedGoals[].goalId`, `affectedGoals[].released`, `affectedGoals[].savedAfter`… | entrada candidata; salida candidata |
| E048 | `GET /api/v1/budgets` | 200 → 200 | `period` | `currency`, `limits[]`, `limits[].category`, `limits[].monthlyLimit`, `limits[].enabled`, `updatedAt`… | entrada candidata; salida candidata |
| E049 | `PUT /api/v1/budgets` | 200 → 200 | `limits[]`, `limits[].category`, `limits[].monthlyLimit` | `limits[]`, `limits[].category`, `limits[].monthlyLimit`, `limits[].enabled`, `updatedAt`, `version` | entrada candidata; salida candidata |
| E050 | `GET /api/v1/budgets/usage` | 200 → 200 | `period` | `period`, `items[]`, `items[].category`, `items[].limit`, `items[].spent`, `items[].remaining`… | entrada candidata; salida candidata |
| E051 | `GET /api/v1/dashboard` | 200 → 200 | `period` | `financialSummary`, `streak`, `recurringPreview[]`, `topCategories[]`, `tip.title`, `tip.text`… | entrada candidata; salida candidata |
| E052 | `GET /api/v1/analytics/insights` | 200 → 200 | `windowDays`, `endDate` | `windowDays`, `from`, `to`, `antExpenseCount`, `minimumSamples`, `enoughData`… | entrada candidata; salida candidata |
| E053 | `GET /api/v1/analytics/categories` | 200 → 200 | `period`, `limit`, `type` | `period`, `categories[]`, `categories[].category`, `categories[].total`, `categories[].percentageOfTop`, `categories[].shareOfTotal`… | entrada candidata; salida candidata |
| E054 | `GET /api/v1/analytics/spending-patterns` | 200 → 200 | `windowDays` | `windowDays`, `count`, `afternoonWindowStart`, `afternoonWindowEndExclusive`, `afternoonCount`, `afternoonPercentage`… | entrada candidata; salida candidata |
| E055 | `GET /api/v1/analytics/recommendations` | 200 → 200 | `period`, `limit` | `recommendations[]`, `recommendations[].id`, `recommendations[].title`, `recommendations[].text`, `recommendations[].reason`, `recommendations[].basedOnPeri`… | entrada candidata; salida candidata |
| E056 | `GET /api/v1/analytics/savings-streak` | 200 → 200 | `asOf` | `lastSavingDate`, `nextMilestoneDays`, `progressPercentage`, `pigMessage`, `milestones[]`, `milestones[].days`… | entrada candidata; salida candidata |
| E057 | `GET /api/v1/notification-preferences` | 200 → 200 | — | `enabled`, `budgetAlerts`, `goalUpdates`, `recurringReminders`, `streakReminders`, `quietHours.enabled`… | salida candidata |
| E058 | `PUT /api/v1/notification-preferences` | 200 → 200 | `enabled`, `budgetAlerts`, `goalUpdates`, `recurringReminders`, `streakReminders`, `quietHours.enabled`… | `enabled`, `budgetAlerts`, `goalUpdates`, `recurringReminders`, `streakReminders`, `quietHours`… | entrada candidata; salida candidata |
| E059 | `POST /api/v1/devices/push-tokens` | 201 → 201 | `deviceId`, `platform`, `appVersion`, `locale`, `timeZone` | `deviceRegistrationId`, `deviceId`, `active` | entrada candidata; salida candidata |
| E060 | `DELETE /api/v1/devices/push-tokens/{deviceRegistrationId}` | 204 → 204 | `deviceRegistrationId` | — | entrada candidata |
| E061 | `GET /api/v1/notifications` | 200 → 200 | `unreadOnly`, `type`, `limit`, `cursor` | `items[]`, `items[].id`, `items[].type`, `items[].title`, `items[].body`, `items[].relatedEntityId`… | entrada candidata; salida candidata |
| E062 | `PATCH /api/v1/notifications/{notificationId}/read` | 200 → 200 | `notificationId` | `readAt`, `unreadCount` | entrada candidata; salida candidata |
| E063 | `POST /api/v1/notifications/read-all` | 202 → 200 | `before` | `updatedCount`, `readAt` | http exito; entrada candidata; salida candidata |
| E064 | `GET /api/v1/reminders` | 200 → 200 | `type`, `activeOnly`, `limit`, `cursor` | `items[]`, `items[].id`, `items[].type`, `items[].title`, `items[].relatedEntityId`, `items[].nextTriggerAt`… | entrada candidata; salida candidata |
| E065 | `POST /api/v1/reminders` | 201 → 201 | `type`, `relatedEntityId`, `triggerAt`, `frequency`, `timeZone`, `enabled` | `type`, `relatedEntityId`, `nextTriggerAt`, `frequency`, `timeZone`, `enabled`… | entrada candidata; salida candidata |
| E066 | `PATCH /api/v1/reminders/{reminderId}` | 200 → 200 | `reminderId`, `triggerAt`, `frequency`, `timeZone`, `enabled` | `nextTriggerAt`, `frequency`, `timeZone`, `enabled`, `updatedAt` | entrada candidata; salida candidata |
| E067 | `DELETE /api/v1/reminders/{reminderId}` | 204 → 204 | `reminderId` | — | entrada candidata |
| E068 | `GET /api/v1/users/me/preferences` | 200 → 200 | — | `appearance`, `currency`, `timeZone`, `updatedAt` | salida candidata |
| E069 | `PATCH /api/v1/users/me/preferences` | 200 → 200 | `appearance`, `timeZone` | `appearance`, `currency`, `timeZone`, `updatedAt` | entrada candidata; salida candidata |
| E070 | `GET /api/v1/help/faq` | 200 → 200 | `locale` | `items[]`, `items[].id`, `items[].question`, `items[].answer`, `items[].order`, `version` | entrada candidata; salida candidata |
| E071 | `GET /api/v1/education/tips` | 200 → 200 | `locale`, `topic`, `limit` | `items[]`, `items[].id`, `items[].title`, `items[].body`, `items[].topic`, `items[].sourceLabel` | entrada candidata; salida candidata |
| E072 | `GET /api/v1/health/live` | 200 → 200 | — | `version` | salida candidata |
| E073 | `GET /api/v1/health/ready` | 200 → 200 | — | `checks.dynamodb`, `checks.configuration`, `checkedAt` | salida candidata |
| E074 | `GET /api/v1/demo/bootstrap` | 200 → 200 | `version` | `mode`, `userName`, `income`, `recurring[]`, `movements[]`, `goals[]`… | entrada candidata; salida candidata |
| E075 | `POST /api/v1/voice/parse` | 200 → 200 | `transcript` | `concept`, `amount`, `category`, `frequency`, `type`, `confidence`… | entrada candidata; salida candidata |

## Diferencias de HTTP que EXIGEN decisión contractual

No se debe corregir automáticamente: un HTTP distinto puede representar semánticas aprobadas como borrado idempotente (204) frente a respuesta JSON (200).

- **E017** `POST /api/v1/users/me/consents`: SDD `201`, auditoría `200`. RFC de compatibilidad requerido.
- **E031** `DELETE /api/v1/movements/{movementId}`: SDD `204`, auditoría `200`. RFC de compatibilidad requerido.
- **E037** `POST /api/v1/recurring-payments/{recurringPaymentId}/payments`: SDD `200`, auditoría `201`. RFC de compatibilidad requerido.
- **E042** `DELETE /api/v1/goals/{goalId}`: SDD `204`, auditoría `200`. RFC de compatibilidad requerido.
- **E063** `POST /api/v1/notifications/read-all`: SDD `202`, auditoría `200`. RFC de compatibilidad requerido.

## Limitaciones de comparación

- El extractor reconoce columnas de tablas PDF; los campos anidados cortados en varias líneas y referencias derivadas pueden quedar incompletos. Las fichas individuales incluyen ambos inventarios para revisión exacta.
- Ausencia de un campo en el listado SDD no demuestra ausencia en OpenAPI o esquema compartido. No se cambian YAML/JSON hasta revisión y Gate.
- Se mantiene prioridad de `constitution.md`, OpenAPI/SDD aprobado, enmiendas y compatibilidad con E001–E004.
- **E005:** EmailRequest actual: `email` → Accepted `{status,requestId}`. Auditoría PDF: `email`,`locale` (opcional), respuesta `{accepted,message,retryAfterSeconds}`. Se necesita decidir contrato sin enumeración, entrega real de correo y proveedor existente/autorizado.
- **E006:** SDD `token` versus PDF `resetToken`; salida SDD `{status,requestId}` versus PDF `{reset,revokedSessions}`. Resolución en RFC antes de su implementación.
- **Plantilla heredada:** E005–E011 referencia `SESSIONHASH#digest` en spec/plan/tasks. El modelo real validado por E003–E004 usa `USER#id/SESSION#id` y `REFRESH#digest/SESSION`; la corrección se propone con Gate transversal, no se aplica a specs aprobadas.
- **Finanzas:** conservar reglas de centavos enteros, transacciones/idempotencia de aportaciones, reversión y no descontar recurrentes dos veces; verificar en las fichas correspondientes.
