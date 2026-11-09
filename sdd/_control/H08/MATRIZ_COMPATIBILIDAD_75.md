# Matriz de compatibilidad E001–E075 — I-150 (INF-150-03)

| Campo | Valor |
|---|---|
| Spec | `I-150-publicacion-y-compatibilidad-api` |
| Versión | 0.1.0 |
| Estado | **IMPLEMENTADO_LOCAL — NO DESPLEGADO** |
| Operaciones | 75 (33 M · 23 S · 12 E · 7 O) |
| Verificación HTTP | `NOT_RUN` |

> Fuente: `sdd/03-endpoints/MATRIZ_OPERACIONES_H06.csv`. La clase M/S/E/O procede de H06 y **no**
> exime requisitos obligatorios del profesor. Un contrato OpenAPI no demuestra handler funcional ni
> respuesta HTTP real.

| ID | Método | Ruta | Clase | Dominio | Auth | Idempotencia | Éxito | Carpeta |
|---|---|---|---|---|---|---|---|---|
| E001 | POST | `/api/v1/auth/register` | M | identidad | public | True | 201 | `E001-auth-register` |
| E002 | POST | `/api/v1/auth/login` | M | identidad | public | False | 200 | `E002-auth-login` |
| E003 | POST | `/api/v1/auth/refresh` | S | identidad | public | False | 200 | `E003-auth-refresh` |
| E004 | POST | `/api/v1/auth/logout` | M | identidad | bearer | False | 204 | `E004-auth-logout` |
| E005 | POST | `/api/v1/auth/password/forgot` | E | identidad | public | False | 202 | `E005-auth-password-forgot` |
| E006 | POST | `/api/v1/auth/password/reset` | E | identidad | public | False | 200 | `E006-auth-password-reset` |
| E007 | POST | `/api/v1/auth/password/change` | S | identidad | bearer | False | 200 | `E007-auth-password-change` |
| E008 | GET | `/api/v1/auth/sessions` | S | identidad | bearer | False | 200 | `E008-auth-sessions` |
| E009 | DELETE | `/api/v1/auth/sessions/{sessionId}` | S | identidad | bearer | False | 204 | `E009-auth-sessions-sessionId` |
| E010 | POST | `/api/v1/auth/email-verification/request` | S | identidad | bearer | False | 202 | `E010-auth-email-verification-request` |
| E011 | POST | `/api/v1/auth/email-verification/confirm` | S | identidad | public | False | 200 | `E011-auth-email-verification-confirm` |
| E012 | GET | `/api/v1/users/me` | M | usuario-privacidad | bearer | False | 200 | `E012-users-me` |
| E013 | PATCH | `/api/v1/users/me` | S | usuario-privacidad | bearer | False | 200 | `E013-users-me` |
| E014 | DELETE | `/api/v1/users/me` | M | usuario-privacidad | bearer | False | 202 | `E014-users-me` |
| E015 | GET | `/api/v1/privacy/notices/current` | M | usuario-privacidad | public | False | 200 | `E015-privacy-notices-current` |
| E016 | GET | `/api/v1/users/me/consents` | S | usuario-privacidad | bearer | False | 200 | `E016-users-me-consents` |
| E017 | POST | `/api/v1/users/me/consents` | S | usuario-privacidad | bearer | False | 201 | `E017-users-me-consents` |
| E018 | GET | `/api/v1/users/me/onboarding` | S | usuario-privacidad | bearer | False | 200 | `E018-users-me-onboarding` |
| E019 | PATCH | `/api/v1/users/me/onboarding` | S | usuario-privacidad | bearer | False | 200 | `E019-users-me-onboarding` |
| E020 | GET | `/api/v1/users/me/export` | S | usuario-privacidad | bearer | False | 200 | `E020-users-me-export` |
| E021 | GET | `/api/v1/bootstrap` | S | usuario-privacidad | bearer | False | 200 | `E021-bootstrap` |
| E022 | GET | `/api/v1/catalogs/categories` | M | catalogos | public | False | 200 | `E022-catalogs-categories` |
| E023 | GET | `/api/v1/catalogs/frequencies` | S | catalogos | public | False | 200 | `E023-catalogs-frequencies` |
| E024 | GET | `/api/v1/income` | M | finanzas | bearer | False | 200 | `E024-income` |
| E025 | PUT | `/api/v1/income` | M | finanzas | bearer | False | 200 | `E025-income` |
| E026 | GET | `/api/v1/financial-summary` | M | finanzas | bearer | False | 200 | `E026-financial-summary` |
| E027 | GET | `/api/v1/movements` | M | movimientos | bearer | False | 200 | `E027-movements` |
| E028 | POST | `/api/v1/movements` | M | movimientos | bearer | True | 201 | `E028-movements` |
| E029 | GET | `/api/v1/movements/{movementId}` | S | movimientos | bearer | False | 200 | `E029-movements-movementId` |
| E030 | PATCH | `/api/v1/movements/{movementId}` | M | movimientos | bearer | False | 200 | `E030-movements-movementId` |
| E031 | DELETE | `/api/v1/movements/{movementId}` | M | movimientos | bearer | False | 204 | `E031-movements-movementId` |
| E032 | GET | `/api/v1/recurring-payments` | M | recurrentes | bearer | False | 200 | `E032-recurring-payments` |
| E033 | POST | `/api/v1/recurring-payments` | M | recurrentes | bearer | True | 201 | `E033-recurring-payments` |
| E034 | GET | `/api/v1/recurring-payments/{recurringPaymentId}` | S | recurrentes | bearer | False | 200 | `E034-recurring-payments-recurringPaymentId` |
| E035 | PATCH | `/api/v1/recurring-payments/{recurringPaymentId}` | M | recurrentes | bearer | False | 200 | `E035-recurring-payments-recurringPaymentId` |
| E036 | DELETE | `/api/v1/recurring-payments/{recurringPaymentId}` | M | recurrentes | bearer | False | 204 | `E036-recurring-payments-recurringPaymentId` |
| E037 | POST | `/api/v1/recurring-payments/{recurringPaymentId}/payments` | M | recurrentes | bearer | True | 200 | `E037-recurring-payments-recurringPaymentId-payments` |
| E038 | GET | `/api/v1/goals` | M | metas-aportaciones | bearer | False | 200 | `E038-goals` |
| E039 | POST | `/api/v1/goals` | M | metas-aportaciones | bearer | True | 201 | `E039-goals` |
| E040 | GET | `/api/v1/goals/{goalId}` | S | metas-aportaciones | bearer | False | 200 | `E040-goals-goalId` |
| E041 | PATCH | `/api/v1/goals/{goalId}` | M | metas-aportaciones | bearer | False | 200 | `E041-goals-goalId` |
| E042 | DELETE | `/api/v1/goals/{goalId}` | M | metas-aportaciones | bearer | False | 204 | `E042-goals-goalId` |
| E043 | POST | `/api/v1/goal-contributions/preview` | S | metas-aportaciones | bearer | False | 200 | `E043-goal-contributions-preview` |
| E044 | POST | `/api/v1/goal-contributions` | M | metas-aportaciones | bearer | True | 201 | `E044-goal-contributions` |
| E045 | GET | `/api/v1/goal-contributions` | S | metas-aportaciones | bearer | False | 200 | `E045-goal-contributions` |
| E046 | GET | `/api/v1/goal-contributions/{contributionId}` | S | metas-aportaciones | bearer | False | 200 | `E046-goal-contributions-contributionId` |
| E047 | POST | `/api/v1/goal-contributions/{contributionId}/reverse` | S | metas-aportaciones | bearer | True | 200 | `E047-goal-contributions-contributionId-reverse` |
| E048 | GET | `/api/v1/budgets` | M | presupuestos | bearer | False | 200 | `E048-budgets` |
| E049 | PUT | `/api/v1/budgets` | M | presupuestos | bearer | False | 200 | `E049-budgets` |
| E050 | GET | `/api/v1/budgets/usage` | M | presupuestos | bearer | False | 200 | `E050-budgets-usage` |
| E051 | GET | `/api/v1/dashboard` | M | analitica | bearer | False | 200 | `E051-dashboard` |
| E052 | GET | `/api/v1/analytics/insights` | M | analitica | bearer | False | 200 | `E052-analytics-insights` |
| E053 | GET | `/api/v1/analytics/categories` | M | analitica | bearer | False | 200 | `E053-analytics-categories` |
| E054 | GET | `/api/v1/analytics/spending-patterns` | M | analitica | bearer | False | 200 | `E054-analytics-spending-patterns` |
| E055 | GET | `/api/v1/analytics/recommendations` | M | analitica | bearer | False | 200 | `E055-analytics-recommendations` |
| E056 | GET | `/api/v1/analytics/savings-streak` | M | analitica | bearer | False | 200 | `E056-analytics-savings-streak` |
| E057 | GET | `/api/v1/notification-preferences` | E | notificaciones | bearer | False | 200 | `E057-notification-preferences` |
| E058 | PUT | `/api/v1/notification-preferences` | E | notificaciones | bearer | False | 200 | `E058-notification-preferences` |
| E059 | POST | `/api/v1/devices/push-tokens` | E | notificaciones | bearer | True | 201 | `E059-devices-push-tokens` |
| E060 | DELETE | `/api/v1/devices/push-tokens/{deviceRegistrationId}` | E | notificaciones | bearer | False | 204 | `E060-devices-push-tokens-deviceRegistrationId` |
| E061 | GET | `/api/v1/notifications` | E | notificaciones | bearer | False | 200 | `E061-notifications` |
| E062 | PATCH | `/api/v1/notifications/{notificationId}/read` | E | notificaciones | bearer | False | 200 | `E062-notifications-notificationId-read` |
| E063 | POST | `/api/v1/notifications/read-all` | O | notificaciones | bearer | False | 202 | `E063-notifications-read-all` |
| E064 | GET | `/api/v1/reminders` | E | notificaciones | bearer | False | 200 | `E064-reminders` |
| E065 | POST | `/api/v1/reminders` | E | notificaciones | bearer | True | 201 | `E065-reminders` |
| E066 | PATCH | `/api/v1/reminders/{reminderId}` | E | notificaciones | bearer | False | 200 | `E066-reminders-reminderId` |
| E067 | DELETE | `/api/v1/reminders/{reminderId}` | E | notificaciones | bearer | False | 204 | `E067-reminders-reminderId` |
| E068 | GET | `/api/v1/users/me/preferences` | O | soporte-opcional | bearer | False | 200 | `E068-users-me-preferences` |
| E069 | PATCH | `/api/v1/users/me/preferences` | O | soporte-opcional | bearer | False | 200 | `E069-users-me-preferences` |
| E070 | GET | `/api/v1/help/faq` | O | soporte-opcional | public | False | 200 | `E070-help-faq` |
| E071 | GET | `/api/v1/education/tips` | O | soporte-opcional | public | False | 200 | `E071-education-tips` |
| E072 | GET | `/api/v1/health/live` | S | soporte-opcional | public | False | 200 | `E072-health-live` |
| E073 | GET | `/api/v1/health/ready` | S | soporte-opcional | public | False | 200 | `E073-health-ready` |
| E074 | GET | `/api/v1/demo/bootstrap` | O | soporte-opcional | bearer | False | 200 | `E074-demo-bootstrap` |
| E075 | POST | `/api/v1/voice/parse` | O | soporte-opcional | bearer | False | 200 | `E075-voice-parse` |

## Columnas obligatorias verificadas

`id`, `metodo`, `ruta`, `clase_h04`, `dominio`, `resumen`, `alcance`, `auth`, `http_exito`,
`idempotency_key`, `if_match`, `carpeta`, `regla_clave`; `request_schema`/`response_schema` se
exigen salvo respuestas 204/202 sin cuerpo o PATCH sin payload.

## Criterios cloud pendientes

| Criterio | Estado |
|---|---|
| HTTPS público con CA válida | `NOT_RUN` |
| Registro/login/logout reales | `NOT_RUN` |
| Cuatro endpoints adicionales | `NOT_RUN` |
| 401/403/404 con `traceId` | `NOT_RUN` |
| Persistencia en DynamoDB | `NOT_RUN` |
| Paso por los dos nodos | `NOT_RUN` |