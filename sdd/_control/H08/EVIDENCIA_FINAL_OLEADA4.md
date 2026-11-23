# Evidencia final — Oleada 4 (E052–E063)

**Rama:** `liz-oleada4` · **Autoridad:** guía V2 §6.1–§6.12/F + OpenAPI `nexum-api-v1.json` · **Base:** `origin/main` `dbff5f8…` · **Release desplegada:** `v0.20.0+4bf6758d372`.

> Documento de consolidación. Ningún estado de AWS, HTTPS, FCM E2E o Gate humano se da por bueno sin evidencia ejecutable. E059–E063 y el emisor FCM se entregan en un commit atómico porque comparten dependencias de runtime (`PushSealer`, `CursorKey`, `Preferencias`).

## 1. Matriz de endpoints

| E-ID | Método / ruta | operationId | Código local | Cambios | Pruebas locales | AWS funcional | HTTPS funcional | Gate |
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

> `NO` en `AWS funcional`/`HTTPS funcional` significa que **no se ejecutó la prueba individual con JWT**. Tras el despliegue se comprobaron las **12 rutas** por HTTPS sin JWT (todas `401`) frente a una ruta inexistente de control (`404`): eso acredita presencia de ruta, no comportamiento funcional. El detalle por E-ID quedó registrado en los `checklist.md` actualizados (§6).

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
- `FCM_WORKER_ACTIVO=DESPLEGADO_NO_VERIFICADO_POR_LOG`, `FCM_CONFIGURED=SI_SIN_E2E`: el release `v0.20.0+4bf6758d372` contiene el emisor/worker FCM y fue desplegado el 2026-11-23 (§5). No hay streams de CloudWatch ni agente SSM en las instancias, así que el mensaje `FCM Oleada 4: worker iniciado` no pudo confirmarse en journal; la prueba E2E con dispositivo sigue sin ejecutarse.
- `FCM_E2E_PASS=NO` por decisión del responsable: no se realizó prueba con dispositivo.

## 4. ML / transversal / límites honestos

- ML: artefacto `kmeans-v1` SHA256 `1997370f1c148df63e126e477d1f931c565d2b1f9b54dd57dc1e48b1a16d48cf`; datos SHA256 `8d2bf29691a08eee6a1cd1230ad81b8454712424721e5e3e21bb9e9bd99918fd`; `ML_GATE_APPROVED=NO`.
- E014/E020: ver `IMPACTO_TRANSVERSAL_OLEADA4.md`; `GATE_TRANSVERSAL_O4=PENDIENTE` y E014/E020 quedan intactos.
- Sin merge a `main` ni PR. El 2026-11-23 se aplicó con Gate el plan `tfplan-v2c` (SHA-256 `3e5a469091dfd1cf79e897632cb4402d9d8248651d3f0a93f0255bab91c2c492`): **0 altas / 5 cambios / 0 bajas**, único cambio funcional `SECRETOS_URI v1 → v2`; IAM sin pérdida de ARN; LT v18; refresh `4f95effd-6acf-4f04-a883-211283c34640` Successful 100 %. Después, con Gate de release propio, se desplegó `v0.20.0+4bf6758d372`: plan `tfplan-o4b` SHA-256 `e45b79c461d5a8cc4968ebd6606fe93ca0151eddc1b14c54fbaea3b1c5e95621` (**0 altas / 4 cambios / 0 bajas**), apply **0 added / 4 changed / 0 destroyed**, LT **v19**, refresh `914d3fc7-f808-4865-a744-689cfe749b61` Successful 100 % y ASG 2/2/2 con protección restaurada. `HTTPS_VERIFIED=SI` (`live` y `ready` 200 por CloudFront, DynamoDB/Redis UP). `FUNCTIONAL_VERIFIED=PARCIAL`: rutas nuevas responden 401 sin JWT; `recommendations`/`kmeans-v1` no consultado. Ver `OPERACION_FCM_AWS_OLEADA4.md` §4.1–§4.2.
- `liz-oleada5` debe rebasearse tras la integración humana. E072/E073 permanecen `CONTRACT_BLOCKED` (Oleada 5).
- Revisión final posterior a `9b5d950`: los ítems sin marcar en E052–E063 son Gates/`race`/CI Linux/E2E/prueba con JWT, no código pendiente. Se corrigió la redacción de `E061/E062/E063-T12` para reflejar que el código sí quedó commitado y pusheado, y se marcó `E061-CT07` como no aplicable verificado.

## 5. Despliegue real del binario Oleada 4 (2026-11-23)

| Hecho | Evidencia sin secretos |
|---|---|
| Release | `v0.20.0+4bf6758d372`, construido desde `4bf6758d372` con `GOOS=linux GOARCH=arm64 CGO_ENABLED=0`, `-trimpath`, `-buildvcs=true`. |
| Binario | `nexum-backend-linux-arm64`, 22 755 777 B, SHA-256 `8f681d4ecf547bb40a45008a78bc683fe44aec5b19cdcea9858c8e51459ce94c`. Marcadores locales: `FCM Oleada 4`, `worker iniciado`, `kmeans-v1`, `NEXUM_FCM_CREDENTIALS_JSON`. |
| Unidad systemd | `nexum-backend.service`, 1 357 B, SHA-256 `6fa4d5fb051914102b1bc3b6cd2814e881bf02bb26df1d938f4f0d46e15705cc`. |
| Publicación | Objetos PUT con SSE-S3 `AES256` en `s3://nexum-h08i-artefactos-713797602090/releases/v0.20.0+4bf6758d372/`; respuesta HTTP 200 para ambos. No se imprimieron URLs presignadas ni se releyeron los objetos con la identidad publicadora (la política lo niega). |
| Terraform | `tfplan-o4b` SHA-256 `e45b79c461d5a8cc4968ebd6606fe93ca0151eddc1b14c54fbaea3b1c5e95621`, RC 0, `0 to add, 4 to change, 0 to destroy`; apply RC 0, `0 added, 4 changed, 0 destroyed`; log SHA-256 `79bef7c0f838d6a034339a96063e4715da58d24e34b31fbad6582f3a8b1957a5`. 34 ARN de artefactos conservados; contraseña RBAC nunca impresa ni versionada. |
| Infraestructura | LT `lt-05445f32a7ee8a2f7` v19; refresh `914d3fc7-f808-4865-a744-689cfe749b61` `Successful 100 %`; ASG `nexum-h08i-asg-backend` 2/2/2 con `i-095a65bdfd7a95448` e `i-0a00134ccecc76ffa` `InService`/`Healthy`/`ProtectedFromScaleIn=True`. La protección se restauró manualmente tras el reemplazo de instancias. |
| Consola EC2 | Ambas instancias: `BIN_SHA_OK=True`, `UNIT_SHA_OK=True`, `BUNDLE_V2_17=True`, `SERVICE_ACTIVE=True`, `BOOTSTRAP_ERROR=False`. |
| HTTPS | CloudFront `https://d2lpmqy5w6x9es.cloudfront.net`: `/api/v1/health/live` y `/api/v1/health/ready` → 200, `dynamodb=UP`, `redis=UP`. |
| Rutas nuevas | Sin JWT, las 12 operaciones E052–E063 responden `401`: `GET /analytics/insights`, `/analytics/categories`, `/analytics/spending-patterns`, `/analytics/recommendations`, `/analytics/savings-streak`, `/notification-preferences`, `/notifications`; `PUT /notification-preferences`; `POST /devices/push-tokens`, `/notifications/read-all`; `DELETE /devices/push-tokens/{id}`; `PATCH /notifications/{id}/read`. Ruta inexistente de control → 404. |
| Límites | Sin streams en CloudWatch Logs ni agente SSM: no se confirmó por journal `FCM Oleada 4: worker iniciado`. Sin JWT no se consultó `recommendations`, por lo que `modelVersion:"kmeans-v1"` sigue sin verificación en producción. `FCM_E2E_PASS=NO`, `ML_GATE_APPROVED=NO`, `GATE_TRANSVERSAL_O4=PENDIENTE`, `CI_LINUX=PENDIENTE`, `go test -race=NOT_RUN_LOCAL`. |

**Estados finales:** `AWS_DEPLOYED=SI` · `HTTPS_VERIFIED=SI` · `FCM_SERVICE_CREDENTIAL_INJECTED=SI` · `FCM_WORKER_ACTIVO=DESPLEGADO_NO_VERIFICADO_POR_LOG` · `FCM_CONFIGURED=SI_SIN_E2E` · `FCM_E2E_PASS=NO` · `FUNCTIONAL_VERIFIED=PARCIAL` · sin merge ni PR.

## 6. Auditoría de cierre y checklists (actualización posterior al despliegue)

Los doce `sdd/03-endpoints/E0xx-*/checklist.md` se actualizaron con la evidencia realmente ejecutada, sin marcar nada que no esté verificado:

| Marca | Estado en los 12 checklists | Base de la marca |
|---|---|---|
| `[x]` despliegue en dos instancias, TLS e IaC | SÍ | release `v0.20.0+4bf6758d372`, LT v19, ASG 2/2/2, refresh `914d3fc7…` 100 %, `/health/live` y `/ready` 200 con DynamoDB/Redis UP. |
| `[x]` Gate agregado de release y rollback preservado | SÍ | `tfplan-o4b` aplicado `0/4/0`; `v1` y `v0.19.2+3098f379d7ab` conservados. |
| `[x]` presencia de ruta HTTPS | SÍ, 12/12 | `401` sin JWT en cada una de las 12 operaciones; `404` en ruta de control. |
| `[ ]` prueba funcional con JWT en AWS | NO | Sin JWT válido; `FUNCTIONAL_VERIFIED_AWS=NO`. |
| `[ ]` `go test -race` y CI Linux | NO | `NOT_RUN_LOCAL` · `CI_LINUX=PENDIENTE`. |
| `[ ]` Gate individual por E-ID y firma H06 | NO | Se ejerció un Gate agregado de Oleada 4; `GATE_H06_PENDIENTE.md` sigue vigente. |
| `[ ]` ML-155, transversal E014/E020 y FCM E2E | NO | `ML_GATE_APPROVED=NO` · `GATE_TRANSVERSAL_O4=PENDIENTE` · `FCM_E2E_PASS=NO`. |

Las fichas `spec.md`, `plan.md`, `tasks.md` y el OpenAPI **no** se modificaron; solo se ajustaron checklists y documentos de control para que el registro sea verdadero.
