# H04 — Matriz íntegra de 75 rutas propuestas V6.0.2

**FUENTE:** catálogo PDF del usuario, 27 páginas, generado 2026-11-02 sobre versión anterior del ZIP. **Estado:** inventario técnico, NO OpenAPI aprobado ni endpoint implementado.

| ID | Método | Ruta propuesta | Fuente | Tratamiento en H04 |
|---|---|---|---|---|
| E001 | POST | `/api/v1/auth/register` | M | Maqueta; contrato funcional necesario |
| E002 | POST | `/api/v1/auth/login` | M | Maqueta; contrato funcional necesario |
| E003 | POST | `/api/v1/auth/refresh` | S | Soporte; evaluar exigencia de seguridad/operación |
| E004 | POST | `/api/v1/auth/logout` | M | Maqueta; contrato funcional necesario |
| E005 | POST | `/api/v1/auth/password/forgot` | E | Prometida/simulada; contrastar docente y resolver cobertura |
| E006 | POST | `/api/v1/auth/password/reset` | E | Prometida/simulada; contrastar docente y resolver cobertura |
| E007 | POST | `/api/v1/auth/password/change` | S | Soporte; evaluar exigencia de seguridad/operación |
| E008 | GET | `/api/v1/auth/sessions` | S | Soporte; evaluar exigencia de seguridad/operación |
| E009 | DELETE | `/api/v1/auth/sessions/{sessionId}` | S | Soporte; evaluar exigencia de seguridad/operación |
| E010 | POST | `/api/v1/auth/email-verification/request` | S | Soporte; evaluar exigencia de seguridad/operación |
| E011 | POST | `/api/v1/auth/email-verification/confirm` | S | Soporte; evaluar exigencia de seguridad/operación |
| E012 | GET | `/api/v1/users/me` | M | Maqueta; contrato funcional necesario |
| E013 | PATCH | `/api/v1/users/me` | S | Soporte; evaluar exigencia de seguridad/operación |
| E014 | DELETE | `/api/v1/users/me` | M | Maqueta; contrato funcional necesario |
| E015 | GET | `/api/v1/privacy/notices/current` | M | Maqueta; contrato funcional necesario |
| E016 | GET | `/api/v1/users/me/consents` | S | Soporte; evaluar exigencia de seguridad/operación |
| E017 | POST | `/api/v1/users/me/consents` | S | Soporte; evaluar exigencia de seguridad/operación |
| E018 | GET | `/api/v1/users/me/onboarding` | S | Soporte; evaluar exigencia de seguridad/operación |
| E019 | PATCH | `/api/v1/users/me/onboarding` | S | Soporte; evaluar exigencia de seguridad/operación |
| E020 | GET | `/api/v1/users/me/export` | S | Soporte; evaluar exigencia de seguridad/operación |
| E021 | GET | `/api/v1/bootstrap` | S | Soporte; evaluar exigencia de seguridad/operación |
| E022 | GET | `/api/v1/catalogs/categories` | M | Maqueta; contrato funcional necesario |
| E023 | GET | `/api/v1/catalogs/frequencies` | S | Soporte; evaluar exigencia de seguridad/operación |
| E024 | GET | `/api/v1/income` | M | Maqueta; contrato funcional necesario |
| E025 | PUT | `/api/v1/income` | M | Maqueta; contrato funcional necesario |
| E026 | GET | `/api/v1/financial-summary` | M | Maqueta; contrato funcional necesario |
| E027 | GET | `/api/v1/movements` | M | Maqueta; contrato funcional necesario |
| E028 | POST | `/api/v1/movements` | M | Maqueta; contrato funcional necesario |
| E029 | GET | `/api/v1/movements/{movementId}` | S | Soporte; evaluar exigencia de seguridad/operación |
| E030 | PATCH | `/api/v1/movements/{movementId}` | M | Maqueta; contrato funcional necesario |
| E031 | DELETE | `/api/v1/movements/{movementId}` | M | Maqueta; contrato funcional necesario |
| E032 | GET | `/api/v1/recurring-payments` | M | Maqueta; contrato funcional necesario |
| E033 | POST | `/api/v1/recurring-payments` | M | Maqueta; contrato funcional necesario |
| E034 | GET | `/api/v1/recurring-payments/{recurringPaymentId}` | S | Soporte; evaluar exigencia de seguridad/operación |
| E035 | PATCH | `/api/v1/recurring-payments/{recurringPaymentId}` | M | Maqueta; contrato funcional necesario |
| E036 | DELETE | `/api/v1/recurring-payments/{recurringPaymentId}` | M | Maqueta; contrato funcional necesario |
| E037 | POST | `/api/v1/recurring-payments/{recurringPaymentId}/payments` | M | Maqueta; contrato funcional necesario |
| E038 | GET | `/api/v1/goals` | M | Maqueta; contrato funcional necesario |
| E039 | POST | `/api/v1/goals` | M | Maqueta; contrato funcional necesario |
| E040 | GET | `/api/v1/goals/{goalId}` | S | Soporte; evaluar exigencia de seguridad/operación |
| E041 | PATCH | `/api/v1/goals/{goalId}` | M | Maqueta; contrato funcional necesario |
| E042 | DELETE | `/api/v1/goals/{goalId}` | M | Maqueta; contrato funcional necesario |
| E043 | POST | `/api/v1/goal-contributions/preview` | S | Soporte; evaluar exigencia de seguridad/operación |
| E044 | POST | `/api/v1/goal-contributions` | M | Maqueta; contrato funcional necesario |
| E045 | GET | `/api/v1/goal-contributions` | S | Soporte; evaluar exigencia de seguridad/operación |
| E046 | GET | `/api/v1/goal-contributions/{contributionId}` | S | Soporte; evaluar exigencia de seguridad/operación |
| E047 | POST | `/api/v1/goal-contributions/{contributionId}/reverse` | S | Soporte; evaluar exigencia de seguridad/operación |
| E048 | GET | `/api/v1/budgets` | M | Maqueta; contrato funcional necesario |
| E049 | PUT | `/api/v1/budgets` | M | Maqueta; contrato funcional necesario |
| E050 | GET | `/api/v1/budgets/usage` | M | Maqueta; contrato funcional necesario |
| E051 | GET | `/api/v1/dashboard` | M | Maqueta; contrato funcional necesario |
| E052 | GET | `/api/v1/analytics/insights` | M | Maqueta; contrato funcional necesario |
| E053 | GET | `/api/v1/analytics/categories` | M | Maqueta; contrato funcional necesario |
| E054 | GET | `/api/v1/analytics/spending-patterns` | M | Maqueta; contrato funcional necesario |
| E055 | GET | `/api/v1/analytics/recommendations` | M | Maqueta; contrato funcional necesario |
| E056 | GET | `/api/v1/analytics/savings-streak` | M | Maqueta; contrato funcional necesario |
| E057 | GET | `/api/v1/notification-preferences` | E | Prometida/simulada; contrastar docente y resolver cobertura |
| E058 | PUT | `/api/v1/notification-preferences` | E | Prometida/simulada; contrastar docente y resolver cobertura |
| E059 | POST | `/api/v1/devices/push-tokens` | E | Prometida/simulada; contrastar docente y resolver cobertura |
| E060 | DELETE | `/api/v1/devices/push-tokens/{deviceRegistrationId}` | E | Prometida/simulada; contrastar docente y resolver cobertura |
| E061 | GET | `/api/v1/notifications` | E | Prometida/simulada; contrastar docente y resolver cobertura |
| E062 | PATCH | `/api/v1/notifications/{notificationId}/read` | E | Prometida/simulada; contrastar docente y resolver cobertura |
| E063 | POST | `/api/v1/notifications/read-all` | O | Opcional del catálogo; no confundir con requisitos obligatorios |
| E064 | GET | `/api/v1/reminders` | E | Prometida/simulada; contrastar docente y resolver cobertura |
| E065 | POST | `/api/v1/reminders` | E | Prometida/simulada; contrastar docente y resolver cobertura |
| E066 | PATCH | `/api/v1/reminders/{reminderId}` | E | Prometida/simulada; contrastar docente y resolver cobertura |
| E067 | DELETE | `/api/v1/reminders/{reminderId}` | E | Prometida/simulada; contrastar docente y resolver cobertura |
| E068 | GET | `/api/v1/users/me/preferences` | O | Opcional del catálogo; no confundir con requisitos obligatorios |
| E069 | PATCH | `/api/v1/users/me/preferences` | O | Opcional del catálogo; no confundir con requisitos obligatorios |
| E070 | GET | `/api/v1/help/faq` | O | Opcional del catálogo; no confundir con requisitos obligatorios |
| E071 | GET | `/api/v1/education/tips` | O | Opcional del catálogo; no confundir con requisitos obligatorios |
| E072 | GET | `/api/v1/health/live` | S | Soporte; evaluar exigencia de seguridad/operación |
| E073 | GET | `/api/v1/health/ready` | S | Soporte; evaluar exigencia de seguridad/operación |
| E074 | GET | `/api/v1/demo/bootstrap` | O | Opcional del catálogo; no confundir con requisitos obligatorios |
| E075 | POST | `/api/v1/voice/parse` | O | Opcional del catálogo; no confundir con requisitos obligatorios |

### Regla de cambios

- No eliminar ninguna fila ni cambiar método/ruta silenciosamente: H06 deberá justificar cualquier cambio y mantener el ID estable.
- El profesor requiere push, eliminación de cuenta, logout, JWT, sesiones persistentes y ML+generativa en el proyecto, por lo que etiquetas `E/O` del catálogo no eximen dichas obligaciones. Si una capacidad exige nuevas rutas, H06 las inventaria por separado con referencias y autorización.
- `/demo/bootstrap` y `/voice/parse` no son indispensables para reproducir la demo actual: Android usa demo local y reconocimiento local; no crear rutas artificiales sin necesidad.
- MongoDB Atlas/Google Cloud citados en el PDF pertenecen a una arquitectura anterior, no a la decisión vigente AWS + DynamoDB + Redis.
