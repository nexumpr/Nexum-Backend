# Firebase / FCM real — Runbook aplicado a Oleada 4

| Campo | Valor |
|---|---|
| Rama | `liz-oleada4` |
| Fecha | 2026-11-22 |
| Alcance | E057–E063 + emisor FCM; reutilizable por E064–E067 en Oleada 5 |
| Estado actual | `FIREBASE_CONSOLE_ACCESS=SI` · `FCM_SERVICE_CREDENTIAL_INJECTED=NO` · `ANDROID_DEVICE_READY=SI` · `FCM_E2E_PASS=NO` · `FCM_CONFIGURED=NO` hasta inyectar la credencial de servidor y ejecutar la prueba real |
| Costo | Firebase **plan Spark**, sin tarjeta, sin servicios facturables |

> Documento de ejecución. **No hay credenciales, tokens ni JSON privado aquí.** No se han tocado proyectos externos ni AWS.
>
> Datos no secretos confirmados por el responsable: proyecto Firebase Spark ya creado; acceso delegado a `perezlizeth489@gmail.com` con rol **Editor**; `applicationId` real **`com.nexum.nexum`**; emulador/dispositivo con Google Play Services listo. El bloqueo previo por acceso a consola y por dispositivo quedó resuelto; **sigue pendiente la credencial de servidor**, por lo que `FCM_E2E_PASS` permanece `NO`.

## 1. Configuración en Firebase Console (acceso confirmado)

1. Proyecto Nexum existente en **Spark**: **HECHO** (no activar Auth, Firestore, Cloud Functions, Cloud Run ni facturación).
2. App Android registrada con `applicationId` **`com.nexum.nexum`**: **HECHO**. `google-services.json` se entrega a Android por canal autorizado; **no se pega en el repo ni en evidencia**.
3. **FCM HTTP v1** habilitado. El emisor usa OAuth 2.0 JWT assertion RS256 con cuenta de servicio; **nunca** legacy server key ni credencial de servidor en Android.
4. Acceso a consola delegado a `perezlizeth489@gmail.com` con rol Editor. Para el emisor se recomienda una identidad de mínimo privilegio (`roles/firebase.messagingAdmin` o equivalente vigente), **sin Owner**; guardar la credencial fuera del repo, con rotación/revocación. Preferir identidad federada sin claves si es compatible con el runtime.
5. Registrar en el handoff: project ID, sender ID, package, versión SDK, API activa, rol usado y fecha — **sin el contenido de la credencial**.

## 2. Emisor Go y persistencia

- Cliente stdlib reutilizable: `POST https://fcm.googleapis.com/v1/projects/{project}/messages:send`, body `{"message":{"token":"...","notification":{"title":"...","body":"..."},"data":{"eventId":"..."}}}`. Access token OAuth con `grant_type=urn:ietf:params:oauth:grant-type:jwt-bearer` y scope `https://www.googleapis.com/auth/firebase.messaging`. Sin legacy server key.
- Retry exponencial con jitter acotado **solo en `429`/`5xx`**, máximo **2 reintentos**. El `data.eventId` es determinista por notificación y viaja en cada intento; un reintento no crea otra notificación.
- Clasificación: `UNREGISTERED`/`INVALID_ARGUMENT` son fallos permanentes (baja E060 y token no reutilizado); credencial inválida es error de configuración. Logs/métricas solo por código HTTP y clase de error; **nunca** por token, credencial ni body.
- Outbox durable en la tabla única existente, patrón nuevo: `PK=OUTBOX#GLOBAL`, `SK=DUE#<RFC3339Nano>#<notificationID>`. Query acotada por PK + `begins_with(SK,"DUE#")` con `Limit`; **sin `Scan`, sin índice nuevo y sin GSI2**. Lease con `UpdateItem` condicional y fence por `owner`; estados `pending|sending|sent|failed|revoked`, con `attempts`, `leaseUntil`, `nextAttemptAt`, `createdAt`, `updatedAt`.
- Idempotencia adicional por `(userID,eventId)` en la misma partición (`IDEM#<sha256>`) con escritura condicional. La bandeja E061 se persiste antes del envío en `USER#<sub>/NOTIFICATION#<UTC>#<id>`; dos consumidores no duplican entrega.
- Consentimiento E057/E058: antes de cada envío se consulta `notificationprefs.Service.Get`. Sin preferencias, `pushEnabled=false`, zona IANA inválida o `quietHours` activo ⇒ **no enviar**. `Defaults()` tiene `pushEnabled=false`, por lo que la ausencia de configuración falla cerrado.
- Tokens E059/E060: el item real usa `USER#<sub>/DEVICE#<id>` con `tokenCipher` (**string base64**), `tokenFingerprint`, `active` (bool), `provider`, `deviceLabel`, `registeredAt`, `revokedAt`, `version`. El AAD del sellador AES-GCM es exactamente `<userID>|<deviceID>`. Los dispositivos `active=false` se omiten. Ante `UNREGISTERED` se replica la baja: `active=false`, `revokedAt`, `version+1` y `REMOVE tokenCipher, tokenFingerprint`.
- Flujo real: evento de dominio → **outbox/notificación persistida** → worker idempotente → dispositivos activos → FCM → estado de intentos y deduplicación. **No crear endpoint público `send-push`.**
- E063 responde `202` con `requestId` y trabajo durable; E061/E062 gestionan bandeja y lectura persistida.

## 3. Variables y secretos (nombres, no valores)

**Prioridad:** los nombres `NEXUM_*` nuevos ganan sobre los alias antiguos. Para la credencial, el orden exacto es `NEXUM_FCM_CREDENTIALS_JSON` → `FCM_SERVICE_ACCOUNT_JSON` → `NEXUM_FCM_CREDENTIALS_FILE` → `GOOGLE_APPLICATION_CREDENTIALS`. Nunca se imprimen valores; los errores indican solo la variable ausente o inválida.

| Variable (prioridad) | Alias aceptado | Uso | Dónde se protege |
|---|---|---|---|
| `NEXUM_FCM_PROJECT_ID` | `FIREBASE_PROJECT_ID` | Proyecto FCM | Configuración, no secreto |
| `NEXUM_FCM_CREDENTIALS_JSON` | `FCM_SERVICE_ACCOUNT_JSON` | Credencial de cuenta de servicio en línea | Secret store del responsable; nunca Git ni logs |
| `NEXUM_FCM_CREDENTIALS_FILE` | `GOOGLE_APPLICATION_CREDENTIALS` | Ruta al JSON de credencial fuera del repo | Secret store / volumen montado |
| `NEXUM_FCM_ENDPOINT` | — | Opcional; por defecto HTTP v1 de Google | Solo pruebas/rewrite controlado |
| `NEXUM_FCM_TIMEOUT_MS` | — | Opcional; defecto `5000`, máximo `60000` | Configuración |
| `NEXUM_PUSH_TOKEN_KEY_B64` | `NEXUM_PUSH_TOKEN_KEY` | Clave AES-GCM de 32 bytes para E059/E060 y el worker | Secret store; rotación. La consume el sellador, no el emisor FCM |
| `DYNAMODB_TABLE`, `AWS_REGION`, Redis | — | Persistencia/outbox/lease | Configuración existente |

## 4. Prueba de aceptación (antes de decir `FCM_E2E_PASS`)

Con el dispositivo/emulador ya listo:

1. Inyectar la credencial de servidor por secret store usando **una sola** de las variables de la tabla; confirmar `FCM_SERVICE_CREDENTIAL_INJECTED=SI` sin mostrar su contenido.
2. App del package `com.nexum.nexum` obtiene token FCM auténtico y renueva en `onNewToken()`.
3. `POST /api/v1/devices/push-tokens` con Bearer JWT + `Idempotency-Key`, body `provider=fcm,token,deviceLabel`.
4. `PUT /api/v1/notification-preferences` con `pushEnabled=true`; permiso `POST_NOTIFICATIONS` concedido.
5. Disparar evento autorizado desde runner local autenticado (no HTTP público).
6. Verificar **dos hechos distintos**: FCM aceptó el envío **y** la app mostró la notificación.
7. Repetir con usuario A/B: rotación, logout, E060, opt-out, `quietHours`, token inválido. Sin secretos en evidencia.

Hasta completar el paso 6: `FCM_SERVICE_CREDENTIAL_INJECTED=NO` y `FCM_E2E_PASS=NO`. No basta con tener consola y dispositivo.

## 5. Handoff y AWS (responsable)

Al propietario: enlaces de consola, IDs no secretos, paquete `com.nexum.nexum`, resultados anonimizados, guía SDK/permisos y runbook de runtime (IAM mínimo, secretos, egress HTTPS, smoke, rollback). Firebase en Spark costo 0. El despliegue en EC2, inyección de credencial y prueba final desde AWS requieren **Gate posterior**; sin él no se declara push recibido en producción.

## 6. Estados separados

`FIREBASE_CONSOLE_ACCESS` · `FCM_SERVICE_CREDENTIAL_INJECTED` · `ANDROID_DEVICE_READY` · `FCM_CONFIGURED` · `FCM_ANDROID_E2E_PASS` · `FCM_E2E_PASS` · `AWS_DEPLOYED` · `HTTPS_VERIFIED` · `FUNCTIONAL_VERIFIED` · `ANDROID_ACCEPTED` se informan por separado; uno no implica el otro.

Estado actual: `FIREBASE_CONSOLE_ACCESS=SI` · `FCM_SERVICE_CREDENTIAL_INJECTED=NO` · `ANDROID_DEVICE_READY=SI` · `FCM_CONFIGURED=NO` · `FCM_E2E_PASS=NO` · `AWS_DEPLOYED=NO`.

## 7. Integración de código (constructores)

- Emisor: `push.NewClient(push.Config{...})` y `Client.Enviar(ctx, push.Mensaje{...})`.
- Configuración desde entorno: `configuracionFCMOleada4DesdeEntorno() (push.Config, error)`.
- Arranque: `iniciarFCMWorkerOleada4(ctx, d, cfg) (func(), error)`. El worker se cancela con la función devuelta; no registra rutas ni usa `init()`.
- Constructor alternativo: `iniciarFCMWorkerOleada4Con(ctx, d, cfg, fuente, bandeja)` para inyectar otra fuente de tokens o bandeja sin editar la estructura de dependencias.
- `d.PushSealer` se usa directamente (tipo `appPushDevices.TokenSealer`, que declara `Sellar`; el sellador concreto también implementa `Abrir`). Si el responsable amplía ese campo a una interfaz con ambos métodos, se elimina la aserción interna.
- `Preferencias` ya es campo real (`*notificationprefs.Service`) y se usa directamente; no queda reflexión en el arranque. Si no está conectado, el worker **no envía nada** (fail-closed).
