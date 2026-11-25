# Matriz de decisiones — 75 operaciones (H08-E)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/MATRIZ_DECISIONES_75.md` |
| Versión | 1.0.0 |
| Fecha | 2026-11-24 |
| Base | `main` `f3bccca8b2437dc154bedbef257a3cabc7620844` (rama `fix/h08e-deuda-historica`) |
| Alcance | Índice de **estado de decisión** por operación. No es una matriz de evidencia |
| Autoridad de contrato | `sdd/03-endpoints/openapi/nexum-api-v1.yaml` + espejo `.json` |

> **No duplica** `MATRIZ_AUDITORIA_54_ENDPOINTS.md` ni `MATRIZ_MAESTRA_54_ENDPOINTS.md`. Aquellas
> registran lectura de fichas, PR, release, HTTPS y evidencia. Esta solo responde: **¿hay una
> decisión vigente para lo que está abierto, o falta una frase humana?**

## 1. Estados

| Estado | Significado |
|---|---|
| **RESUELTA** | Existe una decisión vigente y aprobada que cubre la discrepancia conocida. Puede quedar aplicación mecánica (editar texto, recalcular manifiesto) o implementación ya autorizada; no requiere una frase nueva |
| **PENDIENTE** | Falta una frase humana, o la decisión vigente no cubre el caso, o hay una corrección de comportamiento publicada que aún no está autorizada/verificada |
| **NO_APLICA** | No existe ni se requiere una decisión propia del endpoint: su comportamiento queda fijado por el contrato y por decisiones de otras operaciones |

## 2. Instrumentos citados (leyenda)

| Clave | Documento |
|---|---|
| G-E001 | `GATE_H08E_E001.md` |
| G-E003 / G-E004 / G-E006 | `GATE_H08E_E003.md`, `GATE_H08E_E004.md`, `GATE_H08E_E006.md` |
| G-E005 | `GATE_H08E_E005_A10.md` + `GATE_H08E_E005_DESPLIEGUE_AWS.md` |
| G-E7-11 | `GATE_H08E_E007_E011_OLEADA.md` + `GATE_H08E_E007_E011_DESPLIEGUE_AWS.md` |
| G-O1 | `GATE_H08E_OLEADA_1.md` |
| G-O2 | `GATE_H08E_OLEADA_2.md` |
| G-O3 | `GATE_H08E_OLEADA_3.md` |
| G-REG | `GATE_H08E_REGULARIZACION_OLEADAS_4_5.md` |
| R-01…R-26 | `RFC-001` … `RFC-026` |
| RECON | `RECONCILIACION_CONTRACTUAL_E002_E008_E009_E051.md` |
| DEC-E031 | `DECISION_E031_BORRADO.md` (propuesta) |

## 3. Matriz

| E-ID | Método y ruta | Estado | Instrumento | Frase / referencia | Nota |
|---|---|---|---|---|---|
| E001 | `POST /api/v1/auth/register` | **PENDIENTE** | G-E001 + R-12 + parche OpenAPI | `APRUEBO_RECONCILIACION_E002_RESERVADOS_409_422_Y_PARQUE_OPENAPI` (propuesta, cubre E001/E002) | OpenAPI declara `401` en ruta pública y el handler no lo emite. Misma clase que E002: conservar como reservado con anotación aditiva |
| E002 | `POST /api/v1/auth/login` | **PENDIENTE** | R-13 + R-25 + RECON D-E002-01 | `APRUEBO_RECONCILIACION_E002_RESERVADOS_409_422_Y_PARQUE_OPENAPI` | `409`/`422` declarados y no emitidos; se propone conservarlos como reservados. `401` ya enmendado por R-25 |
| E003 | `POST /api/v1/auth/refresh` | RESUELTA | R-14 + G-E7-11 + hotfix #79 | `APRUEBO_RFC_014_E003_REFRESH` | Queda residuo documental `SESSIONHASH` en `spec/plan` (T-DOC-01). Contrato y runtime alineados |
| E004 | `POST /api/v1/auth/logout` | RESUELTA | G-E004 + R-15 | `APRUEBO_ENMIENDA_DOCUMENTAL_E004_PLAN_PASO3` | `plan.md` corregido; doc dice Redis y el código decide solo DynamoDB (nota de vigencia) |
| E005 | `POST /api/v1/auth/password/forgot` | RESUELTA | R-16 + R-25 + G-E005 | `APRUEBO_CORRECCIONES_TRANSVERSALES_CONFIRMADAS_54_ENDPOINTS` | `413` añadido al contrato. Evidencia SES `PARTIAL_SES` (no es decisión) |
| E006 | `POST /api/v1/auth/password/reset` | RESUELTA | R-18 + G-E006 | `APRUEBO_RFC_018_A6_EPOCA_REVOCACION` | Época de revocación A6 aprobada e implementada |
| E007 | `POST /api/v1/auth/password/change` | RESUELTA | R-19 + G-E7-11 | `APRUEBO_RFC_019_OLEADA_E007_E011` | Reutiliza A6 |
| E008 | `GET /api/v1/auth/sessions` | **PENDIENTE** | G-O1 D-429 + RECON §3.1 | `APRUEBO_CUOTA_D429_EN_E008_E009_Y_CODIGO_RATE_LIMITED` (propuesta) | `429` declarado y no emitido; implementación **en curso** por el orquestador. La omisión por «Redis no configurado» no aplica en producción |
| E009 | `DELETE /api/v1/auth/sessions/{sessionId}` | **PENDIENTE** | G-O1 D-429 + RECON §4.1 | misma frase que E008 | `429` declarado y no emitido; operación destructiva: la cuota es prioritaria |
| E010 | `POST /api/v1/auth/email-verification/request` | RESUELTA | R-19 + G-E7-11 + hotfix #100 | `APRUEBO_RFC_019_OLEADA_E007_E011` | Deadline SES de 8 s ya desplegado. Evidencia de recepción `PARTIAL_SES` |
| E011 | `POST /api/v1/auth/email-verification/confirm` | **PENDIENTE** | G-E7-11 + auditoría | `APRUEBO_DECISION_E011_TOKEN_USUARIO_CORREO` (propuesta) | La spec exige token ligado a **usuario/correo**; el runtime solo lo liga a `userId`. Decidir: corregir runtime o enmendar spec |
| E012 | `GET /api/v1/users/me` | RESUELTA | G-O1 | `APRUEBO_GATE_OLEADA_1_RFC019_E012_E013_E015_E016_E017_E018_E019_E022_E023_E024_E025_E068_E069_E070_E071` | — |
| E013 | `PATCH /api/v1/users/me` | RESUELTA | G-O1 D-E013 | misma frase de Oleada 1 | ETag `"v<version>"` decidido; su declaración aditiva en OpenAPI es T-ETAG |
| E014 | `DELETE /api/v1/users/me` | RESUELTA | G-O2 + R-23 + G-REG #8 | `APRUEBO_TRANSVERSAL_E014_E020` | Purga verificable `PURGED`; sin cambio de decisión |
| E015 | `GET /api/v1/privacy/notices/current` | **PENDIENTE** | G-REG §5 (excluye aprobación legal) + `AVISO_PRIVACIDAD_2026_11_V3_PROPUESTA.md` | `APRUEBO_AVISO_PRIVACIDAD_E015_2026_11_V3_SHA256_1CEC4609FF2F3FC052D3DEB29271BA8AAED322D1EC97AE9757E222CC8C0563B9` (propuesta) | Aviso vigente `2026-11-v2` declara no disponibles funciones ya publicadas. v3 nueva propuesta, no publicada |
| E016 | `GET /api/v1/users/me/consents` | RESUELTA | G-O1 | frase de Oleada 1 | — |
| E017 | `POST /api/v1/users/me/consents` | RESUELTA | G-O1 | frase de Oleada 1 | — |
| E018 | `GET /api/v1/users/me/onboarding` | RESUELTA | G-O1 | frase de Oleada 1 | — |
| E019 | `PATCH /api/v1/users/me/onboarding` | RESUELTA | G-O1 | frase de Oleada 1 | Evidencia funcional individual no localizada (auditoría) |
| E020 | `GET /api/v1/users/me/export` | RESUELTA | G-O3 + R-22 + G-REG #8 | `APRUEBO_TRANSVERSAL_E014_E020` | Familias nuevas en exportación; sin operaciones destructivas |
| E021 | `GET /api/v1/bootstrap` | NO_APLICA | G-REG | `APRUEBO_REGULARIZACION_OLEADAS_4_5` | Agregado de lectura de datos ya decididos (E012/E026); sin decisión propia |
| E022 | `GET /api/v1/catalogs/categories` | NO_APLICA | G-O1 + catálogo A-020 | frase de Oleada 1 | Catálogo estático; sin decisión abierta |
| E023 | `GET /api/v1/catalogs/frequencies` | RESUELTA | R-20 + R-21 + G-O1 | `APRUEBO_RFC_021_FRECUENCIAS_RECURRENTES` | 7 canónicas emitidas; 9 aceptadas en entradas (R-21) |
| E024 | `GET /api/v1/income` | RESUELTA | R-20 + G-O1 D-E024 | frase de Oleada 1 | ETag decidido; declaración aditiva T-ETAG |
| E025 | `PUT /api/v1/income` | RESUELTA | R-20 + G-O1 D-E025 | frase de Oleada 1 | D-E025 exige `> 0`; el patrón del contrato aún admite `0.00` (T-PATRON) |
| E026 | `GET /api/v1/financial-summary` | RESUELTA | G-O3 + R-22 (alternativa B) | `APRUEBO_RFC_022_CONTRATOS_OLEADA_3_V1_2_1` | Sin `availableEstimated`; `net` realizado |
| E027 | `GET /api/v1/movements` | RESUELTA | G-O2 | `APRUEBO_GATE_OLEADA_2_RFC019_E014_E027_E028_E029_E030_E031_E032_E033_E034_E035_E036_E038_E039_E040_E041` | Cursor firmado; sin `Scan` |
| E028 | `POST /api/v1/movements` | RESUELTA | G-O2 | frase de Oleada 2 | Idempotencia por usuario |
| E029 | `GET /api/v1/movements/{movementId}` | RESUELTA | G-O2 | frase de Oleada 2 | Puntero O(1) |
| E030 | `PATCH /api/v1/movements/{movementId}` | RESUELTA | G-O2 + R-22 T-1 | `APRUEBO_CAMBIO_TRANSVERSAL_E030_MOVIMIENTO_APORTACION` | `409` si el movimiento tiene aportación; no-op `{}` → `200` (decisión vigente) |
| E031 | `DELETE /api/v1/movements/{movementId}` | **PENDIENTE** | DEC-E031 + auditoría | `APRUEBO_DECISION_E031_BORRADO_LOGICO_TOMBSTONE` (propuesta) | Runtime borra físicamente; spec y A-060 exigen soft delete/tombstone. Sin decisión, sigue no conforme |
| E032 | `GET /api/v1/recurring-payments` | RESUELTA | G-O2 | frase de Oleada 2 | Proyección de ítems heredados D-E032-01 |
| E033 | `POST /api/v1/recurring-payments` | RESUELTA | R-21 + G-O2 | `APRUEBO_RFC_021_FRECUENCIAS_RECURRENTES` | — |
| E034 | `GET /api/v1/recurring-payments/{recurringPaymentId}` | RESUELTA | G-O2 | frase de Oleada 2 | — |
| E035 | `PATCH /api/v1/recurring-payments/{recurringPaymentId}` | RESUELTA | G-O2 + R-21 | frase de Oleada 2 | `{}` → `200` no-op (decisión vigente) |
| E036 | `DELETE /api/v1/recurring-payments/{recurringPaymentId}` | RESUELTA | G-O2 | frase de Oleada 2 | — |
| E037 | `POST /api/v1/recurring-payments/{recurringPaymentId}/payments` | RESUELTA | R-22 D-W3-01/D-W3-02 + G-O3 | `APRUEBO_RFC_022_CONTRATOS_OLEADA_3_V1_2_1` | Categorías `otros`/`ingreso`/`ahorro`; duplicado por fecha local |
| E038 | `GET /api/v1/goals` | RESUELTA | G-O2 + R-22 | `APRUEBO_RFC_022_CONTRATOS_OLEADA_3_V1_2_1` | `totals` sobre la colección completa |
| E039 | `POST /api/v1/goals` | RESUELTA | G-O2 | frase de Oleada 2 | — |
| E040 | `GET /api/v1/goals/{goalId}` | RESUELTA | G-O2 + R-16 A7 | `APRUEBO_ENMIENDA_A7_E040` | `openingSaved` añadido |
| E041 | `PATCH /api/v1/goals/{goalId}` | **PENDIENTE** | G-O2 + auditoría | `APRUEBO_DECISION_E041_PATCH_VACIO` (propuesta) | `{}` devuelve `400` aunque el contrato lo admite; decidir contrato (`minProperties: 1`) o runtime (`200` no-op como E030/E035) |
| E042 | `DELETE /api/v1/goals/{goalId}` | RESUELTA | R-22 D-W3-03 + G-O3 | `APRUEBO_RFC_022_CONTRATOS_OLEADA_3_V1_2_1` | Tombstone de meta; 409 con aportaciones activas |
| E043 | `POST /api/v1/goal-contributions/preview` | RESUELTA | R-22 §6 + G-O3 | `APRUEBO_RFC_022_CONTRATOS_OLEADA_3_V1_2_1` | `Σ allocations == amount` |
| E044 | `POST /api/v1/goal-contributions` | RESUELTA | R-22 D-W3-04 | frase R-22 | Un movimiento de ahorro por total |
| E045 | `GET /api/v1/goal-contributions` | RESUELTA | R-22 + G-O3 | frase R-22 | — |
| E046 | `GET /api/v1/goal-contributions/{contributionId}` | RESUELTA | R-22 + G-O3 | frase R-22 | — |
| E047 | `POST /api/v1/goal-contributions/{contributionId}/reverse` | RESUELTA | R-22 D-W3-05 | frase R-22 | Reversión sin borrar movimiento |
| E048 | `GET /api/v1/budgets` | RESUELTA | R-22 §3.1 + G-O3 | `APRUEBO_RFC_022_CONTRATOS_OLEADA_3_V1_2_1` | `BUDGETSET#`; ETag declarado |
| E049 | `PUT /api/v1/budgets` | RESUELTA | R-22 §3.1 + G-O3 | frase R-22 | Conjunto completo ≤50; ETag |
| E050 | `GET /api/v1/budgets/usage` | RESUELTA | R-22 §8.1 + G-O3 | frase R-22 | Usa movimientos `expense`; afectado por DEC-E031 |
| E051 | `GET /api/v1/dashboard` | RESUELTA | R-22 **D-W3-07** + **R-26** | `APRUEBO_ENMIENDA_E051_SIN_CACHE_RFC_022` (**concedida**) | Sin caché de respuesta. Enmienda documental pendiente de aplicar (RFC-026) |
| E052 | `GET /api/v1/analytics/insights` | RESUELTA | R-16 + G-REG #3 | `APRUEBO_CORRECCION_E052_PERIOD` | Normalización de `period` |
| E053 | `GET /api/v1/analytics/categories` | RESUELTA | R-16 + G-REG | `APRUEBO_RFC_016_CONTRATOS_E005_E075` | Importes en centavos exactos |
| E054 | `GET /api/v1/analytics/spending-patterns` | RESUELTA | R-16 + G-REG | frase R-16 | Suficiencia de historial |
| E055 | `GET /api/v1/analytics/recommendations` | RESUELTA | R-16 §6 + `ML_E055_CONTRATO_Y_DISENO.md` + G-REG #9 | `APRUEBO_CORRECCION_ML_155` | `hasEnoughData` decidido. Aprobación **final del modelo ML** sigue pendiente (no es decisión de contrato) |
| E056 | `GET /api/v1/analytics/savings-streak` | RESUELTA | R-16 + G-REG | frase R-16 | Días locales IANA |
| E057 | `GET /api/v1/notification-preferences` | RESUELTA | R-16 §17.6 + G-REG | `APRUEBO_RFC_016_CONTRATOS_E005_E075` | Preferencias granulares + `quietHours` |
| E058 | `PUT /api/v1/notification-preferences` | RESUELTA | R-16 A3 (`APRUEBO_ENMIENDA_A3_E058`) | `APRUEBO_ENMIENDA_A3_E058` | Contrato exacto en 17.6 |
| E059 | `POST /api/v1/devices/push-tokens` | RESUELTA | R-16 A8 + G-REG | `APRUEBO_ENMIENDA_A8_ADITIVOS` | Token cifrado; **envío FCM** condicionado a FinOps/egreso (no es decisión del contrato) |
| E060 | `DELETE /api/v1/devices/push-tokens/{deviceRegistrationId}` | RESUELTA | R-16 A8 + G-REG | frase A8 | Baja borra token cifrado |
| E061 | `GET /api/v1/notifications` | RESUELTA | R-16 + G-REG | `APRUEBO_RFC_016_CONTRATOS_E005_E075` | Solo destinatario; sin PII en push |
| E062 | `PATCH /api/v1/notifications/{notificationId}/read` | RESUELTA | G-REG #4 (**otra unidad**: U1) | `APRUEBO_DECISION_IDS_NOTIFICACION_E062` | UUID contractuales deterministas; sin relajar contrato. Implementación en U1 |
| E063 | `POST /api/v1/notifications/read-all` | RESUELTA | R-16 + G-REG | frase R-16 | `202` sin conteo final |
| E064 | `GET /api/v1/reminders` | RESUELTA | G-REG #5 + #7 | `APRUEBO_CORRECCION_REMINDERS_SK_E065_E067` | Paginación y scheduler durable |
| E065 | `POST /api/v1/reminders` | RESUELTA | G-REG #5 + R-16 A8 | `APRUEBO_CORRECCION_REMINDERS_SK_E065_E067` | `frequency` opcional |
| E066 | `PATCH /api/v1/reminders/{reminderId}` | RESUELTA | G-REG #5 | misma frase | `If-Match`; reprogramación idempotente |
| E067 | `DELETE /api/v1/reminders/{reminderId}` | RESUELTA | G-REG #5 | misma frase | Soft delete idempotente |
| E068 | `GET /api/v1/users/me/preferences` | RESUELTA | G-O1 D-E068 | frase de Oleada 1 | Materialización perezosa |
| E069 | `PATCH /api/v1/users/me/preferences` | RESUELTA | G-O1 D-E069 + G-REG #15 | `APRUEBO_CORRECCIONES_TRANSVERSALES_CONFIRMADAS_54_ENDPOINTS` | D-E069 exige **rechazar** `UTC`; el runtime lo acepta → corregir runtime (defecto inequívoco). ETag aditivo T-ETAG |
| E070 | `GET /api/v1/help/faq` | **PENDIENTE** | G-REG U6 (**otra unidad**) + auditoría | `APRUEBO_CORRECCION_FAQ_E070` (propuesta) | FAQ afirma edición de correo inexistente, 4 frecuencias y notificaciones no implementadas. Requiere corrección de contenido y Gate propio |
| E071 | `GET /api/v1/education/tips` | RESUELTA | G-O1 D-E070/D-E071 | frase de Oleada 1 | Contenido estático versionado |
| E072 | `GET /api/v1/health/live` | RESUELTA | G-REG #6 + A-090 | `APRUEBO_CONTRATO_HEALTH_E072_E073` | Cuerpo `Health`; liveness sin dependencias |
| E073 | `GET /api/v1/health/ready` | RESUELTA | G-REG #6 + A-090 | misma frase | `503` si no puede servir |
| E074 | `GET /api/v1/demo/bootstrap` | RESUELTA | R-16 + G-REG + implementación | `APRUEBO_REGULARIZACION_OLEADAS_4_5` | `NEXUM_DEMO_ENABLED` por defecto **false**; no inserta datos |
| E075 | `POST /api/v1/voice/parse` | RESUELTA | R-16 §E075 | `APRUEBO_RFC_016_CONTRATOS_E005_E075` | Interpreta **texto**; sin audio ni persistencia |

## 4. Acciones transversales (no son filas de endpoint)

| ID | Acción | Estado | Instrumento / frase | Afecta |
|---|---|---|---|---|
| T-DOC-01 | Eliminar las 3 referencias `SESSIONHASH` restantes de E003 (`spec/plan`) | PENDIENTE | Enmienda documental propia (R-26 no la cubre) | E003 |
| T-ETAG | Declarar `ETag` de respuesta en OpenAPI (hoy el runtime lo emite y el contrato no lo declara) | PENDIENTE | Parche aditivo; `APRUEBO_PARQUE_ETAG_ADITIVO_E013_E024_E025_E069` (propuesta) | E013, E024, E025, E069 |
| T-PATRON | Excluir `0.00` del patrón de `amount` en E025 | PENDIENTE | Mismo parche de contrato; D-E025 ya exige `> 0` | E025 |
| T-429 | Cuota por usuario en rutas autenticadas; `Retry-After` en el contrato; unificar `RATE_LIMITED` | PENDIENTE | `APRUEBO_CUOTA_D429_EN_E008_E009_Y_CODIGO_RATE_LIMITED` (propuesta) | E008, E009 y ~25 endpoints que ya usan el helper |
| T-429-B | El helper `identity_support.go:142` emite `TOO_MANY_REQUESTS` y las rutas de auth emiten `RATE_LIMITED` | PENDIENTE | Incluida en T-429 | Toda la API con 429 |
| T-E031 | Filtros de exclusión de tombstones en E026/E027/E029/E030/E050/E051/E020 y analítica | PENDIENTE | `APRUEBO_DECISION_E031_BORRADO_LOGICO_TOMBSTONE` | 8+ endpoints |

## 5. Recuento

| Estado | Nº | E-IDs |
|---|---|---|
| **PENDIENTE** | **9** | E001, E002, E008, E009, E011, E015, E031, E041, E070 |
| **RESUELTA** | **64** | Resto |
| **NO_APLICA** | **2** | E021, E022 |
| **Total** | **75** | — |

Frases humanas pendientes (no concedidas en esta unidad):

1. `APRUEBO_RECONCILIACION_E002_RESERVADOS_409_422_Y_PARQUE_OPENAPI` (E001/E002).
2. `APRUEBO_CUOTA_D429_EN_E008_E009_Y_CODIGO_RATE_LIMITED` (E008/E009 + T-429).
3. `APRUEBO_DECISION_E011_TOKEN_USUARIO_CORREO` (E011).
4. `APRUEBO_AVISO_PRIVACIDAD_E015_2026_11_V3_SHA256_1CEC4609FF2F3FC052D3DEB29271BA8AAED322D1EC97AE9757E222CC8C0563B9` (E015; hash del artefacto del aviso).
5. `APRUEBO_DECISION_E031_BORRADO_LOGICO_TOMBSTONE` (E031 + T-E031).
6. `APRUEBO_DECISION_E041_PATCH_VACIO` (E041).
7. `APRUEBO_CORRECCION_FAQ_E070` (E070, unidad U6).
8. `APRUEBO_PARQUE_ETAG_ADITIVO_E013_E024_E025_E069` (T-ETAG + T-PATRON).

Concedidas que sostienen filas RESUELTA: `APRUEBO_ENMIENDA_E051_SIN_CACHE_RFC_022` (E051),
`APRUEBO_DECISION_IDS_NOTIFICACION_E062` (E062), `APRUEBO_RFC_022_CONTRATOS_OLEADA_3_V1_2_1`,
`APRUEBO_RFC_021_FRECUENCIAS_RECURRENTES`, `APRUEBO_RFC_016_CONTRATOS_E005_E075` y sus
enmiendas A1/A2/A3/A5/A7/A8, `APRUEBO_RFC_018_A6_EPOCA_REVOCACION`, `APRUEBO_RFC_014_E003_REFRESH`,
`APRUEBO_RFC_013_E002_V602`, `APRUEBO_RFC_019_OLEADA_E007_E011`,
`APRUEBO_CORRECCION_REMINDERS_SK_E065_E067`, `APRUEBO_CONTRATO_HEALTH_E072_E073`,
`APRUEBO_CORRECCION_E052_PERIOD`, `APRUEBO_CORRECCION_ML_155`,
`APRUEBO_TRANSVERSAL_E014_E020`, `APRUEBO_CAMBIO_TRANSVERSAL_E030_MOVIMIENTO_APORTACION`, y las
frases de Gate de Oleada 1/2/3 y de regularización 4/5.

## 6. Cómo se actualiza

1. Al concederse una frase, cambiar el estado de la fila a `RESUELTA`, anotar la fecha y el
   documento donde consta, y retirar la frase de la lista del §5.
2. Al aplicarse una enmienda documental, anotar en la nota «aplicado en `<commit>`» sin cambiar el
   estado.
3. Una decisión nueva que afecte a varios endpoints se registra primero en el §4 y luego se
   refleja en cada fila.
4. Esta matriz **no** sustituye la evidencia de cada endpoint ni la matriz de auditoría; no se
   añaden aquí PR, SHA de release ni resultados HTTPS.

## 7. Trazabilidad

- `sdd/03-endpoints/openapi/nexum-api-v1.yaml` y `.json` (75 operaciones)
- `sdd/_control/H08/RECONCILIACION_CONTRACTUAL_E002_E008_E009_E051.md`
- `sdd/_control/H08/RFC-026-ENMIENDA_E051_CACHE.md`
- `sdd/_control/H08/DECISION_E031_BORRADO.md`
- `sdd/_control/H08/AVISO_PRIVACIDAD_2026_11_V3_PROPUESTA.md`
- `sdd/_control/H08/MATRIZ_AUDITORIA_54_ENDPOINTS.md` (evidencia y hallazgos)
- Gates citados en §2
