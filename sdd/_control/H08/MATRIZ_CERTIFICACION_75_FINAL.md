# Matriz definitiva de certificación 75/75 — Nexum-Backend

**Fecha:** 2026-11-26 · **Base de gobierno:** `main` `345f3279d4f31933371e6401bfab33c7d62aaf1a` · **Release desplegada al cierre:** `v0.21.2+a0a49f1e968e` · **Launch Template:** v24 · **Contrato:** `sdd/03-endpoints/openapi/nexum-api-v1.json`, SHA-256 `3f0ec765fe5ea662e4c9223ca091c6cfbe67d2ddb47249c907ff34206dd232ba`, 75 operaciones · **Evidencia productiva:** `dist/cert-evidence/` (copia sanitizada de `C:\Nexum-Custodia\H08E-CERT-20261125`).

> **Alcance de este documento:** certificación funcional y contractual de las 75 operaciones. Se elaboró **solo con lectura local**: no se ejecutaron peticiones HTTP, AWS, Terraform ni mutaciones. No se modificó ningún otro archivo.
>
> **Regla de veracidad:** un `PASS` productivo exige HTTP real por CloudFront con evidencia en `dist/cert-evidence/` o en un documento de control que la cite. Un `PASS` apoyado en la release anterior se identifica como **histórico** y no se extrapola a `v0.21.2`. No se usa un `401` sin JWT como prueba funcional. No se declara entrega SES, entrega FCM ni purga `PURGED` sin evidencia específica.

## 1. Cómo leer la matriz

| Concepto | Criterio aplicado |
|---|---|
| `PASS` | El escenario obligatorio del endpoint se ejecutó con HTTP real y evidencia localizable. Si la evidencia es de `v0.21.1`, se indica en la columna de release y se marca la re-ejecución pendiente. |
| `FAIL` | Una comprobación obligatoria falló en la release desplegada. No se compensa con evidencia histórica. |
| `CONTENT_MISMATCH` | El endpoint responde, pero el contenido no cumple el contrato o la obligación de negocio. |
| `CONTRACT_MISMATCH` | La respuesta o los códigos difieren del OpenAPI. No se usó si la diferencia es solo una expectativa errónea del harness. |
| `BLOCKED_ANDROID` | La certificación restante exige dispositivo físico (recepción push real o E2E FCM). |
| `BLOCKED_DEPENDENCY` | Falta una corrección ya integrada en `main` pero **no desplegada** en `v0.21.2`, o falta un prerrequisito funcional. |
| `BLOCKED_EXTERNAL` | Dependencia externa no observable por HTTP (p. ej. buzón de correo). No se usó como comodín. |
| `BLOCKED_AUTHORIZATION` | Falta una autorización humana. No se usó en esta matriz. |
| `NOT_RUN` | No hay ejecución. No se usó: todas las filas tienen evidencia productiva o de control. |

**Mapa de releases usado (solo si la evidencia lo respalda):**

| Release | LT | Fuente | Uso en esta matriz |
|---|---|---|---|
| `v0.21.2+a0a49f1e968e` | v24 | `dist/auditoria/CHECKPOINT_FCM_V25.md` | Release desplegada; evidencia `50`–`60` y `70` |
| `v0.21.1+0a73a1e89c19` | NO_CONSTA | `dist/auditoria/verificar_plan_v0211.py` y `verificar_estado_v0211.py` | Evidencia `30`–`46` |
| `v0.21.0+b527f981c1d7` | NO_CONSTA | `dist/auditoria/append_memory_e007.py`, `append_memory_mov.py`, `pr149-body.md` | Evidencia `01`–`24` y `tmp-e001.json` |
| `v0.18.0+9722224c15c5` | v14 | `sdd/_control/H08/CIERRE_OLEADA_2.md` | E014 42/42 |
| `v0.15.0+8f13c86d2ca0` | v10 | `sdd/_control/H08/GATE_H08E_E007_E011_DESPLIEGUE_AWS.md` | Positivos históricos E001–E011 |
| `v0.21.3` | NO_DESPLEGADA | `main` `345f327` + rama de certificación `593d35c` | Pendientes de re-ejecución |

**Resumen de cobertura:** 75 filas únicas, sin duplicados ni huecos. **37 filas** tienen evidencia directa de `v0.21.2` (incluye E010, E011 y E060–E063); **37 filas** se apoyan en evidencia productiva de `v0.21.1`; **E014** combina evidencia histórica 42/42 con evidencia parcial de `v0.21.2`.

## 2. Matriz E001–E010

| E-ID | Método | Ruta | Release/LT probados | Casos obligatorios ejecutados (resumen) | Evidencia (archivo/prueba concreta) | Resultado | Seguridad/aislamiento | Pendiente | Justificación de reutilización |
|---|---|---|---|---|---|---|---|---|---|
| E001 | POST | `/api/v1/auth/register` | `v0.21.0+b527f981c1d7` / LT NO_CONSTA; `v0.21.1+0a73a1e89c19` / LT NO_CONSTA | Alta 201; `userId`; replay 201 con `X-Idempotent-Replay`; 409 con misma clave y otra carga. En v0.21.1 la corrida reutilizó cuentas A/B (preexistencia autenticada). | `tmp-e001.json` (201/replay/409; el FAIL del archivo fue por `traceId` no UUID y 429 de cuota); `46-v0211-e006b.json` (preexistencia A/B); `GATE_H08E_E007_E011_DESPLIEGUE_AWS.md` §4 (201 histórico). | PASS | Cuentas A/B separadas; sin correos, contraseñas ni tokens en la evidencia; cuota de registro respetada (429 en ráfaga). | Alta nueva con cuentas sintéticas nuevas y replay/409; el harness actual reutiliza cuentas por DEF-H11. | Alta 201 histórica + preexistencia v0.21.1; no se extrapola a alta nueva en v0.21.2. |
| E002 | POST | `/api/v1/auth/login` | `v0.21.2+a0a49f1e968e` / LT v24 | Login A/B 200; `tokenType=Bearer`; `expiresIn=900`; `session.id=sessionId`; 401 con contraseña incorrecta y sin distinguir motivo. | `70-v0213-e010.json` (E002, 10 checks); `53-v0212-regresion.json`. | PASS | 401 genérico; sin enumeración; cuota dual IP/email; tokens no persistidos en evidencia. | Re-ejecutar en v0.21.3; sin pendiente funcional. | Evidencia directa en la release desplegada; no se reutiliza. |
| E003 | POST | `/api/v1/auth/refresh` | `v0.21.2+a0a49f1e968e` / LT v24 | Refresh 200; tokens nuevos; misma sesión; access nuevo usable 200; replay del refresh consumido 409; tokens fuera de resultados. | `53-v0212-regresion.json`. | PASS | Rotación atómica; replay revoca familia; A/B aislados; sin tokens en evidencia. | Re-ejecutar en v0.21.3. | Evidencia directa; no se reutiliza. |
| E004 | POST | `/api/v1/auth/logout` | `v0.21.2+a0a49f1e968e` / LT v24 | Logout 204 sin cuerpo; refresh posterior 401; access posterior 401. | `53-v0212-regresion.json`. | PASS | Revocación durable en DynamoDB; sin cuerpo; access/refresh invalidados. | Re-ejecutar en v0.21.3. | Evidencia directa; no se reutiliza. |
| E005 | POST | `/api/v1/auth/password/forgot` | `v0.21.1+0a73a1e89c19` / LT NO_CONSTA | 202 `accepted`; `requestId`; sin token en la respuesta. | `46-v0211-e006b.json`; `GATE_H08E_E005_DESPLIEGUE_AWS.md` (MessageId y Delivery=1 históricos). | PASS | Sin enumeración; token no devuelto; cuota; correo por SES. | Recepción en buzón no verificada; re-ejecutar 202 y entrega en v0.21.3. | 202 v0.21.1; no se reutiliza para afirmar entrega en buzón. |
| E006 | POST | `/api/v1/auth/password/reset` | `v0.21.1+0a73a1e89c19` / LT NO_CONSTA | Restablecimiento 200; replay del token 409; contraseña anterior 401; login con la nueva 200. | `46-v0211-e006b.json`. | PASS | Token de un solo uso; replay 409; epoch/sesiones; sin token en evidencia. | Re-ejecutar con token nuevo en v0.21.3. | Positivo v0.21.1; no re-ejecutado en v0.21.2. |
| E007 | POST | `/api/v1/auth/password/change` | `v0.21.1+0a73a1e89c19` / LT NO_CONSTA | `currentPassword` incorrecta 422; cambio 200; `requestId`; login anterior 401 y nueva 200; access/refresh previos 401; access nuevo 200; sesiones coherentes. | `46-v0211-e006b.json`. | PASS | Epoch invalida access/refresh previos; 422 sin mutación; sin tokens. | Re-ejecutar 422 tras el fix de la rama `593d35c` y regresión en v0.21.3. | Positivo v0.21.1; el fix de rama no está desplegado. |
| E008 | GET | `/api/v1/auth/sessions` | `v0.21.1+0a73a1e89c19` / LT NO_CONSTA | Listado no vacío; sesión actual marcada; `current` coincide con el token; sin tokens en el listado. | `46-v0211-e006b.json`. | PASS | Solo sesiones propias; sin tokens; 401/404. | Re-ejecutar 429/`Retry-After` con Redis en v0.21.3. | Positivo v0.21.1; no re-ejecutado en v0.21.2. |
| E009 | DELETE | `/api/v1/auth/sessions/{sessionId}` | `v0.21.1+0a73a1e89c19` / LT NO_CONSTA | Revocación 204; cuerpo vacío; `revoked=true`; sesión A invisible para B; 404 de sesión inexistente. | `46-v0211-e006b.json`. | PASS | Aislamiento A/B; 404 homogéneo; sin cuerpo. | Re-ejecutar 429 y 404 en v0.21.3. | Positivo v0.21.1; no re-ejecutado en v0.21.2. |
| E010 | POST | `/api/v1/auth/email-verification/request` | `v0.21.2+a0a49f1e968e` / LT v24 (FAIL); `v0.21.1+0a73a1e89c19` / LT NO_CONSTA (202 histórico) | 202 esperado; v0.21.2 devolvió **503 a 12.2 s**; 409 de cuenta ya verificada PASS. Histórico 202 en v0.21.1 y en el Gate v0.15.0. | `70-v0213-e010.json` (FAIL 503; 409 PASS); `41-v0211-e010.json` y `42-v0211-e010b.json` (202 histórico). | FAIL | Sin token en la respuesta; 409 sin reenvío; cuota; sin enumeración. | Desplegar `#159` (main `345f327`) y re-ejecutar 202/409; verificar SES y entrega. | No se reutiliza el 202 histórico para ocultar el FAIL de v0.21.2; se conserva como antecedente. |

## 3. Matriz E011–E020

| E-ID | Método | Ruta | Release/LT probados | Casos obligatorios ejecutados (resumen) | Evidencia (archivo/prueba concreta) | Resultado | Seguridad/aislamiento | Pendiente | Justificación de reutilización |
|---|---|---|---|---|---|---|---|---|---|
| E011 | POST | `/api/v1/auth/email-verification/confirm` | `v0.21.2+a0a49f1e968e` / LT v24; `v0.15.0+8f13c86d2ca0` / LT v10 (histórico) | Confirmación 200; replay 409; perfil B `emailVerified=true`. El archivo `51` marcó CONTENT_MISMATCH **solo** porque el harness esperaba `emailVerified` en la respuesta; el contrato devuelve `Accepted{status,requestId}`. | `51-v0212-e011.json` (200/409/perfil true; único fail no contractual); `GATE_H08E_E007_E011_DESPLIEGUE_AWS.md` §4 (200 histórico); `herramientas/certificacion/escenarios.py` actual (contrato `Accepted`). | PASS | Token de un solo uso; replay 409; verificación persistida; sin token en evidencia. | Re-ejecutar con harness corregido y verificar `status/requestId`; E2E con buzón. | Positivo histórico v0.15.0; el CONTENT_MISMATCH v0.21.2 es del harness, no del endpoint. |
| E012 | GET | `/api/v1/users/me` | `v0.21.2+a0a49f1e968e` / LT v24 | Perfil 200; `id`; email de la sesión; `version >= 1`. | `53-v0212-regresion.json`. | PASS | JWT `sub`; solo perfil propio; sin datos de B. | Re-ejecutar en v0.21.3. | Evidencia directa; no se reutiliza. |
| E013 | PATCH | `/api/v1/users/me` | `v0.21.1+0a73a1e89c19` / LT NO_CONSTA | 428 sin `If-Match`; 200 con `If-Match`; `displayName` persistido; versión incrementada; 412 con versión obsoleta y con ETag débil. | `46-v0211-e006b.json`. | PASS | JWT; CAS por versión; sin datos de B. | Re-ejecutar en v0.21.3; ETag de respuesta no declarado en OpenAPI. | Positivo v0.21.1; no re-ejecutado en v0.21.2. |
| E014 | DELETE | `/api/v1/users/me` | `v0.18.0+9722224c15c5` / LT v14 (42/42); `v0.21.2+a0a49f1e968e` / LT v24 (202/IDs) | DELETE 202 con `requestId` y `sessionsRevoked`; login/access posteriores 401; **PURGED 42/42 histórico**; IDs de v0.21.2. | `e014-final-ids.json`; `CIERRE_OLEADA_2.md` (42/42); `e014-b.json` (400 previo, no usado). | PASS | Tombstone + `sessionEpoch`; 401 posterior; B intacto; purga verificada histórica. | Re-verificar `PURGED` y familias `OUTBOX#GLOBAL`/`SCHEDULE#GLOBAL` tras `#145` en v0.21.3. | 42/42 histórico + 202 v0.21.2; `PURGED` no re-medido en v0.21.2. |
| E015 | GET | `/api/v1/privacy/notices/current` | `v0.21.2+a0a49f1e968e` / LT v24 | 200; versión `2026-11-v2`; `effectiveAt`; contenido no vacío (6648 caracteres); `requiresAcceptance`. | `53-v0212-regresion.json`; `MATRIZ_AUDITORIA_54_ENDPOINTS.md` (`NO_CONFORME_SEMANTICA`). | CONTENT_MISMATCH | Público; sin PII; contenido versionado. | Publicar v3 tras Gate legal; hoy v2 declara no disponibles funciones ya desplegadas. | No se reutiliza como PASS: el contrato técnico pasa, el contenido no es conforme. |
| E016 | GET | `/api/v1/users/me/consents` | `v0.21.1+0a73a1e89c19` / LT NO_CONSTA | Consentimientos listados; consentimiento del aviso presente. | `17-perfil.json` y `46-v0211-e006b.json`. | PASS | JWT; solo consentimientos propios. | Re-ejecutar en v0.21.3. | Positivo v0.21.1; no re-ejecutado en v0.21.2. |
| E017 | POST | `/api/v1/users/me/consents` | `v0.21.1+0a73a1e89c19` / LT NO_CONSTA | 201 con `id`; consentimiento persistido. `Location` no emitida (cabecera opcional del contrato). | `21-e017.json` y `46-v0211-e006b.json`. | PASS | JWT; consentimiento propio; sin datos de B. | Re-ejecutar en v0.21.3. | Positivo v0.21.1; no re-ejecutado en v0.21.2. |
| E018 | GET | `/api/v1/users/me/onboarding` | `v0.21.1+0a73a1e89c19` / LT NO_CONSTA | `configured` y `done` booleanos. | `17-perfil.json` y `46-v0211-e006b.json`. | PASS | JWT; solo onboarding propio. | Re-ejecutar en v0.21.3. | Positivo v0.21.1; no re-ejecutado en v0.21.2. |
| E019 | PATCH | `/api/v1/users/me/onboarding` | `v0.21.1+0a73a1e89c19` / LT NO_CONSTA | `completedSteps` y `configured` persistidos; versión incrementada. | `17-perfil.json` y `46-v0211-e006b.json`. | PASS | JWT; CAS; solo usuario propio. | Re-ejecutar en v0.21.3. | Positivo v0.21.1; no re-ejecutado en v0.21.2. |
| E020 | GET | `/api/v1/users/me/export` | `v0.21.1+0a73a1e89c19` / LT NO_CONSTA | Perfil, `movements[]`, `goals[]`, `consents[]`; sin tokens, hashes ni leases; sin datos de B; exportación agregada de 43 elementos. **No cubre notificaciones legacy.** | `46-v0211-e006b.json`; `8b5e8d7` (`#156`, exportación legacy pendiente de despliegue). | CONTENT_MISMATCH | Solo datos propios; sin tokens/hashes; B no aparece. | Desplegar `#156` y re-ejecutar export con notificaciones legacy; cubrir `notifications[]`. | No se reutiliza como PASS: la exportación legacy está incompleta en v0.21.2. |

## 4. Matriz E021–E030

| E-ID | Método | Ruta | Release/LT probados | Casos obligatorios ejecutados (resumen) | Evidencia (archivo/prueba concreta) | Resultado | Seguridad/aislamiento | Pendiente | Justificación de reutilización |
|---|---|---|---|---|---|---|---|---|---|
| E021 | GET | `/api/v1/bootstrap` | `v0.21.1+0a73a1e89c19` / LT NO_CONSTA | `profile`, `financialSummary` y `catalogVersion`; `id` y email coinciden con la sesión. | `46-v0211-e006b.json`. | PASS | JWT; solo datos propios. | Re-ejecutar en v0.21.3. | Positivo v0.21.1; no re-ejecutado en v0.21.2. |
| E022 | GET | `/api/v1/catalogs/categories` | `v0.21.2+a0a49f1e968e` / LT v24 | Catálogo no vacío; 7 categorías; ids únicos; categorías de gasto presentes. | `53-v0212-regresion.json`. | PASS | Público; contenido estático; sin datos personales. | Re-ejecutar en v0.21.3. | Evidencia directa; no se reutiliza. |
| E023 | GET | `/api/v1/catalogs/frequencies` | `v0.21.2+a0a49f1e968e` / LT v24 | 7 frecuencias canónicas; sin alias duplicados (`biweekly/yearly` no se emiten). | `53-v0212-regresion.json`. | PASS | Público; contenido estático. | Re-ejecutar en v0.21.3. | Evidencia directa; no se reutiliza. |
| E024 | GET | `/api/v1/income` | `v0.21.1+0a73a1e89c19` / LT NO_CONSTA | `amount` decimal; `currency=MXN`. | `18-finanzas.json` y `46-v0211-e006b.json`. | PASS | JWT; solo ingreso propio. | Re-ejecutar en v0.21.3; ETag de respuesta no declarado. | Positivo v0.21.1; no re-ejecutado en v0.21.2. |
| E025 | PUT | `/api/v1/income` | `v0.21.1+0a73a1e89c19` / LT NO_CONSTA | 428 sin `If-Match`; número flotante rechazado; 200 con `If-Match`; importe exacto; 412 con versión obsoleta; persistencia. | `18-finanzas.json` y `46-v0211-e006b.json`. | PASS | JWT; CAS; importes en centavos. | Re-ejecutar en v0.21.3; `0.00` del patrón devuelve 422. | Positivo v0.21.1; no re-ejecutado en v0.21.2. |
| E026 | GET | `/api/v1/financial-summary` | `v0.21.2+a0a49f1e968e` / LT v24 | Periodo solicitado; `income`, `expenses`, `savings` y `net` exactos; `currency=MXN`; aislamiento B en los cuatro; periodo inválido rechazado. | `53-v0212-regresion.json`. | PASS | JWT; A/B aislados; sin caché de respuesta. | Re-ejecutar en v0.21.3. | Evidencia directa; no se reutiliza. |
| E027 | GET | `/api/v1/movements` | `v0.21.2+a0a49f1e968e` / LT v24 | Movimiento listado; filtro `type=expense`; `type` inválido rechazado; `nextCursor` tipado. **No cubrió `from`/`to`.** | `54-v0212-finanzas-get.json`. | PASS | JWT; solo movimientos propios; cursor firmado. | Re-ejecutar `from`/`to` tras el fix de la rama `593d35c`; v0.21.2 no cubrió rango. | Evidencia directa de los casos ejecutados; el rango de fechas queda pendiente. |
| E028 | POST | `/api/v1/movements` | `v0.21.2+a0a49f1e968e` / LT v24 | 201; `Location`; replay 201 con el mismo `id`; 409 con la misma clave y otra carga. | `55-v0212-finanzas-delete.json`. | PASS | JWT; `Idempotency-Key` por usuario; importes exactos. | Re-ejecutar en v0.21.3. | Evidencia directa; no se reutiliza. |
| E029 | GET | `/api/v1/movements/{movementId}` | `v0.21.2+a0a49f1e968e` / LT v24 | Importe exacto; 404 para otra cuenta (no 403 enumerable). | `55-v0212-finanzas-delete.json`. | PASS | Aislamiento A/B; 404 homogéneo. | Re-ejecutar en v0.21.3. | Evidencia directa; no se reutiliza. |
| E030 | PATCH | `/api/v1/movements/{movementId}` | `v0.21.2+a0a49f1e968e` / LT v24 | 428 sin `If-Match`; 412 con ETag débil; 200 con `If-Match`; descripción persistida; importe intacto; 412 con versión obsoleta; cambio de `occurredAt` mueve el SK y sigue legible. | `55-v0212-finanzas-delete.json`. | PASS | JWT; CAS; aislamiento A/B. | Re-ejecutar en v0.21.3. | Evidencia directa; no se reutiliza. |

## 5. Matriz E031–E040

| E-ID | Método | Ruta | Release/LT probados | Casos obligatorios ejecutados (resumen) | Evidencia (archivo/prueba concreta) | Resultado | Seguridad/aislamiento | Pendiente | Justificación de reutilización |
|---|---|---|---|---|---|---|---|---|---|
| E031 | DELETE | `/api/v1/movements/{movementId}` | `v0.21.2+a0a49f1e968e` / LT v24 | 204 sin cuerpo; lectura posterior 404; replay idempotente 204; replay sin cuerpo. | `55-v0212-finanzas-delete.json`. | PASS | JWT; tombstone con puntero; 404 posterior; aislamiento. | Re-ejecutar en v0.21.3; decisión de tombstone ya aprobada. | Evidencia directa; no se reutiliza. |
| E032 | GET | `/api/v1/recurring-payments` | `v0.21.1+0a73a1e89c19` / LT NO_CONSTA | Recurrente listado; filtro `active`. | `34-v0211-e032.json` y `46-v0211-e006b.json`. | PASS | JWT; solo recurrentes propios. | Re-ejecutar en v0.21.3. | Positivo v0.21.1; no re-ejecutado en v0.21.2. |
| E033 | POST | `/api/v1/recurring-payments` | `v0.21.1+0a73a1e89c19` / LT NO_CONSTA | 201 con `id`; `Location`; replay 201 estable; 9 frecuencias aceptadas y 7 canónicas emitidas. | `33-v0211-finanzas.json` y `46-v0211-e006b.json`. | PASS | JWT; `Idempotency-Key`; RFC-021. | Re-ejecutar en v0.21.3. | Positivo v0.21.1; no re-ejecutado en v0.21.2. |
| E034 | GET | `/api/v1/recurring-payments/{recurringPaymentId}` | `v0.21.1+0a73a1e89c19` / LT NO_CONSTA | Importe exacto; `nextDueDate` fecha. | `33-v0211-finanzas.json` y `46-v0211-e006b.json`. | PASS | JWT; 404 para ajeno; proyección de heredados. | Re-ejecutar en v0.21.3. | Positivo v0.21.1; no re-ejecutado en v0.21.2. |
| E035 | PATCH | `/api/v1/recurring-payments/{recurringPaymentId}` | `v0.21.1+0a73a1e89c19` / LT NO_CONSTA | 428 sin `If-Match`; 200 con `If-Match`; nombre persistido; `frequency` no editable rechazada; 412 con versión obsoleta. | `33-v0211-finanzas.json` y `46-v0211-e006b.json`. | PASS | JWT; CAS; `type` inmutable. | Re-ejecutar en v0.21.3. | Positivo v0.21.1; no re-ejecutado en v0.21.2. |
| E036 | DELETE | `/api/v1/recurring-payments/{recurringPaymentId}` | `v0.21.1+0a73a1e89c19` / LT NO_CONSTA | 204 sin cuerpo; baja lógica `active=false`; excluido de `active=true`; replay idempotente. | `33-v0211-finanzas.json` y `46-v0211-e006b.json`. | PASS | JWT; 404 para ajeno; idempotente. | Re-ejecutar en v0.21.3. | Positivo v0.21.1; no re-ejecutado en v0.21.2. |
| E037 | POST | `/api/v1/recurring-payments/{recurringPaymentId}/payments` | `v0.21.1+0a73a1e89c19` / LT NO_CONSTA | Pago genera movimiento; importe exacto; `recurringPaymentId` correcto; categoría de gasto existente; replay devuelve el mismo movimiento; 409 con otra carga. | `33-v0211-finanzas.json` y `46-v0211-e006b.json`. | PASS | JWT; idempotencia; importes en centavos. | Re-ejecutar en v0.21.3. | Positivo v0.21.1; no re-ejecutado en v0.21.2. |
| E038 | GET | `/api/v1/goals` | `v0.21.2+a0a49f1e968e` / LT v24 | `totals`; `activeCount >= 1`; `saved` decimal; filtro `status=active`. | `54-v0212-finanzas-get.json`. | PASS | JWT; solo metas propias. | Re-ejecutar en v0.21.3. | Evidencia directa; no se reutiliza. |
| E039 | POST | `/api/v1/goals` | `v0.21.2+a0a49f1e968e` / LT v24 | 201 con `id`; `openingSaved`; `savedAmount` inicial; replay 201; 409 con otra carga. | `56-v0212-e042.json`. | PASS | JWT; `Idempotency-Key`; importes exactos. | Re-ejecutar en v0.21.3. | Evidencia directa; no se reutiliza. |
| E040 | GET | `/api/v1/goals/{goalId}` | `v0.21.2+a0a49f1e968e` / LT v24 | `openingSaved` visible; 404 para otra cuenta. | `55-v0212-finanzas-delete.json`. | PASS | JWT; 404 homogéneo; aislamiento A/B. | Re-ejecutar en v0.21.3. | Evidencia directa; no se reutiliza. |

## 6. Matriz E041–E050

| E-ID | Método | Ruta | Release/LT probados | Casos obligatorios ejecutados (resumen) | Evidencia (archivo/prueba concreta) | Resultado | Seguridad/aislamiento | Pendiente | Justificación de reutilización |
|---|---|---|---|---|---|---|---|---|---|
| E041 | PATCH | `/api/v1/goals/{goalId}` | `v0.21.1+0a73a1e89c19` / LT NO_CONSTA | 428 sin `If-Match`; `{}` es no-op; 200 con `If-Match`; `targetAmount` persistido; `targetAmount` por debajo de `savedAmount` rechazado. | `33-v0211-finanzas.json` y `46-v0211-e006b.json`. | PASS | JWT; CAS; 422 de negocio; aislamiento. | Re-ejecutar en v0.21.3; discrepancia documental de `{}` cerrada por `#146`. | Positivo v0.21.1; no re-ejecutado en v0.21.2. |
| E042 | DELETE | `/api/v1/goals/{goalId}` | `v0.21.2+a0a49f1e968e` / LT v24 | 204 sin cuerpo; lectura posterior 404; replay idempotente. | `56-v0212-e042.json`. | PASS | JWT; tombstone; 404; aislamiento. | Re-ejecutar en v0.21.3. | Evidencia directa; no se reutiliza. |
| E043 | POST | `/api/v1/goal-contributions/preview` | `v0.21.1+0a73a1e89c19` / LT NO_CONSTA | `remaining=0.00`; `canCommit=true`; suma incompleta rechazada; suma excedida rechazada. | `19-finanzas-v2.json` y `46-v0211-e006b.json`. | PASS | JWT; montos exactos; sin persistencia. | Re-ejecutar en v0.21.3. | Positivo v0.21.1; no re-ejecutado en v0.21.2. |
| E044 | POST | `/api/v1/goal-contributions` | `v0.21.2+a0a49f1e968e` / LT v24 | 201 con `id`; `status=active`; replay estable con el mismo `id`; 409 con otra carga. | `56-v0212-e042.json`. | PASS | JWT; `Idempotency-Key`; importes exactos. | Re-ejecutar en v0.21.3. | Evidencia directa; no se reutiliza. |
| E045 | GET | `/api/v1/goal-contributions` | `v0.21.1+0a73a1e89c19` / LT NO_CONSTA | Aportación listada. | `19-finanzas-v2.json` y `46-v0211-e006b.json`. | PASS | JWT; solo aportaciones propias. | Re-ejecutar en v0.21.3. | Positivo v0.21.1; no re-ejecutado en v0.21.2. |
| E046 | GET | `/api/v1/goal-contributions/{contributionId}` | `v0.21.2+a0a49f1e968e` / LT v24 | Importe exacto; 404 para otra cuenta. | `56-v0212-e042.json`. | PASS | JWT; 404 homogéneo; aislamiento A/B. | Re-ejecutar en v0.21.3. | Evidencia directa; no se reutiliza. |
| E047 | POST | `/api/v1/goal-contributions/{contributionId}/reverse` | `v0.21.2+a0a49f1e968e` / LT v24 | `status=reversed`; `savedAmount` vuelve al inicial; reversión duplicada 409. | `56-v0212-e042.json`. | PASS | JWT; idempotencia; consistencia de saldo. | Re-ejecutar en v0.21.3. | Evidencia directa; no se reutiliza. |
| E048 | GET | `/api/v1/budgets` | `v0.21.1+0a73a1e89c19` / LT NO_CONSTA | `items[]` devuelto. | `33-v0211-finanzas.json` y `46-v0211-e006b.json`. | PASS | JWT; solo presupuestos propios. | Re-ejecutar en v0.21.3. | Positivo v0.21.1; no re-ejecutado en v0.21.2. |
| E049 | PUT | `/api/v1/budgets` | `v0.21.1+0a73a1e89c19` / LT NO_CONSTA | 428 sin `If-Match`; 200 con `If-Match`; presupuesto persistido; `limitAmount` exacto. | `33-v0211-finanzas.json` y `46-v0211-e006b.json`. | PASS | JWT; CAS; importes exactos. | Re-ejecutar en v0.21.3. | Positivo v0.21.1; no re-ejecutado en v0.21.2. |
| E050 | GET | `/api/v1/budgets/usage` | `v0.21.1+0a73a1e89c19` / LT NO_CONSTA | `limitAmount` exacto; `usedAmount` exacto. | `33-v0211-finanzas.json` y `46-v0211-e006b.json`. | PASS | JWT; solo datos propios; importes exactos. | Re-ejecutar en v0.21.3. | Positivo v0.21.1; no re-ejecutado en v0.21.2. |

## 7. Matriz E051–E060

| E-ID | Método | Ruta | Release/LT probados | Casos obligatorios ejecutados (resumen) | Evidencia (archivo/prueba concreta) | Resultado | Seguridad/aislamiento | Pendiente | Justificación de reutilización |
|---|---|---|---|---|---|---|---|---|---|
| E051 | GET | `/api/v1/dashboard` | `v0.21.2+a0a49f1e968e` / LT v24 | `summary`; `dashboard.period`, `income`, `expenses`, `savings` y `net` iguales a E026; `goals[]`; `latestMovements[]`; `no-store`. | `53-v0212-regresion.json`. | PASS | JWT; A/B aislados; sin caché de respuesta; `no-store`. | Re-ejecutar en v0.21.3; RFC-026 de caché pendiente de publicación documental. | Evidencia directa; no se reutiliza. |
| E052 | GET | `/api/v1/analytics/insights` | `v0.21.1+0a73a1e89c19` / LT NO_CONSTA | `enoughData=true`; `minimumSamples=12`; insights con evidencia; periodo inválido rechazado. | `35-v0211-analitica.json` y `46-v0211-e006b.json`. | PASS | JWT; datos propios; sin caché de datos financieros. | Re-ejecutar en v0.21.3; `period` no vacío. | Positivo v0.21.1; no re-ejecutado en v0.21.2. |
| E053 | GET | `/api/v1/analytics/categories` | `v0.21.1+0a73a1e89c19` / LT NO_CONSTA | Categorías analizadas; suma exacta de importes; porcentajes suman 100. | `36-v0211-analitica2.json` y `46-v0211-e006b.json`. | PASS | JWT; datos propios; porcentajes deterministas. | Re-ejecutar en v0.21.3. | Positivo v0.21.1; no re-ejecutado en v0.21.2. |
| E054 | GET | `/api/v1/analytics/spending-patterns` | `v0.21.1+0a73a1e89c19` / LT NO_CONSTA | Patrones calculados; `sampleSize` coherente (mínimo 3 movimientos y 2 días). | `36-v0211-analitica2.json` y `46-v0211-e006b.json`. | PASS | JWT; datos propios; sin probabilidades inventadas. | Re-ejecutar en v0.21.3. | Positivo v0.21.1; no re-ejecutado en v0.21.2. |
| E055 | GET | `/api/v1/analytics/recommendations` | `v0.21.1+0a73a1e89c19` / LT NO_CONSTA | `hasEnoughData` booleano; `true` con historial; `modelVersion` y `disclaimer` en cada recomendación; determinismo en dos llamadas; ventana de 90 días; `items=[]` sin datos; `period` explícito amplía la ventana. | `35-v0211-analitica.json` y `46-v0211-e006b.json`; `ML_E055_CONTRATO_Y_DISENO.md` (Gate ML-155 aprobado). | PASS | JWT; datos propios; inferencia determinista; sin PII en el artefacto. | Re-ejecutar en v0.21.3 y afirmar `kmeans-v2`/hash del artefacto; el JSON no fija la versión exacta. | Positivo v0.21.1; no re-ejecutado en v0.21.2. |
| E056 | GET | `/api/v1/analytics/savings-streak` | `v0.21.1+0a73a1e89c19` / LT NO_CONSTA | `days >= 0`; `timezone` presente. | `35-v0211-analitica.json` y `46-v0211-e006b.json`. | PASS | JWT; datos propios; zona IANA resuelta. | Re-ejecutar en v0.21.3. | Positivo v0.21.1; no re-ejecutado en v0.21.2. |
| E057 | GET | `/api/v1/notification-preferences` | `v0.21.2+a0a49f1e968e` / LT v24 | `pushEnabled`, `emailEnabled` y `reminderEnabled` booleanos. | `59-v0212-notif-fcmfix.json`. | PASS | JWT; preferencias propias; sin datos de B. | Re-ejecutar en v0.21.3. | Evidencia directa; no se reutiliza. |
| E058 | PUT | `/api/v1/notification-preferences` | `v0.21.2+a0a49f1e968e` / LT v24 | `pushEnabled` persistido; `quietHours` objeto; `timeZone` persistida. | `59-v0212-notif-fcmfix.json`. | PASS | JWT; CAS por versión; preferencias propias. | Re-ejecutar en v0.21.3. | Evidencia directa; no se reutiliza. |
| E059 | POST | `/api/v1/devices/push-tokens` | `v0.21.2+a0a49f1e968e` / LT v24 | Dispositivo 201; `Location`; respuesta sin token FCM. | `59-v0212-notif-fcmfix.json`. | PASS | JWT; token cifrado y no devuelto; `Idempotency-Key`; aislamiento A/B. | Re-ejecutar registro en v0.21.3; E2E FCM en dispositivo pendiente (no bloquea E059). | Evidencia directa; no se reutiliza. |
| E060 | DELETE | `/api/v1/devices/push-tokens/{deviceRegistrationId}` | `v0.21.2+a0a49f1e968e` / LT v24 | Revocación 204; cuerpo vacío; segunda revocación idempotente; dispositivo de A invisible para B. Falta comprobar la **no entrega push** tras revocar. | `59-v0212-notif-fcmfix.json` (`BLOCKED_EXTERNAL` solo por la entrega física). | BLOCKED_ANDROID | JWT; 404 homogéneo; dispositivo ajeno invisible; sin token en respuesta. | E2E FCM físico: registrar token, provocar push, revocar y confirmar que no llega. | El CRUD 204 está verificado; la única comprobación restante exige dispositivo físico. |

## 8. Matriz E061–E070

| E-ID | Método | Ruta | Release/LT probados | Casos obligatorios ejecutados (resumen) | Evidencia (archivo/prueba concreta) | Resultado | Seguridad/aislamiento | Pendiente | Justificación de reutilización |
|---|---|---|---|---|---|---|---|---|---|
| E061 | GET | `/api/v1/notifications` | `v0.21.2+a0a49f1e968e` / LT v24 | Bandeja 200 con `items=0` y `unreadCount=0`; `Cache-Control: private, no-store`; sin duplicados; aislamiento B con intersección 0. **No hubo ocurrencia real**: la bandeja quedó vacía por `entityType`. | `60-v0212-e061-e063.json`; `59-v0212-notif-fcmfix.json` (`BLOCKED_DEPENDENCY` por ausencia de notificación). | BLOCKED_DEPENDENCY | JWT; solo notificaciones propias; cursor firmado; sin tokens. | Desplegar `#156` y sembrar una notificación; verificar `items`, `unreadCount` y filtro `isRead`. | No se reutiliza como PASS: la corrección de `entityType` está en `main` pero no en v0.21.2. |
| E062 | PATCH | `/api/v1/notifications/{notificationId}/read` | `v0.21.2+a0a49f1e968e` / LT v24 | Sin notificación objetivo en v0.21.2; no se ejecutó el caso positivo. Evidencia negativa: `404` con UUID inexistente y `401` sin Bearer. | `60-v0212-e061-e063.json` (`E062.sin_objetivo=true`); `notif-negativos.json`. | BLOCKED_DEPENDENCY | JWT; 404 homogéneo; sin cuerpo; aislamiento A/B. | Depende de E061; verificar ID UUID, idempotencia y aislamiento. | No se reutiliza como PASS: falta la notificación que produce E061. |
| E063 | POST | `/api/v1/notifications/read-all` | `v0.21.2+a0a49f1e968e` / LT v24 | 202 `accepted`; `requestId`; `unread` final 0; todas leídas; segundo 202; revocación de dispositivo 204. | `60-v0212-e061-e063.json`. | BLOCKED_DEPENDENCY | JWT; solo propias; `202` no acredita acabado hasta verificar; sin tokens. | Depende de E061; verificar `read-all` con notificaciones reales y no leídas. | No se reutiliza como PASS: la ocurrencia real no existe en v0.21.2. |
| E064 | GET | `/api/v1/reminders` | `v0.21.2+a0a49f1e968e` / LT v24 | Recordatorio listado; filtro `active`. | `59-v0212-notif-fcmfix.json`. | PASS | JWT; solo recordatorios propios. | Re-ejecutar en v0.21.3; scheduler durable no verificado. | Evidencia directa; no se reutiliza. |
| E065 | POST | `/api/v1/reminders` | `v0.21.2+a0a49f1e968e` / LT v24 | 201; `Location`; replay estable con el mismo `id`. | `59-v0212-notif-fcmfix.json`. | PASS | JWT; `Idempotency-Key`; recordatorio propio. | Re-ejecutar con `frequency` y replay en v0.21.3. | Evidencia directa; no se reutiliza. |
| E066 | PATCH | `/api/v1/reminders/{reminderId}` | `v0.21.1+0a73a1e89c19` / LT NO_CONSTA | 428 sin `If-Match`; título persistido; `dueAt` movido; 412 con versión obsoleta. | `37-v0211-notificaciones.json` y `46-v0211-e006b.json`. | PASS | JWT; CAS; 404 para ajeno. | Re-ejecutar en v0.21.3. | Positivo v0.21.1; no re-ejecutado en v0.21.2. |
| E067 | DELETE | `/api/v1/reminders/{reminderId}` | `v0.21.1+0a73a1e89c19` / LT NO_CONSTA | 204 sin cuerpo; lectura posterior 404; replay idempotente; recordatorio de A invisible para B. | `37-v0211-notificaciones.json` y `46-v0211-e006b.json`. | PASS | JWT; 404; aislamiento A/B; sin cuerpo. | Re-ejecutar en v0.21.3. | Positivo v0.21.1; no re-ejecutado en v0.21.2. |
| E068 | GET | `/api/v1/users/me/preferences` | `v0.21.1+0a73a1e89c19` / LT NO_CONSTA | `locale` y `timezone` presentes. | `32-v0211-perfil.json` y `46-v0211-e006b.json`. | PASS | JWT; solo preferencias propias. | Re-ejecutar en v0.21.3. | Positivo v0.21.1; no re-ejecutado en v0.21.2. |
| E069 | PATCH | `/api/v1/users/me/preferences` | `v0.21.1+0a73a1e89c19` / LT NO_CONSTA | 428 sin `If-Match`; 200 con `If-Match`; `locale` y `timezone` persistidos; 412 con versión obsoleta. | `32-v0211-perfil.json` y `46-v0211-e006b.json`. | PASS | JWT; CAS; `timeZone` IANA. | Re-ejecutar en v0.21.3; ETag no declarado y UTC aceptado pese a D-E069. | Positivo v0.21.1; no re-ejecutado en v0.21.2. |
| E070 | GET | `/api/v1/help/faq` | `v0.21.2+a0a49f1e968e` / LT v24 | 10 FAQ; sin promesa de cambiar/editar el correo desde la app; sin promesa de cuatro frecuencias. | `53-v0212-regresion.json`. | PASS | Público; contenido estático; sin datos personales. | Re-ejecutar en v0.21.3; contenido corregido por `#146`. | Evidencia directa; no se reutiliza. |

## 9. Matriz E071–E075

| E-ID | Método | Ruta | Release/LT probados | Casos obligatorios ejecutados (resumen) | Evidencia (archivo/prueba concreta) | Resultado | Seguridad/aislamiento | Pendiente | Justificación de reutilización |
|---|---|---|---|---|---|---|---|---|---|
| E071 | GET | `/api/v1/education/tips` | `v0.21.2+a0a49f1e968e` / LT v24 | 8 tips; ids únicos. | `53-v0212-regresion.json`. | PASS | Público; contenido estático. | Re-ejecutar en v0.21.3. | Evidencia directa; no se reutiliza. |
| E072 | GET | `/api/v1/health/live` | `v0.21.2+a0a49f1e968e` / LT v24 | `status` en `ok/degraded`; liveness sin dependencias. | `53-v0212-regresion.json`. | PASS | Público; sin secretos; sin dependencias en liveness. | Re-ejecutar health 200 en v0.21.3; un 503 de readiness bloquea. | Evidencia directa; no se reutiliza. |
| E073 | GET | `/api/v1/health/ready` | `v0.21.2+a0a49f1e968e` / LT v24 | `status` en `ok/degraded`; readiness lista (`ok`). | `53-v0212-regresion.json`. | PASS | Público; sin secretos; readiness evalúa dependencias. | Re-ejecutar readiness 200 en v0.21.3; 503 bloquea la certificación. | Evidencia directa; no se reutiliza. |
| E074 | GET | `/api/v1/demo/bootstrap` | `v0.21.2+a0a49f1e968e` / LT v24 | `enabled` booleano; `scope=sandbox`. | `53-v0212-regresion.json`; `39-v0211-e074.json`. | PASS | Público; flag desactivado por defecto; sin datos productivos ni PII. | Re-ejecutar flag apagado y scope sandbox en v0.21.3. | Evidencia directa; no se reutiliza. |
| E075 | POST | `/api/v1/voice/parse` | `v0.21.1+0a73a1e89c19` / LT NO_CONSTA | `intent=createMovement`; importe exacto `150.00`; `requiresConfirmation=true`; ambigüedad explícita y con confirmación; sin persistencia de movimiento. | `38-v0211-contenido.json` y `46-v0211-e006b.json`. | PASS | JWT; solo texto `es-MX`; sin audio; sin DynamoDB ni llamada a E028; sin persistencia. | Re-ejecutar en v0.21.3 con texto; no requiere dispositivo físico. | Positivo v0.21.1; no re-ejecutado en v0.21.2. |

## 10. Totales exactos por estado

| Estado | Total | E-ID |
|---|---:|---|
| PASS | 68 | E001, E002, E003, E004, E005, E006, E007, E008, E009, E011, E012, E013, E014, E016, E017, E018, E019, E021, E022, E023, E024, E025, E026, E027, E028, E029, E030, E031, E032, E033, E034, E035, E036, E037, E038, E039, E040, E041, E042, E043, E044, E045, E046, E047, E048, E049, E050, E051, E052, E053, E054, E055, E056, E057, E058, E059, E064, E065, E066, E067, E068, E069, E070, E071, E072, E073, E074, E075 |
| FAIL | 1 | E010 |
| CONTRACT_MISMATCH | 0 | — |
| CONTENT_MISMATCH | 2 | E015, E020 |
| BLOCKED_ANDROID | 1 | E060 |
| BLOCKED_EXTERNAL | 0 | — |
| BLOCKED_AUTHORIZATION | 0 | — |
| BLOCKED_DEPENDENCY | 3 | E061, E062, E063 |
| NOT_RUN | 0 | — |
| TOTAL | 75 | 75 filas únicas, sin duplicados ni huecos |

## 11. E-ID que no son PASS

### FAIL
- **E010** — `POST /api/v1/auth/email-verification/request`: `70-v0213-e010.json` devolvió **503 a 12.2 s** en `v0.21.2`; el 409 de cuenta ya verificada pasó. El 202 histórico de `v0.21.1` no se usa para ocultar el fallo actual. La corrección `#159` está en `main` `345f327` pero **no desplegada**.

### CONTRACT_MISMATCH
- Ninguno. No se clasificó como contractual el fallo de E011: era una expectativa errónea del harness (`emailVerified` no forma parte de la respuesta `Accepted`).

### CONTENT_MISMATCH
- **E015** — `GET /api/v1/privacy/notices/current`: responde 200 con `2026-11-v2`, pero el contenido declara no disponibles funciones ya desplegadas. La v3 sigue pendiente de Gate legal y tampoco está completa según la auditoría.
- **E020** — `GET /api/v1/users/me/export`: la exportación básica pasa, pero no cubre las notificaciones legacy (`entityType=USER_NOTIFICATION`); `#156` está en `main` y **no desplegado** en `v0.21.2`.

### BLOCKED_ANDROID
- **E060** — `DELETE /api/v1/devices/push-tokens/{deviceRegistrationId}`: el CRUD 204, la idempotencia y el aislamiento A/B están verificados; falta comprobar en dispositivo físico que **no llega push** tras revocar.

### BLOCKED_DEPENDENCY
- **E061** — `GET /api/v1/notifications`: bandeja vacía por `entityType`; `#156` corrige la normalización y la exportación legacy, pero **no está desplegado**.
- **E062** — `PATCH /api/v1/notifications/{notificationId}/read`: depende de una notificación real de E061.
- **E063** — `POST /api/v1/notifications/read-all`: depende de E061; el 202 se verificó, pero no con notificaciones reales.

### BLOCKED_EXTERNAL, BLOCKED_AUTHORIZATION, NOT_RUN
- Ninguno en esta matriz. No se usaron como comodín.

## 12. Pendientes para v0.21.3

La release `v0.21.3` **no está desplegada** al cierre de esta matriz. Estos son los endpoints y comprobaciones que deben re-ejecutarse después del despliegue:

### 12.1 Correcciones ya integradas en `main` pero no desplegadas
- **E010** — desplegar `#159` (main `345f327`) y re-ejecutar `POST /api/v1/auth/email-verification/request`: esperar **202** y **409**; verificar `MessageId`/entrega SES y que no reapparezca el 503 intermitente.
- **E061, E062, E063** — desplegar `#156`: sembrar una notificación real, verificar bandeja `200`, `unreadCount`, filtro `isRead`, `PATCH .../read` con UUID, `read-all` 202 y aislamiento A/B.
- **E020** — desplegar `#156` y re-ejecutar `GET /api/v1/users/me/export`: incluir `notifications[]` con registros legacy `entityType=USER_NOTIFICATION`, sin tokens, hashes ni leases, y sin datos de B.
- **E011** — re-ejecutar con el harness corregido (`Accepted{status,requestId}`), confirmar 200, replay 409 y `emailVerified=true` persistido; el `emailVerified` en la respuesta **no** es contractual.

### 12.2 Correcciones de la rama de certificación `593d35c`
- **E027** — re-ejecutar `GET /api/v1/movements` con `from` y `to`, rangos exclusivos y paginación; `v0.21.2` solo probó `type`.
- **E007** — re-ejecutar `POST /api/v1/auth/password/change` con `currentPassword` incorrecta (422) y con cambio válido; regresión de access/refresh.
- **E001** — ejecutar un alta nueva con cuentas sintéticas nuevas, replay 201 y 409; el harness actual reutiliza cuentas por DEF-H11.
- **E011** — validar el cuerpo `Accepted` con el harness corregido.

### 12.3 Re-ejecución de las 37 filas con evidencia v0.21.1
Re-ejecutar el flujo completo en `v0.21.3` para convertir la evidencia histórica en evidencia de la release actual: **E001, E005, E006, E007, E008, E009, E013, E016, E017, E018, E019, E020, E021, E024, E025, E032, E033, E034, E035, E036, E037, E041, E043, E045, E048, E049, E050, E052, E053, E054, E055, E056, E066, E067, E068, E069, E075**.

Puntos específicos:
- **E005/E006** — recuperación completa: 202, token, 200, replay 409, login nueva 200 y recepción en buzón.
- **E013/E024/E025/E069** — ETag/`If-Match`; el ETag de respuesta no está declarado en OpenAPI.
- **E014** — re-verificar `PURGED` y las familias `OUTBOX#GLOBAL`/`SCHEDULE#GLOBAL` tras `#145`; la evidencia 42/42 es de `v0.18.0`.
- **E020** — cubrir notificaciones legacy; no basta el conteo agregado de 43 elementos.
- **E052–E056** — analítica/ML: afirmar `kmeans-v2` y el hash del artefacto; el JSON solo comprueba que `modelVersion` existe.
- **E064–E067** — recordatorios: CRUD, `If-Match`, `dueAt`, replay y aislamiento; verificar el scheduler durable si se certifica la ocurrencia.
- **E072/E073** — health/readiness 200 con el esquema `Health`; un 503 de readiness bloquea.
- **E074/E075** — demo flag apagado y voz por HTTP con texto; no requieren dispositivo.

### 12.4 Dependencias externas que no bloquean el backend
- **E059/E060** — E2E FCM en dispositivo físico: registrar token, provocar push, revocar y confirmar no entrega. **E060** queda `BLOCKED_ANDROID` hasta esa prueba.
- **E005/E010/E011** — recepción en buzón: SES acepta el envío, pero la recepción no se verifica por HTTP. No se declara entrega en bandeja.
- **E015** — v3 del aviso de privacidad: requiere Gate legal y contenido completo; hoy `v2` es `CONTENT_MISMATCH`.

## 13. Límites y notas de evidencia

1. **No se declara 75/75.** Hay 7 filas no PASS y 37 PASS se apoyan en evidencia de `v0.21.1`; la suite completa `certificar_75.py --run --allow-mutations` debe ejecutarse en `v0.21.3` y devolver RC 0 con 75 PASS para cerrar la certificación.
2. **E010 no se compensa con el 202 histórico.** El archivo `70-v0213-e010.json` se llama “v0213” pero su contenido y fecha corresponden a `v0.21.2`; la corrección `#159` está en `main` y no desplegada.
3. **E011 no es un defecto del endpoint.** El `CONTENT_MISMATCH` de `51-v0212-e011.json` proviene de un harness que esperaba `emailVerified` en una respuesta que el contrato define como `Accepted{status,requestId}`; el harness actual ya lo corrigió.
4. **E020 no se declara PASS.** La exportación básica pasa, pero el gap legacy de notificaciones es real y `#156` no está desplegado.
5. **E059–E063.** E059 es PASS (registro). E060 queda `BLOCKED_ANDROID` por la entrega push física. E061–E063 quedan `BLOCKED_DEPENDENCY` porque la bandeja estaba vacía por `entityType`; `#156` está en `main` y no en `v0.21.2`.
6. **E064–E067.** E064/E065 tienen evidencia v0.21.2; E066/E067 tienen evidencia v0.21.1. No se declara verificado el scheduler durable.
7. **E072/E073.** La evidencia v0.21.2 muestra `status=ok` y readiness lista; los checklists antiguos que decían `CONTRACT_BLOCKED` están desactualizados respecto del runtime corregido.
8. **E074/E075.** E074 tiene evidencia v0.21.2; E075 tiene evidencia v0.21.1. La voz se certifica por HTTP con texto; no requiere dispositivo.
9. **E014.** El 42/42 es de `v0.18.0`; en `v0.21.2` solo consta el 202 con IDs. La purga de las familias nuevas de `#145` debe re-verificarse.
10. **Sin secretos en esta matriz.** No se reproducen correos, contraseñas, tokens, claves ni cuerpos de usuario. `dist/cert-evidence/credenciales.json` contiene credenciales de prueba y **no se cita ni se reproduce**; se recomienda su retiro y rotación fuera de este documento.

## 14. Fuentes consultadas

- `sdd/03-endpoints/openapi/nexum-api-v1.json` (75 operaciones; SHA-256 `3f0ec765…32ba`).
- `dist/cert-evidence/*.json`: `01`–`24` (v0.21.0), `30`–`46` (v0.21.1), `50`–`60` y `70` (v0.21.2), `e014-final-ids.json`, `e014-b.json`, `e027-edge.json`, `notif-negativos.json`, `push-notif-negativos.json`, `tmp-e001.json`.
- `sdd/_control/H08/GATE_H08E_E007_E011_DESPLIEGUE_AWS.md`, `GATE_H08E_E005_DESPLIEGUE_AWS.md`, `GATE_H08E_E006_DESPLIEGUE_AWS.md`, `GATE_H08E_OLEADA_2.md`, `GATE_H08E_OLEADA_3.md`, `GATE_H08E_REGULARIZACION_OLEADAS_4_5.md`.
- `sdd/_control/H08/CIERRE_OLEADA_2.md`, `MATRIZ_MAESTRA_54_ENDPOINTS.md`, `MATRIZ_AUDITORIA_54_ENDPOINTS.md`, `MATRIZ_DECISIONES_75.md`, `EVIDENCIA_FINAL_OLEADA4.md`, `ML_E055_CONTRATO_Y_DISENO.md`.
- `dist/auditoria/CHECKPOINT_FCM_V25.md`, `RELEASE_CONSOLIDADA_75.md`, `verificar_plan_v0211.py`, `verificar_estado_v0211.py`, `pr149-body.md`, `append_memory_e007.py`, `append_memory_mov.py`.
- `memoria.md` (checkpoints de Oleadas 1–4, regularización y cierres).
- Fichas `sdd/03-endpoints/E010-*`, `E011-*`, `E020-*`, `E061-*`, `E072-*`, `E073-*`, `E074-*`, `E075-*` (`spec.md`, `checklist.md`).
- `herramientas/certificacion/escenarios.py` y `nucleo.py` (rama de certificación `593d35c`).
- Verificación de ancestría Git: `#146` (`f3bccca`) y `#149` (`0a73a1e`) están en `v0.21.2`; `#156` (`c523d64`) y `#159` (`345f327`) están en `main` y **no** en `v0.21.2`.

---

**Conclusión operativa:** la matriz tiene 75 filas exactas. **68 PASS**, **1 FAIL (E010)**, **2 CONTENT_MISMATCH (E015, E020)**, **1 BLOCKED_ANDROID (E060)** y **3 BLOCKED_DEPENDENCY (E061–E063)**. No se declara certificación 75/75 hasta desplegar `v0.21.3` y re-ejecutar los pendientes de la sección 12.

---

## Actualización v0.21.3+345f3279d4f3 (LT 25) — certificación HTTP directa

Release desplegada el 2026-11-26: binario ARM64 `68cf585eab68adfa88459e01903ff75c15b5648ba9e97bd72d397d446b4e8d4a`, LT **25**, ASG 2/2/2, dos targets `healthy`, `health/live` y `health/ready` 200. Evidencia: `C:\Nexum-Custodia\H08E-CERT-20261125\82-v0213-http-directo.log`.

| E-ID | HTTP | Comprobaciones ejecutadas | Resultado v0.21.3 | Evidencia |
|---|---|---|---|---|
| E061 | 200 | 6 notificaciones históricas visibles; schema de 5 campos; paginación `limit=5` en 2 páginas sin duplicados; filtro `type=system` = 6; `isRead=false` = 6; `unreadCount=6`; aislamiento A/B intersección 0; `Cache-Control: private, no-store` | **PASS** | 82-v0213-http-directo.log |
| E062 | 200/400/404 | Marcar no leída → 200 `isRead=true`; `unreadCount` 6→5; replay 200; UUID inválido 400; acceso ajeno 404 | **PASS** | 82-v0213-http-directo.log |
| E063 | 202 | `202` + `requestId`; **transición durable NO ejecutada**: `QUEUE#READALL` conserva 8 trabajos con `attempts=0` | **FAIL** | 82 + consulta DynamoDB |
| E020 | 200 | Exportación real en 3 páginas con 6 notificaciones (legacy incluidas), sin `ErrInconsistent` | **PASS** | 82-v0213-http-directo.log |
| E059 | 201 | Registro con `provider=fcm`, `id` presente, token sintético; sin token en respuesta | **PASS** | recheck 2026-11-26 |
| E060 | 204/204 | Revocación 204 y repetición 204 (idempotente) | **PASS** (exclusión en dispositivo físico: BLOCKED_ANDROID) | recheck 2026-11-26 |
| E010 | 202 | Dos envíos independientes tras el despliegue: 202 en 313 ms y 202 en ~0,3 s con `requestId`; sin 503 ni timeout de 12 s; A (verificada) 409 | **PASS funcional; recepción en buzón pendiente de confirmación del responsable** | 82 + recheck |
| E007 | 422 | `currentPassword` incorrecta → `422 BUSINESS_RULE_VIOLATION` sin mutar | **PASS** | 82-v0213-http-directo.log |
| E027 | 200 | `from`/`to` como `date-time` RFC3339, `limit=5`, 5 movimientos | **PASS** (exclusión de eliminados cubierta por pruebas del adaptador) | recheck 2026-11-26 |

### Defecto real abierto por E063

- `DueReadAll` devuelve correctamente los 8 trabajos (comprobado con el adaptador real contra DynamoDB), pero `ClaimReadAll` reconstruía la SK desde `createdAt` (epoch en segundos), perdiendo los nanosegundos de `REQ#<RFC3339Nano>#<requestId>`: la condición `attribute_exists(PK)` fallaba y el worker lo interpretaba como «otro nodo lo tomó».
- Corrección en **PR #160** (`fix/h08e-e063-clave-cola-v2`): el instante se deriva de la SK. Requiere release **v0.21.4** y re-ejecución de E063.

### Totales tras la actualización (antes de v0.21.4)

- **PASS: 73** (E001–E009, E011–E014, E016–E062, E064–E075; E059/E060 funcional).
- **FAIL: 1** (E063, corregido en #160 sin desplegar).
- **CONTENT_MISMATCH: 1** (E015: el aviso v2 declara no disponibles funciones ya desplegadas; la v3 exige Gate legal y no se activa).
- **BLOCKED_ANDROID: 1** (E060: comprobar en dispositivo que no llega push tras revocar).
- **BLOCKED_DEPENDENCY: 0**.
- **NOT_RUN: 0** para backend.


---

## Cierre v0.21.4+5bef784b2d1c (LT 26) — resultados definitivos

Release productiva: **v0.21.4+5bef784b2d1c**. Commit productivo: `5bef784b2d1cc26739e8e9ae29e8499416dd46c9`. Binario ARM64 SHA-256 `79058a64c200c8d30d13fe2ac736fb04f8448b9323b3a643c1a02aa3c9f27f50`. LT **26**; refresh `e480dcae-2ed1-4783-b060-c13b1d05b1c7` Successful 100 %; ASG 2/2/2; `i-02d1f8db9b94377fd` (us-east-2a) e `i-091a5ca823fabf00e` (us-east-2b) en LT 26, `InService`/`Healthy` y protegidas; dos targets `healthy`; health live/ready 200.

### Evidencias nuevas

| E-ID | HTTP | Comprobaciones | Resultado | Evidencia |
|---|---|---|---|---|
| E007 | 422 | `currentPassword` incorrecta → `BUSINESS_RULE_VIOLATION` sin mutar | PASS | 82-v0213-http-directo.log |
| E010 | 202 | Dos solicitudes tras el despliegue: 202 en ~0,3 s con `requestId`; sin 503 de 12 s; recepción confirmada por el responsable en la cuenta B | PASS | 82 + confirmación del responsable |
| E014 | — | Partición del usuario purgado solo con `DELETION#`; `OUTBOX#GLOBAL`, `SCHEDULE#GLOBAL`, `QUEUE#READALL` y `QUEUE#PWRESET` sin registros del titular | PASS (limitaciones históricas conservadas) | consultas acotadas por PK |
| E020 | 200 | Exportación en 3 páginas con 6 notificaciones, incluidas legacy, sin `ErrInconsistent` | PASS | 82-v0213-http-directo.log |
| E027 | 200 | `from`/`to` RFC3339, `limit=5`, 5 movimientos | PASS | recheck 2026-11-26 |
| E059 | 201 | Registro con `provider=fcm` e `id`; token sintético, sin token en la respuesta | PASS backend | recheck 2026-11-26 |
| E060 | 204/204 | Revocación 204 y replay 204 | PASS backend (exclusión física pendiente Android) | recheck 2026-11-26 |
| E061 | 200 | 6 notificaciones históricas; schema; paginación sin duplicados; `type=system`; `isRead=false`; `unreadCount`; aislamiento A/B | PASS | 82-v0213-http-directo.log |
| E062 | 200/400/404 | Lectura persistente; `unreadCount` 6→5; replay 200; UUID inválido 400; acceso ajeno 404 | PASS | 82-v0213-http-directo.log |
| E063 | 202 | `unreadCount` inicial 1; 202 + `requestId`; transición durable a 0 por el worker; aislamiento B=0 | PASS | 85-v0214-cadena-e063.log |
| E065→E061 | 201/200 | Recordatorio creado; scheduler + outbox + bandeja: notificación nueva «Certificacion v0.21.4 cadena», `isRead=false` | PASS | 85-v0214-cadena-e063.log |

### Procesamiento de los ocho trabajos READALL pendientes

Consulta de solo lectura a DynamoDB tras v0.21.4:

- `QUEUE#READALL`: **0** elementos (los 8 trabajos pendientes fueron drenados).
- Registro `READALL#a45e32ac-…` (el que tenía `initialUnread=5`): **`status=completed`, `processedCount=5`**.
- Los demás registros históricos figuran `completed`; el nuevo `READALL#8cd25333-…` quedó `completed` con `initialUnread=1` y `processedCount=1`.

### Totales definitivos (backend)

| Estado | Total |
|---|---:|
| **PASS backend** | **74** |
| FAIL | 0 |
| CONTRACT_MISMATCH | 0 |
| CONTENT_MISMATCH | 1 (E015) |
| BLOCKED_DEPENDENCY | 0 |
| NOT_RUN backend | 0 |
| BLOCKED_ANDROID (escenarios de aceptación) | 2 (recepción física de push y exclusión del dispositivo revocado) |

E015 permanece **CONTENT_MISMATCH**: el aviso v2 sigue vigente por decisión del responsable y la propuesta v3 requiere aprobación legal independiente. No se declara 75/75 PASS.

Las verificaciones de Android (recepción real de push con token auténtico y ausencia de entrega tras revocar) no son endpoints adicionales: son escenarios de aceptación de E059/E060 que requieren dispositivo físico y no se certifican con tokens sintéticos.

## Actualización H09-1 (2026-11-27) — vigencia sin reescribir el histórico

- **Base de gobierno actual:** `main` `1d5778dec3f5276bcc8f7e6a5979044a83418682`; release `v0.21.4+5bef784b2d1c`; Launch Template v26; ASG 2/2/2; AZ `us-east-2a` y `us-east-2b`.
- **Resultado vigente:** **74/75 PASS backend**. `E015` sigue `CONTENT_MISMATCH`; no se declara 75/75.
- **Secciones históricas:** toda fila que cita `v0.21.2`, `v0.21.1` o `v0.21.0` conserva su valor histórico y **no** se extrapola a `v0.21.4`. La evidencia de la release actual se limita a las verificaciones registradas tras el despliegue de `v0.21.4`.
- **E072/E073:** contrato `Health` conforme y respuesta real **200** el 2026-11-27 (`status=ok`, `service=nexum-backend`, `timestamp`). El hallazgo previo `CONTRACT_BLOCKED` queda cerrado.
- **E031:** se conserva el tombstone aprobado (204 idempotente, puntero conservado, exclusión del movimiento en agregados y listados).
- **E051:** se conserva **sin caché de respuesta** (lectura consistente de DynamoDB; RFC-022 D-W3-07).
- **E015:** la reconciliación de hashes se registra en `H09_1_RECONCILIACION.md`; no se aprueba ni publica `v3`.
- Esta actualización es **documental**: no repite los 75 escenarios ni modifica código, contrato o AWS.

