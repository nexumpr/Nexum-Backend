# Evidencia final — Oleada 4 (E052–E063)

**Rama:** `liz-oleada4` · **Autoridad:** guía V2 §6.1–§6.12/F + OpenAPI `nexum-api-v1.json` · **Base:** `origin/main` `dbff5f8…`.

> Documento de consolidación. Ningún estado de AWS, HTTPS, FCM E2E o Gate humano se da por bueno sin evidencia ejecutable. E059–E063 y el emisor FCM se entregan en un commit atómico porque comparten dependencias de runtime (`PushSealer`, `CursorKey`, `Preferencias`).

## 1. Matriz de endpoints

| E-ID | Método / ruta | operationId | Código local | Cambios | Pruebas locales | AWS | HTTPS | Gate |
|---|---|---|---|---|---|---|---|---|
| E052 | GET `/api/v1/analytics/insights` | `nexum_e052` | `a653004` | analytics determinista | unit + HTTP + H06 | NO | NO | — |
| E053 | GET `/api/v1/analytics/categories` | `nexum_e053` | `32ae647` | mayor resto, suma 100 | unit + HTTP + H06 | NO | NO | — |
| E054 | GET `/api/v1/analytics/spending-patterns` | `nexum_e054` | `bd0f6cd` | patrones verificables | unit + HTTP + H06 | NO | NO | — |
| E055 | GET `/api/v1/analytics/recommendations` | `nexum_e055` | `3f3c42f` | K-Means real `kmeans-v1` | unit + HTTP + Python↔Go | NO | NO | `ML_GATE_APPROVED=NO` |
| E056 | GET `/api/v1/analytics/savings-streak` | `nexum_e056` | `9ea388b` | racha IANA real | 8 unit + 3 HTTP | NO | NO | regla pendiente |
| E057 | GET `/api/v1/notification-preferences` | `nexum_e057` | `d9b3442` | defaults RFC-016 + SK dedicada | 3 unit + 5 Dynamo + 4 HTTP | NO | NO | — |
| E058 | PUT `/api/v1/notification-preferences` | `nexum_e058` | `d35943d` | CAS interno por versión | 3 unit + 3 Dynamo + 5 HTTP | NO | NO | discrepancia `version` reportada |
| E059 | POST `/api/v1/devices/push-tokens` | `nexum_e059` | atómico E059–E063 | token AES-GCM en reposo, rotación e idempotencia | 8 app + 7 Dynamo + 8 HTTP | NO | NO | credencial no inyectada |
| E060 | DELETE `/api/v1/devices/push-tokens/{deviceRegistrationId}` | `nexum_e060` | atómico E059–E063 | baja idempotente y token irrecuperable | comparte suite E059 | NO | NO | credencial no inyectada |
| E061 | GET `/api/v1/notifications` | `nexum_e061` | atómico E059–E063 | bandeja persistida, cursor firmado, `unreadCount` global | 7 app + 5 Dynamo + 5 HTTP | NO | NO | — |
| E062 | PATCH `/api/v1/notifications/{notificationId}/read` | `nexum_e062` | atómico E059–E063 | lectura atómica e idempotente | comparte suite E061 | NO | NO | — |
| E063 | POST `/api/v1/notifications/read-all` | `nexum_e063` | atómico E059–E063 | 202 durable con reanudación | comparte suite E061 | NO | NO | — |

## 2. Evidencia de ejecución local

- `gofmt` aplicado a todo el código nuevo; `gofmt -l` sin salida.
- `go vet ./...` PASS; `go build ./...` PASS.
- `go test ./... -count=1 -timeout 600s` PASS (todos los paquetes, incluidos `cmd/server`, `internal/...`, `ml` y `tests/...`).
- Pruebas nuevas: 8 app + 7 Dynamo + 8 HTTP (E059/E060); 7 app + 5 Dynamo + 5 HTTP (E061–E063); 11 app + 6 Dynamo + 6 HTTP v1 + 1 integración de fuente E059 + 3 de configuración de claves (FCM/outbox).
- `py -3 herramientas/validar_sdd_integral.py --check-git` → `H06_CONTRACT_PASS` (75 operaciones; 300 specs; 33M+23S+12E+7O; OpenAPI 3.1 válido) y `SDD_DOCUMENTAL_PASS` (894 archivos rastreados; SHA256 exacto; AWS/HTTP NOT_RUN).
- `py -3 herramientas/verificar_iac_devsecops.py` → `IAC_DEVSECOPS_PASS: 51 archivos sin hallazgos`.
- `go test -race`: `NOT_RUN_LOCAL` — requiere cgo/gcc ausente en Windows; CI Linux es autoridad.

## 3. FCM real, secretos y consentimiento

- Cliente HTTP v1 + OAuth2 JWT RS256 con stdlib; sin legacy server key. Reintento acotado sólo en 429/5xx con `data.eventId` estable; `UNREGISTERED`/`INVALID_ARGUMENT` son permanentes.
- Outbox durable `OUTBOX#GLOBAL / DUE#<RFC3339Nano>#<id>` con lease condicional, deduplicación por `(userID,eventId)`, sin `Scan`, sin GSI2 y sin endpoint público `send-push`.
- Antes de enviar se consulta `notificationprefs.Service.Get`: sin preferencias o con `pushEnabled=false`, `quietHours` activo o zona IANA inválida, **no se envía**. `Defaults()` de E057 tiene `pushEnabled=false`, por lo que el sistema falla cerrado.
- El worker lee `tokenCipher` base64 de E059 con AAD `<userID>|<deviceID>`, omite dispositivos `active=false` y replica la baja E060 ante `UNREGISTERED`.
- Claves de runtime: `NEXUM_PUSH_TOKEN_KEY_B64` (AES-GCM, alias `NEXUM_PUSH_TOKEN_KEY`) y `NEXUM_NOTIFICATION_CURSOR_KEY_B64` (HMAC). Nunca se generan efímeras: sin clave válida, 503. Nunca aparecen en repo, logs, `memoria.md` ni evidencia.
- Firebase: proyecto Spark creado, acceso Editor delegado, `applicationId=com.nexum.nexum`, dispositivo/emulador con Google Play Services listo. **Credencial privada de servidor nunca se pegó en chat/repo.**
- `FCM_SERVICE_CREDENTIAL_INJECTED=SI` desde el 2026-11-23: el bundle `v2` (17 variables, 13 de `v1` + 4 FCM) fue publicado en S3 y el bootstrap de las dos instancias nuevas registró `[bootstrap] configuracion instalada (version v2, 17 variables)`. El contenido del JSON y las claves **nunca** se imprimieron ni se leyeron desde la instancia.
- `FCM_WORKER_ACTIVO=NO`, `FCM_CONFIGURED=PARCIAL`: el release productivo sigue siendo `v0.19.2+3098f379d7ab`, anterior al commit `9b5d950` que implementa el emisor. La credencial está inyectada, pero el binario desplegado no tiene código para usarla. Hace falta publicar/desplegar un artefacto Oleada 4 con Gate propio.
- `FCM_E2E_PASS=NO` por decisión del responsable: no se realizó prueba con dispositivo.

## 4. ML / transversal / límites honestos

- ML: artefacto `kmeans-v1` SHA256 `1997370f1c148df63e126e477d1f931c565d2b1f9b54dd57dc1e48b1a16d48cf`; datos SHA256 `8d2bf29691a08eee6a1cd1230ad81b8454712424721e5e3e21bb9e9bd99918fd`; `ML_GATE_APPROVED=NO`.
- E014/E020: ver `IMPACTO_TRANSVERSAL_OLEADA4.md`; `GATE_TRANSVERSAL_O4=PENDIENTE` y E014/E020 quedan intactos.
- Sin merge a `main`, PR ni cambio de release. El 2026-11-23 se aplicó con Gate el plan `tfplan-v2c` (SHA-256 `3e5a469091dfd1cf79e897632cb4402d9d8248651d3f0a93f0255bab91c2c492`): **0 altas / 5 cambios / 0 bajas**, único cambio funcional `SECRETOS_URI v1 → v2`; IAM sin pérdida de ARN. Release productiva sigue `v0.19.2+3098f379d7ab`; LT v18; refresh `4f95effd-6acf-4f04-a883-211283c34640` Successful 100 %; ASG 2/2/2 con protección restaurada. `HTTPS_VERIFIED=SI` (`live` y `ready` 200 por CloudFront, DynamoDB/Redis UP). `FUNCTIONAL_VERIFIED=NO`: `recommendations`/`kmeans-v1` no se pudo verificar en producción porque el binario no incluye E055. Ver `OPERACION_FCM_AWS_OLEADA4.md` §4.1.
- `liz-oleada5` debe rebasearse tras la integración humana. E072/E073 permanecen `CONTRACT_BLOCKED` (Oleada 5).
- Revisión final posterior a `9b5d950`: los ítems sin marcar en E052–E063 son Gates/despliegue/dos nodos/HTTPS/`race`/E2E, no código pendiente. Se corrigió la redacción de `E061/E062/E063-T12` para reflejar que el código sí quedó commitado y pusheado, y se marcó `E061-CT07` como no aplicable verificado.
