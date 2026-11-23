# Impacto transversal de la Oleada 4 (E052–E063) sobre E014/E020

**Estado:** propuesta documental, `PENDIENTE_GATE_TRANSVERSAL_O4`. E014 y E020 siguen intactos; OpenAPI, `spec.md` y `plan.md` no se modificaron. El documento se redactó antes del despliegue de la Oleada 4; después se aplicó con Gate el bundle `v2` y la release `v0.20.0+4bf6758d372` (Terraform/IAM/LT v19 y AWS), sin tocar por ello E014 ni E020.

## 1. Familias de datos introducidas por E052–E063

| Familia | PK / SK | Propietario | Consumo | Cifrado / secreto | TTL / retención | Acceso DynamoDB | Coste estimado |
|---|---|---|---|---|---|---|---|
| `NOTIFICATION_PREFERENCES` | `USER#<sub>` / `NOTIFICATION_PREFERENCES` | Titular (`sub` verificado) | E057 GET, E058 PUT y worker FCM | Sin PII sensible; `quietHours`/`timeZone` son preferencias; credencial FCM NUNCA aquí | Sin TTL; se elimina con el usuario | `GetItem` consistente, `PutItem`/`UpdateItem` condicionales por versión | 1 RRU/WRU por operación, ítem <4 KB |
| `DEVICE#<id>` | `USER#<sub>` / `DEVICE#<uuid>` | Titular | E059 alta/rotación, E060 baja, worker FCM | `tokenCipher` AES-GCM con clave de entorno; `tokenFingerprint` SHA-256 no reversible; token en claro NUNCA se persiste ni se devuelve | Sin TTL propio: la vida la decide E060/el titular; la baja elimina `tokenCipher`/`fingerprint` y marca `active=false` | `TransactWriteItems`, `GetItem`, `Query begins_with(DEVICE#)`, `UpdateItem` condicional | ~2 WRU por alta (device + `IDEM#`), 1 RRU por consulta |
| `IDEM#PUSH#<hash>` | `USER#<sub>` / `IDEM#PUSH#<sha256(clave)>` | Titular | Replay/409 de E059 | Solo hash de clave y de payload; nunca clave ni token en claro | `expiresAt` epoch seconds (TTL de tabla, 24 h propuestas) + comprobación lógica en aplicación | `GetItem` + `PutItem attribute_not_exists` dentro de la transacción de E059 | 1 RRU + 1 WRU por alta |
| `NOTIFICATION#<UTC>#<id>` | `USER#<sub>` / `NOTIFICATION#<RFC3339Nano>#<uuid>` | Titular | E061 listado, E062 lectura individual, outbox FCM | `title`/`message` sin datos financieros no confirmados; no contiene token FCM | Sin TTLAuto; bandeja durable hasta baja del usuario | `Query` por rango, `Select=COUNT`, `UpdateItem` CAS de `isRead` | 1 RRU por página + 1 RRU por conteo |
| `READALL#<requestId>` | `USER#<sub>` / `READALL#<uuid>` | Titular | E063 lote durable y reanudación | Sin secretos | `expiresAt` 7 días (propuesto) tras `completedAt`; la respuesta 202 ya es durable | `PutItem` antes del 202, `UpdateItem` de progreso | 1 WRU por lote + 1 por actualización final |
| Outbox FCM (implementado en local) | `OUTBOX#GLOBAL` / `DUE#<RFC3339Nano>#<id>` | Worker autorizado, no cliente HTTP | Emisor FCM real; no expone endpoint público `send-push` | `eventId` determinista + deduplicación por `(userID,eventId)`; `tokenCipher` se resuelve desde `DEVICE#`; credencial FCM solo en secret store de entorno, nunca en ítem | Los pendientes se reprograman con backoff; los terminales quedan auditables bajo la política de retención aprobada | `Query` acotada por `DUE#`, `UpdateItem` condicional de lease con `owner` (`sending`/`attempts`), sin `Scan` ni GSI2 (deshabilitado) | 1 RRU por ciclo de worker + 1 WRU por transición de estado |
| Caché Redis | claves `nexum:analytics:*`, `nexum:ratelimit:*` | Compartida entre nodos | Limitación por usuario de E052–E063; sin caché de datos financieros en E057–E063 | Sin tokens, sin JWT, sin preferencias persistentes | TTL de ventana 60 s para rate limit; `volatile-lru` aprobado en I-050 | `INCR`/`EXPIRE`/Lua atómico | Sin coste DynamoDB; Redis ya presupuestado |

**Secretos de runtime:** `NEXUM_PUSH_TOKEN_KEY_B64` (o su alias `NEXUM_PUSH_TOKEN_KEY`) es la clave AES-GCM de 32 bytes de E059/E060 y del worker; `NEXUM_NOTIFICATION_CURSOR_KEY_B64` es la clave HMAC de 32 bytes que firma los cursores de E061. Ninguna se genera efímera: sin clave válida los endpoints responden 503. Ninguna se escribe en DynamoDB, logs, `memoria.md` ni este documento.

**Índices:** no se crea ninguno. Se reutilizan `PK`/`SK`, `GSI1` (sparse, ya aprobado) y el TTL físico `expiresAt`. `GSI2` permanece **deshabilitado**; el worker usa una partición dedicada con `Query`, nunca `Scan`. No se modifica Terraform.

## 2. Cambios mínimos propuestos en E014 (borrado)

Archivo de aplicación previsto: `internal/application/privacy/deletion_worker.go` (y su store en `internal/adapters/dynamo/`), sin tocar el handler HTTP ni el OpenAPI de E014.

1. Ampliar la purga por lotes para que, bajo la misma partición `USER#<sub>`, elimine también `DEVICE#*`, `IDEM#PUSH#*`, `NOTIFICATION#*`, `READALL#*` y `NOTIFICATION_PREFERENCES` con `Query` por prefijo + `BatchWriteItem`/`DeleteItem`, respetando retenciones legales.
2. Revocar dispositivos FCM **antes** de borrar el cipher: marcar `active=false` y borrar `tokenCipher`/`fingerprint`. No se afirma que Google haya revocado el token; solo que Nexum deja de usarlo (guide §H).
3. Cancelar outbox pendiente del usuario y marcarlo `revoked` para que el worker no envíe tras la purga.
4. Publicar en el recibo de E014 un `deletedFamilies` agregado (solo nombres de familia/count, sin valores), **solo si** se aprueba el cambio de esquema de respuesta; si no, mantener el contrato actual y registrar el detalle solo en auditoría interna.

**Alternativa sin tocar E014:** dejar que el TTL y la política de retención eliminen las familias nuevas de forma asíncrona. Riesgo: dispositivos activos y notificaciones pendientes podrían seguir siendo usados por el worker entre la solicitud de borrado y la purga; por eso se recomienda el cambio 1–3.

## 3. Cambios mínimos propuestos en E020 (exportación)

Archivos previstos: `internal/application/export/export.go` y el adaptador de exportación, sin tocar el contrato HTTP.

1. Añadir al payload `UserExport` (bajo una enmienda RFC aprobada) las secciones `notificationPreferences`, `pushDevices` y `notifications`:
   - `pushDevices[]`: `id`, `provider`, `deviceLabel`, `registeredAt`, `active`. **Nunca** `token`, `tokenCipher` ni `tokenFingerprint`.
   - `notificationPreferences`: los diez campos de `NotificationPreferences` ya redactados por E057.
   - `notifications[]`: `id`, `title`, `message`, `isRead`, `createdAt`; sin `readAt` ni datos de otros usuarios.
2. El `cursor` existente debe cubrir también estas secciones para no truncarlas; la paginación sigue siendo por partición `USER#<sub>`, sin `Scan`.
3. No exportar ítems `IDEM#*`, `OUTBOX#*` ni credenciales: son artefactos operativos, no datos del titular.

**Alternativa sin tocar E020:** documentar al propietario que la exportación actual no incluye preferencias/dispositivos/notificaciones y entregar esos datos por un canal administrativo aprobado. Riesgo: divergencia entre lo que la app muestra y lo que el usuario puede exportar.

## 4. Solicitud formal de Gate

`SOLICITO_GATE_TRANSVERSAL_O4: origen E052–E063 → destino E014/E020 → archivos: internal/application/privacy/deletion_worker.go, internal/adapters/dynamo/*purga*, internal/application/export/export.go, tests asociados (sin OpenAPI ni spec.md) → razón: evitar dispositivos FCM activos, outbox pendiente y notificaciones huérfanas tras borrado, y permitir exportación completa sin secretos → alternativa: TTL asíncrono + exportación administrativa → riesgo de regresión: la purga ya verificada de E014 y el cursor de E020 deben seguir pasando sus pruebas; se ejecutará la suite completa y pruebas A/B → pruebas: go test ./..., validadores H06/SDD/DevSecOps, replay de E014/E020 con dos usuarios y verificación de que ningún token cifrado aparece en respuestas → impacto AWS: solo lecturas/escrituras DynamoDB ya permitidas sobre la tabla única; sin Terraform, sin índices nuevos, sin coste Firebase adicional, sin despliegue.`

**Hasta que exista esa frase de aprobación:** `BLOCKED_INTEGRACION_TRANSVERSAL`; E014/E020 quedan intactos y no se afirma borrado/exportación total de las familias nuevas.

## 5. Estado de despliegue

`CODE_IMPLEMENTADO_EN_LOCAL=SI` · `AWS_DEPLOYED=SI` (release `v0.20.0+4bf6758d372`, bundle `v2`) · `HTTPS_VERIFIED_AWS=SI` (health 200 y 12 rutas con 401 sin JWT) · `FUNCTIONAL_VERIFIED_AWS=NO` (sin JWT) · `FCM_SERVICE_CREDENTIAL_INJECTED=SI` · `FCM_E2E_PASS=NO` (prueba con dispositivo no ejecutada por decisión humana) · `ML_GATE_APPROVED=NO` · `GATE_TRANSVERSAL_O4=PENDIENTE` · `BLOCKED_INTEGRACION_TRANSVERSAL`.
