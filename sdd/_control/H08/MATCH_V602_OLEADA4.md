# MATCH Auditoría V6.0.2 ↔ Oleada 4 (E052–E063)

| Campo | Valor |
|---|---|
| Rama | `liz-oleada4` |
| Fecha | 2026-11-22 |
| Entrada | `Nexum_V6_0_2_Auditoria_Definitiva_Pantallas_75_Contratos_Verificados.pdf` (126 páginas) |
| Autoridad | `constitution.md` > OpenAPI `sdd/03-endpoints/openapi/nexum-api-v1.{json,yaml}` > fichas `spec/plan/tasks/checklist` > este documento |
| Estado | `MATCH_REVISADO` — documento de control, **no modifica contratos aprobados** |

> El propio PDF declara: *"Documento propuesto / no contrato SDD aprobado"* y en cada ficha *"PENDIENTE CONTRASTE SDD/OPENAPI BACKEND"*. Por tanto **no se cambia OpenAPI ni `spec.md`/`plan.md`**: eso exige RFC y Gate humano. Lo que se ajusta aquí es el **plan de trabajo y el mapeo cliente↔backend** para que Android pueda consumir la Oleada 4 sin inventar campos.

## 1. Reglas del prototipo que sí se incorporan al cálculo backend

Del PDF (R14, R15, R16, páginas 4–5 y 86–97):

- No prometer IA predictiva; mostrar muestreo, calendario local y categorías reales.
- Ventana del prototipo: 14 días, umbral `minimumSamples=3`, franja tarde `14:00–18:59` locales, 7 barras L–D agregadas (no 14 barras, no pronóstico).
- Racha: un día cuenta **una sola vez** aunque haya varias aportaciones; las reversiones recomputan; progreso/hitos/mensaje son derivados.
- Permisos nativos (`POST_NOTIFICATIONS`, Android 13+) son la autoridad; E019 solo sincroniza estado. Preferencia `enabled=true` **no** autoriza push sin permiso del SO y token vigente.
- No almacenar datos financieros completos en el texto de push visible en pantalla bloqueada.

**Ajuste aplicado:** estas reglas quedan como requisitos de cálculo/validación en el diseño de E052–E056 y en la comprobación de consentimiento de E057–E063. La ventana de 14 días del prototipo **no está en el OpenAPI** (que usa `period=YYYY-MM`): se respeta `period`; si el producto exige 14 días, se tramita RFC.

## 2. Conciliación por E-ID

Leyenda: ✅ coincide · ⚠️ parcial, se resuelve con mapeo · ❌ discrepancia que requiere RFC/Gate si se quiere el contrato del PDF.

| E-ID | PDF V6.0.2 propone | OpenAPI/SDD aprobado (autoridad) | Resultado | Ajuste aplicado en nuestro plan |
|---|---|---|---|---|
| **E052** | query `windowDays`, `endDate`; response `windowDays,from,to,antExpenseCount,minimumSamples,enoughData,title,summary,afternoonCount,afternoonPercentage,weekdayBars[].weekday/total` | query `period`; `PageOfInsight{items[],nextCursor,enoughData,minimumSamples}`; `Insight{id,title,explanation,period,evidenceCount,modelVersion}` | ❌ | Backend implementa `PageOfInsight`. Cliente: `antExpenseCount←evidenceCount`, `title/summary←title/explanation`, `minimumSamples/enoughData` directos. `from/to/afternoon*/weekdayBars` requieren RFC; mientras tanto se calculan en cliente solo si hay datos, o se muestran como no disponibles. |
| **E053** | `period,limit,type=ant`; `period,categories[]{category,total,percentageOfTop,shareOfTotal},allCategoriesTotal` | `period`; `PageOfCategoryAnalytic{items[],nextCursor}`; items `categoryId,period,amount,currency,percentage`(0–100) | ⚠️ | `category←categoryId`, `total←amount`, `percentageOfTop←percentage`. `shareOfTotal` y `allCategoriesTotal` se derivan sumando `amount` de la página completa; `limit/type` se resuelven con paginación y filtro en cliente. Si se quiere en servidor: RFC. |
| **E054** | `windowDays`; `windowDays,count,afternoonWindowStart,afternoonWindowEndExclusive,afternoonCount,afternoonPercentage,patternDetected,headline` | `period`; `PageOfSpendingPattern{items[],nextCursor}`; items `categoryId,description,sampleSize,period` | ❌ | Backend calcula patrones por categoría con `sampleSize` real y `description` verificable. Franja horaria y `patternDetected/headline` requieren RFC; no inferir probabilidades. |
| **E055** | `period,limit`; `recommendations[]{id,title,text,reason,basedOnPeriod,category,observedAmount}` + `hasEnoughData` | `period`; `PageOfRecommendation{items[],nextCursor,hasEnoughData}`; items `id,title,reason,disclaimer,modelVersion` | ⚠️ | `recommendations←items`; `text←reason`; `basedOnPeriod←period`. El PDF omite `disclaimer` y `modelVersion`, que son **obligatorios** en backend: Android debe mostrarlos. `category/observedAmount` solo si el modelo los produce con evidencia; si se exigen como campos: RFC. ML real K-Means con Gate `ML-155`. |
| **E056** | query `asOf`; `days,lastSavingDate,nextMilestoneDays,progressPercentage,pigMessage,milestones[]{days,reached}` | sin parámetros; `SavingsStreak{days,timezone,asOfDate}` | ⚠️ | `asOf←asOfDate`; `nextMilestoneDays/progressPercentage/pigMessage/milestones` se derivan en cliente desde `days` (hitós 3/7/15/30). `lastSavingDate` no existe: RFC si se requiere. Backend recomputa tras reversión y cuenta un día una sola vez. |
| **E057** | `enabled,budgetAlerts,goalUpdates,recurringReminders,streakReminders,quietHours.enabled/start/end,timeZone,updatedAt` | `pushEnabled,emailEnabled,reminderEnabled,version,budgetAlerts,goalUpdates,recurringReminders,streakReminders,quietHours{enabled,start,end},timeZone` | ❌ | No existe `enabled`: Android usa `pushEnabled` (+ `emailEnabled`, `reminderEnabled`). `updatedAt` se sustituye por `version`. No añadir campos. Permiso OS se controla aparte. |
| **E058** | body con `enabled` requerido y `quietHours.*` requeridos; response con `updatedAt` | body `pushEnabled,emailEnabled,reminderEnabled` requeridos, **sin `version`**; response como E057 con `version` | ❌ | Android envía solo los tres booleanos y opcionales; `enabled`/`version`/`updatedAt` se rechazan como campos desconocidos. Si se quiere `enabled`: RFC. Respetar `quietHours` e IANA antes de enviar push. |
| **E059** | `token,deviceId,platform=android,appVersion,locale,timeZone`; response `deviceRegistrationId,deviceId,registeredAt,active` | header `Idempotency-Key` obligatorio; body `provider=fcm,token,deviceLabel`; response `PushDevice{id,provider,deviceLabel,registeredAt}` + `Location` | ❌ | `deviceRegistrationId←id`; `deviceId←deviceLabel` (formato acordado con Android); `platform←provider=fcm`; `active=true` implícito. `appVersion/locale/timeZone` no van en el contrato: no enviarlos sin RFC. **Idempotency-Key obligatoria**; token cifrado y nunca devuelto ni logueado. |
| **E060** | path `deviceRegistrationId`; 204; revocar en logout/baja/rotación | path UUID; 204; 404 homogéneo para ajeno | ✅ | Coincide. Conectar logout/E014 como impacto transversal con Gate. |
| **E061** | `unreadOnly,type(budget,goal,recurring,streak,security),limit,cursor`; items `id,type,title,body,relatedEntityId,createdAt,readAt`; `unreadCount,nextCursor` | `limit,cursor,isRead,type(budget,goal,recurring,streak,system)`; items `id,title,message,isRead,createdAt,unreadCount?` | ⚠️ | `unreadOnly←isRead=true`; `security→system`; `body←message`; `readAt` se deriva de `isRead`; `relatedEntityId/type` por item requieren RFC. `unreadCount` es global, no por página. Sin PII en texto. |
| **E062** | response `id,readAt,unreadCount?` | `Notification{id,title,message,isRead,createdAt,unreadCount?}` | ⚠️ | `isRead=true` equivale a `readAt`; transición atómica e idempotente. Si Android exige timestamp de lectura: RFC. |
| **E063** | 200 con `updatedCount,readAt`; query `before` opcional | **202** `Accepted{status:"accepted",requestId}`; sin parámetros | ❌ | Backend cumple **202 con trabajo durable** (guía V2). Android maneja 202 + `requestId` y consulta E061; si exige 200/`before`: RFC y Gate. **No cambiar el status por iniciativa propia.** |

## 3. Lo que sí ajustamos (sin tocar contratos aprobados)

1. **Mapa cliente↔backend** de las 12 operaciones (tabla anterior), para que Android no espere campos inexistentes.
2. **Reglas de cálculo** de R14/R15/R16 incorporadas al diseño: `minimumSamples=3`, ventana local, 7 barras agregadas, racha por día único y recomputada, consentimiento real antes de push.
3. **Lista de discrepancias que requieren RFC/Gate**: E052 (`windowDays/from/to/afternoon*/weekdayBars`), E054 (franja horaria), E057/E058 (`enabled/updatedAt`), E059 (`deviceId/platform/appVersion/locale/timeZone`), E061/E062 (`relatedEntityId/type/readAt`), E063 (200 vs 202).
4. **Estado honesto**: `PDF_MATCH=REVISADO`, `CONTRATO_MODIFICADO=NO`, `RFC_REQUERIDA=SI` para los campos marcados ❌/⚠️ si el producto los quiere en la API.

## 4. Aplicación del Contexto Maestro V2 (Oleadas 4 y 5)

- Partimos de `origin/main` `dbff5f8...` (PR #142); **54/75**, release `v0.19.2`; Oleadas 4/5 no desplegadas.
- Vocabulario separado: `CODE_IMPLEMENTED`, `CI_GREEN`, `AWS_DEPLOYED`, `HTTPS_VERIFIED`, `FUNCTIONAL_VERIFIED`, `ANDROID_ACCEPTED`, `BLOCKED`.
- RFC-016 está `PROPUESTO/NO APROBADO`: solo usamos los campos ya presentes en OpenAPI (`enoughData`, `minimumSamples`, `hasEnoughData`); no añadimos `version` a `NotificationPreferencesUpdate`.
- ML-155: K-Means real y 100% funcional, pero con `ML_GATE_APPROVED` pendiente; hasta entonces `BLOCKED_ML_GATE`, nunca "modelo" = reglas fijas.
- Firebase/FCM real es parte de Oleada 4; sin AWS, sin secretos en repo; ver `FIREBASE_FCM_OLEADA4_RUNBOOK.md`.
- E063 `202` durable; E061–E063 bandeja real, no arrays vacíos.
- Impacto transversal E014/E020: entregar inventario y pedir Gate puntual antes de tocarlos.
- Deudas históricas (E015, E070, E031, SES, multi-AZ, health E072/E073) **no** se arreglan por iniciativa propia.
- Ramas: `liz-oleada4` desde `main`; `liz-oleada5` desde `liz-oleada4` para heredar FCM sin esperar merge; se documenta y se rebaseará sobre `main` cuando el responsable integre Oleada 4.

**Fin del documento de control.**
