# Firebase / FCM real — Runbook aplicado a Oleada 4

| Campo | Valor |
|---|---|
| Rama | `liz-oleada4` |
| Fecha | 2026-11-22 |
| Alcance | E057–E063 + emisor FCM; reutilizable por E064–E067 en Oleada 5 |
| Estado actual | `FCM_CONFIGURED=NO` · `FCM_E2E_PASS=NO` · `BLOCKED_FIREBASE_ACCESS` / `BLOCKED_ANDROID_TEST_DEVICE` hasta que se concedan acceso y dispositivo |
| Costo | Firebase **plan Spark**, sin tarjeta, sin servicios facturables |

> Documento de ejecución. **No hay credenciales, tokens ni JSON privado aquí.** No se han tocado proyectos externos ni AWS.

## 1. Configuración en Firebase Console (requiere acceso del titular)

1. Buscar proyecto Nexum existente y reutilizarlo. Si no existe, crear `nexum-dev` (ID definitivo lo asigna Firebase) en **Spark**. No activar Auth, Firestore, Cloud Functions, Cloud Run ni facturación.
2. Registrar la app Android con el `applicationId` **real** del repositorio móvil (pedirlo a Android; no inventarlo). Descargar `google-services.json` y entregarlo a Android por canal autorizado.
3. Verificar **FCM HTTP v1** habilitado. Usar Admin SDK para Go u OAuth 2.0; **nunca** legacy server key ni credencial de servidor en Android.
4. Crear identidad de servidor de mínimo privilegio (`roles/firebase.messagingAdmin` o equivalente vigente); **sin Owner**. Guardar la credencial privada fuera del repo, con rotación/revocación. Preferir identidad federada sin claves si es compatible con AWS.
5. Registrar en el handoff: project ID, app ID, sender ID, package, versión SDK, API activa, rol usado y fecha — **sin el contenido de la credencial**.

## 2. Emisor Go y persistencia

- Cliente reutilizable con timeout acotado, retry exponencial con jitter solo para errores recuperables, límite de tasa y métrica por código HTTP. Sin token en logs.
- Manejo explícito de `UNREGISTERED`/token inválido, error de credencial y baja mediante E060.
- Flujo real: evento de dominio → **outbox/notificación persistida** → worker idempotente → dispositivos activos → FCM → estado de intentos y deduplicación. **No crear endpoint público `send-push`.**
- Consentimiento: `pushEnabled`, `reminderEnabled`, `budgetAlerts`, `goalUpdates`, `recurringReminders`, `streakReminders`, `quietHours`, `timeZone` de E057/E058. Sin permiso del SO o sin token vigente, no enviar aunque `pushEnabled=true`.
- Token cifrado en reposo con **AES-GCM** y clave de entorno/secret store (`NEXUM_PUSH_TOKEN_KEY`), con plan de rotación. Nunca devolver el token ni su hash reversible por HTTP.
- E063 responde `202` con `requestId` y trabajo durable; E061/E062 gestionan bandeja y lectura persistida.

## 3. Variables y secretos (nombres, no valores)

| Variable | Uso | Dónde se protege |
|---|---|---|
| `FIREBASE_PROJECT_ID` | Proyecto FCM | Configuración, no secreto |
| `GOOGLE_APPLICATION_CREDENTIALS` o `FCM_SERVICE_ACCOUNT_JSON` | Identidad de emisor | Secret store del responsable; nunca Git |
| `NEXUM_PUSH_TOKEN_KEY` | Cifrado AES-GCM de tokens | Secret store; rotación |
| `NEXUM_DYNAMO_TABLE`, `AWS_REGION`, Redis | Persistencia/outbox/lease | Configuración existente |

## 4. Prueba de aceptación (antes de decir `FCM_E2E_PASS`)

Con dispositivo Android real o emulador con Google Play Services:

1. App del package correcto obtiene token FCM auténtico y renueva en `onNewToken()`.
2. `POST /api/v1/devices/push-tokens` con Bearer JWT + `Idempotency-Key`, body `provider=fcm,token,deviceLabel`.
3. `PUT /api/v1/notification-preferences` con `pushEnabled=true`; permiso `POST_NOTIFICATIONS` concedido.
4. Disparar evento autorizado desde runner local autenticado (no HTTP público).
5. Verificar **dos hechos distintos**: FCM aceptó el envío **y** la app mostró la notificación.
6. Repetir con usuario A/B: rotación, logout, E060, opt-out, `quietHours`, token inválido. Sin secretos en evidencia.

Sin acceso o dispositivo: `BLOCKED_FIREBASE_ACCESS` / `BLOCKED_ANDROID_TEST_DEVICE`; **no** marcar `FCM_E2E_PASS`.

## 5. Handoff y AWS (responsable)

Al propietario: enlaces de consola, IDs no secretos, paquete para Android, resultados anonimizados, guía SDK/permisos y runbook de runtime (IAM mínimo, secretos, egress HTTPS, smoke, rollback). Firebase en Spark costo 0. El despliegue en EC2, inyección de credencial y prueba final desde AWS requieren **Gate posterior**; sin él no se declara push recibido en producción.

## 6. Estados separados

`FCM_CONFIGURED` · `FCM_ANDROID_E2E_PASS` · `AWS_DEPLOYED` · `HTTPS_VERIFIED` · `FUNCTIONAL_VERIFIED` · `ANDROID_ACCEPTED` se informan por separado; uno no implica el otro.
