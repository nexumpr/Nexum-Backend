# Operación FCM en AWS — Oleada 4 (E057–E063)

**Rama:** `liz-oleada4` · **Autoridad:** guía V2 §6.1–§6.12/F + OpenAPI + `FIREBASE_FCM_OLEADA4_RUNBOOK.md`
**Fecha:** 2026-11-22 · **Estado:** `FCM_SERVICE_CREDENTIAL_INJECTED=NO` · `AWS_DEPLOYED=NO` · `FCM_E2E_PASS=NO`

> Documento operativo para el responsable de AWS. **No contiene secretos.** El JSON de cuenta de servicio y las claves B64 se generan y permanecen fuera del repositorio. Ningún valor se pega en chat, `memoria.md`, evidencia ni Terraform state.

---

## 0. Resumen de lo que ya está armado

| Componente | Dónde está | ¿Requiere acción en AWS? |
|---|---|---|
| Emisor HTTP v1 + OAuth2 JWT RS256 | `internal/infrastructure/push/fcm.go` | No |
| Outbox durable, lease y deduplicación | `internal/application/pushoutbox/`, `internal/adapters/dynamo/push_outbox_store.go` | No |
| Consentimiento E057/E058 (fail-closed) | `cmd/server/fcm_oleada4.go` | No |
| Bandeja E061–E063 | `internal/adapters/dynamo/notifications_store.go` | No |
| Modelo K-Means `kmeans-v1` | `ml/artifact.go` (`go:embed` dentro del binario) | **No**: no hay Python ni archivos ML en el runtime |
| Inyección de secretos al proceso | `infra/terraform/modules/computo/user-data.sh.tftpl` → `/etc/nexum/backend.env` → `infra/systemd/nexum-backend.service` | Sí: publicar nueva versión del bundle |

**Conclusión:** el código está listo. Lo único que falta para que FCM funcione desplegado es **inyectar la credencial de servidor** en el bundle de secretos.

---

## 1. Verificación en Firebase / Google Cloud

### 1.1 Paquete Android `com.nexum.nexum`

1. Entra a **Firebase Console** → selecciona el proyecto `proyectonexum-ee910`.
2. Clic en **⚙️ Project settings** → pestaña **Your apps**.
3. Busca la tarjeta **Android** con el package name **`com.nexum.nexum`**.
   - Si aparece: listo. Descarga el `google-services.json` desde esa misma tarjeta y entrégalo al equipo Android por canal autorizado. **No lo subas al repo.**
   - Si no aparece: **Add app → Android**, escribe exactamente `com.nexum.nexum`, registra y descarga el `google-services.json`.
4. En la app deben existir: permiso `POST_NOTIFICATIONS` (Android 13+), canal de notificaciones, obtención del token con `FirebaseMessaging.getInstance().token` y renovación en `onNewToken()`.

### 1.2 API de FCM habilitada

1. **Google Cloud Console** → selecciona el mismo proyecto `proyectonexum-ee910`.
2. **APIs & Services → Enabled APIs & Services** → busca **Firebase Cloud Messaging API**.
   - Si dice **Enabled**: listo.
   - Si no aparece: **APIs & Services → Library** → busca *Firebase Cloud Messaging API* → **Enable**.
3. Alternativa rápida: desde la Library, el botón muestra *Manage* cuando ya está habilitada y *Enable* cuando no.

### 1.3 Cuenta de servicio con rol mínimo

Ya creaste la cuenta con rol **Firebase Messaging Admin**. Verifícalo aquí:

- Google Cloud Console → **IAM & Admin → Service Accounts** → abre la cuenta creada → pestaña **Permissions**: debe figurar `Firebase Messaging Admin` (`roles/firebase.messagingAdmin`). No debe tener Owner.
- Firebase Console → ⚙️ Project settings → **Service accounts**: también aparece la lista de cuentas. El botón **Generate new private key** genera el JSON; ya lo descargaste en `C:\secretos-nexum\fcm-key.json`.

Comprueba, **sin mostrar el contenido**, que el `project_id` del JSON coincide con el project ID configurado:

```powershell
$j = Get-Content "C:\secretos-nexum\fcm-key.json" -Raw | ConvertFrom-Json
Write-Output ("PROJECT_ID_MATCH=" + ($j.project_id -eq "proyectonexum-ee910"))
Write-Output ("HAS_PRIVATE_KEY=" + [bool]$j.private_key)
Write-Output ("HAS_CLIENT_EMAIL=" + [bool]$j.client_email)
```

Debe imprimir `True` en las tres líneas. Si `PROJECT_ID_MATCH=False`, el envío fallará aunque la credencial sea válida.

---

## 2. Preparar el bundle `v2` (corrección incluida)

### 2.1 Descargar el bundle actual

1. AWS Console → **S3** → abre el bucket de secretos (`nexum-<entorno>-secretos-<cuenta>`).
2. Ruta: `nexum/secrets/v1/secrets.json` → **Download**.
3. Copia el archivo descargado a `C:\secretos-nexum\secrets.json` (junto a `fcm-key.json`).
4. Guarda una copia de seguridad local: `secrets-v1-backup.json`.

### 2.2 Script corregido

El script que circuló es **casi correcto**, pero en **Windows PowerShell 5.1** `Set-Content -Encoding UTF8` escribe **BOM** (`EF BB BF`). El bootstrap lee el bundle con Python `encoding="utf-8"` y un BOM provoca `JSONDecodeError`, por lo que la instancia falla y el ASG la reemplaza. La corrección está en la última línea.

Verificado con datos sintéticos: el valor `NEXUM_FCM_CREDENTIALS_JSON` queda en **una sola línea** con `\n` escapados, que es lo que exige el validador del bootstrap (`user-data.sh.tftpl`, líneas 117–124). El `private_key` recupera sus saltos reales al hacer `json.Unmarshal` en Go, que es lo que necesita la firma RS256.

Pega este bloque en PowerShell:

```powershell
$rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
$b1 = New-Object byte[] 32; $rng.GetBytes($b1); $pushKey = [Convert]::ToBase64String($b1)
$b2 = New-Object byte[] 32; $rng.GetBytes($b2); $cursorKey = [Convert]::ToBase64String($b2)

$secrets = Get-Content "C:\secretos-nexum\secrets.json" -Raw | ConvertFrom-Json
$fcmJson = (Get-Content "C:\secretos-nexum\fcm-key.json" -Raw | ConvertFrom-Json) | ConvertTo-Json -Compress

$secrets.version = "v2"
$secrets.variables | Add-Member -NotePropertyName "NEXUM_FCM_PROJECT_ID" -NotePropertyValue "proyectonexum-ee910" -Force
$secrets.variables | Add-Member -NotePropertyName "NEXUM_FCM_CREDENTIALS_JSON" -NotePropertyValue $fcmJson -Force
$secrets.variables | Add-Member -NotePropertyName "NEXUM_PUSH_TOKEN_KEY_B64" -NotePropertyValue $pushKey -Force
$secrets.variables | Add-Member -NotePropertyName "NEXUM_NOTIFICATION_CURSOR_KEY_B64" -NotePropertyValue $cursorKey -Force

$s = $secrets | ConvertTo-Json -Depth 10
[System.IO.File]::WriteAllText("C:\secretos-nexum\secrets.json", $s, (New-Object System.Text.UTF8Encoding($false)))
Write-Output "V2_GENERADA_SIN_BOM"
```

> La última línea sustituye `Set-Content ... -Encoding UTF8`. Escribe UTF-8 **sin BOM**, que es lo que acepta el bootstrap. No cambies las demás líneas: ya conservan todas las variables del `v1` y generan claves aleatorias de 32 bytes.

### 2.3 Validar antes de subir (no imprime secretos)

```powershell
py -3 -c "import json;from pathlib import Path;p=Path(r'C:\secretos-nexum\secrets.json');r=p.read_bytes();assert not r.startswith(b'\xef\xbb\xbf'),'BOM';d=json.loads(r.decode('utf-8'));req={'APP_ENV','NEXUM_FCM_PROJECT_ID','NEXUM_FCM_CREDENTIALS_JSON','NEXUM_PUSH_TOKEN_KEY_B64','NEXUM_NOTIFICATION_CURSOR_KEY_B64'};assert req<=set(d['variables']),req-set(d['variables']);[(_ for _ in ()).throw(AssertionError(k)) for k in req if not isinstance(d['variables'][k],str) or '\n' in d['variables'][k] or '\r' in d['variables'][k]];c=json.loads(d['variables']['NEXUM_FCM_CREDENTIALS_JSON']);assert c['project_id']==d['variables']['NEXUM_FCM_PROJECT_ID'];print('BUNDLE_PREFLIGHT_OK')"
```

Si imprime `BUNDLE_PREFLIGHT_OK`, el archivo es apto. Cualquier error significa que no se debe subir.

---

## 3. Subir a S3

1. Consola AWS → S3 → bucket de secretos → entra a `nexum/secrets/`.
2. **Create folder** → `v2`.
3. Entra a `v2` → **Upload → Add files** → selecciona `C:\secretos-nexum\secrets.json`.
4. En **Properties → Server-side encryption**, confirma **SSE-S3 (Amazon S3-managed keys)**. No uses KMS propio.
5. **Upload**. No hagas público el objeto; debe conservar la política privada del bucket.
6. **No borres** `v1`: es el rollback.

---

## 4. Aplicar en Terraform (con Gate humano)

No hay que crear recursos nuevos ni cambiar IAM: el rol de instancia ya tiene `GetObject` sobre `nexum/secrets/*`.

```powershell
cd infra/terraform/envs/h08i
terraform plan -var='secretos_version=v2'
```

Revisa el plan. Lo esperado:

- Actualización del **launch template** (cambia `user_data` porque cambia `secretos_uri`).
- Posible **instance refresh** (`instance_refresh` con estrategia `Rolling`, `min_healthy_percentage=100`, `max_healthy_percentage=200`).
- **No** debe haber borrado de DynamoDB, Redis, ALB, buckets, IAM ni state.

Si el plan muestra bajas o reemplazos inesperados, **detén y escala**. Si todo es coherente y hay Gate de release firmado:

```powershell
terraform apply -var='secretos_version=v2'
```

> Usar `-var` evita editar `variables.tf` y hace el rollback trivial. La instancia nueva descarga `v2`, genera `/etc/nexum/backend.env` con permisos `0640 root:nexum` y reinicia el servicio. Las instancias antiguas **no leen el bundle nuevo** hasta ser reemplazadas; verifica que el refresh termine con dos nodos sanos.

### 4.1 Ejecución real del rollout (2026-11-23, con Gates explícitos)

- Gate 1 (humanos): autorización para aplicar el plan `tfplan-v2c`, SHA-256 `3e5a469091dfd1cf79e897632cb4402d9d8248651d3f0a93f0255bab91c2c492`. Plan **0 altas / 5 cambios / 0 bajas**; el único cambio funcional fue `SECRETOS_URI` de `v1` a `v2` en el `user_data` (158 líneas antes/después, diff de 5 líneas). La política IAM conservó sus 34 ARN de rollback. Se descartó el plan inicial con defaults porque proponía destruir la aplicación.
- `terraform apply -auto-approve` sobre ese plan: **Apply complete! 0 added, 5 changed, 0 destroyed**. Launch template `lt-05445f32a7ee8a2f7` pasó a la versión **18**, con `SECRETOS_URI="s3://nexum-h08i-secretos-713797602090/nexum/secrets/v2/secrets.json"`.
- Terraform **no disparó instance refresh**. El primer refresh manual (`61ab3404-4895-41cb-a96e-aa0d2f3c3c9e`) quedó bloqueado al 0 % porque `protect_from_scale_in = true`: AWS indicó `is protected. Remove instance scale-in protection to continue`. Se canceló (`Cancelled`) sin impacto de servicio; las instancias seguían sanas con `v1`.
- Gate 2 (humano): autorización para desproteger temporalmente las dos instancias, relanzar el refresh y volver a protegerlas. Refresh `4f95effd-6acf-4f04-a883-211283c34640`: **Successful 100 %**, estratégia Rolling (`min_healthy=100`, `max_healthy=200`, `warmup=120`), ASG final 2/2/2. Protección `ProtectedFromScaleIn=True` restaurada en ambas instancias (`i-06838d26d116db732`, `i-0db7e5679704d21b9`), ambas `InService`/`Healthy`.
- Evidencia de inyección sin secretos: `aws ec2 get-console-output` de ambas instancias muestra `[bootstrap] configuracion instalada (version v2, 17 variables)`, además del SHA del binario `acd45cf9…ac553213` y de la unidad `6fa4d5fb…e15705cc`, idénticos a los declarados. No se leyó ni imprimió `/etc/nexum/backend.env`; no hay agente SSM en las instancias.
- HTTPS por CloudFront (`https://d2lpmqy5w6x9es.cloudfront.net`): `/api/v1/health/live` → **200** `{"status":"UP"}`; `/api/v1/health/ready` → **200** `{"status":"UP","dependencies":{"dynamodb":"UP","redis":"UP"}}`.
- `CloudWatch Logs /nexum/h08i/backend` no tiene streams, así que no hay evidencia de arranque del worker FCM por logs. Además, el binario desplegado `v0.19.2+3098f379d7ab` es **anterior** al commit `9b5d950` que implementa el emisor FCM; por tanto **el worker FCM no está ejecutándose en producción**. El bundle `v2` deja la configuración lista, pero hace falta publicar y desplegar un binario Oleada 4 con Gate propio para activarlo.

### Rollback

1. `terraform apply -var='secretos_version=v1'` con plan y Gate propios. Implica otro refresh y una breve interrupción; **no** es una operación de impacto cero.
2. Si el refresh no se dispara solo, ejecútalo manualmente. Si las instancias están protegidas, repite el procedimiento de desprotección temporal con Gate, y restaura la protección al terminar.
3. No elimines el objeto `v2` hasta confirmar que el rollback funcionó.

---

## 5. Verificación posterior (solo lectura)

1. `curl -fsS https://<ALB>/api/v1/health/live` y `/ready` en los dos nodos.
2. Revisar logs del servicio **sin imprimir secretos**: debe aparecer `FCM Oleada 4: worker iniciado` y **no** `FCM_CREDENCIAL_AUSENTE` ni `FCM_WORKER_NO_INICIADO`.
3. `GET /api/v1/analytics/recommendations` con JWT válido: la respuesta debe traer `"modelVersion":"kmeans-v1"`. Eso confirma que el modelo embedido carga en producción.
4. Registrar en la evidencia: `FCM_SERVICE_CREDENTIAL_INJECTED=SI`, `NEXUM_FCM_PROJECT_ID=<id>`, versión del bundle `v2`, fecha y hash del objeto. **Nunca** el contenido del JSON ni las claves.
5. La verificación final de entrega (`FCM_E2E_PASS=SI`) sigue exigiendo una notificación real recibida en `com.nexum.nexum`. Sin ese hecho, el estado honesto es "servidor configurado, entrega no verificada".

---

## 6. Reglas de seguridad

- El JSON de cuenta de servicio y las claves B64: **solo** en `C:\secretos-nexum\` local y en el bucket privado cifrado.
- Prohibido: pegarlos en chat, `memoria.md`, evidencia, `spec.md`, `plan.md`, OpenAPI, `.tfvars` o Terraform state.
- Rotación: si la credencial se expone, revocar la clave en Google Cloud IAM y publicar un `v3`.
- Credencial con rol `Firebase Messaging Admin`, sin Owner, sin legacy server key.
- Sin merge a `main`, PR, AWS mutation ni despliegue sin Gate humano separado.

---

## 7. Estados a reportar

`FIREBASE_CONSOLE_ACCESS=SI` · `ANDROID_DEVICE_READY=SI` · `FIREBASE_MESSAGING_ADMIN_CREADO=SI` · `BUNDLE_V2_PREFLIGHT=OK` · `V2_PUBLICADO=SI` · `V1_PRESERVADO=SI` · `FCM_SERVICE_CREDENTIAL_INJECTED=SI` (bootstrap v2, 17 variables, verificado en consola; contenido no leído) · `FCM_WORKER_ACTIVO=NO` (binario `v0.19.2+3098f379d7ab` anterior al código FCM) · `FCM_CONFIGURED=PARCIAL` · `FCM_E2E_PASS=NO` · `ML_GATE_APPROVED=NO` · `AWS_DEPLOYED=SI` (configuración v2 aplicada; release sin cambiar) · `HTTPS_VERIFIED=SI` · `FUNCTIONAL_VERIFIED=NO` (recommendations con JWT no ejecutado: el binario no contiene `kmeans-v1`).

Para activar el worker FCM hace falta un **Gate nuevo** de publicación de artefacto Oleada 4, plan con las URI/SHA del binario y la unidad, apply y refresh; después sí aplican las comprobaciones de la sección 5.
