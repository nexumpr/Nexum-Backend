# Checklist del bundle de secretos FCM (RFC-004 v3)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/CHECKLIST_BUNDLE_FCM.md` |
| Versión | 1.0.0 |
| Fecha | 2026-11-24 |
| Objeto del bundle | `s3://nexum-h08i-secretos-713797602090/nexum/secrets/<version>/secrets.json` |
| Inyector | Custodio humano (publicación S3) + bootstrap de instancia (RFC-004 opción B) |
| Estado | **CONTRATO_DOCUMENTADO · `FCM_LOADER_NOT_IMPLEMENTED` · `AWS_NOT_RUN`** |
| Contiene valores | **No.** Este documento enumera nombres, formatos y controles; nunca valores |

> El worker FCM y el scheduler **no existen todavía** en el árbol Go. Este
> checklist fija el contrato que deberá cumplir el cargador cuando se implemente
> (Oleada 4/5) y lo que el custodio debe y no debe hacer al publicar el bundle.
> No se declara que el arranque FCM esté verificado.

## 1. Variables y alias

| Variable | Obligatoria | Formato | Secreto | Consumidor previsto | Rotación |
|---|---|---|---|---|---|
| `NEXUM_FCM_PROJECT_ID` | Sí, si el push está activo | ID de proyecto de Firebase/Google Cloud (`^[a-z0-9-]{6,30}$`) | No (identificador) | Cliente FCM HTTP v1 | No rota |
| `NEXUM_FCM_CREDENTIALS_JSON` | Una de las dos fuentes | JSON **minificado** de cuenta de servicio (`type`, `project_id`, `private_key`, `client_email`, `token_uri`) | **Sí** | Cliente OAuth2/FCM | Rotar en Google Cloud y publicar versión nueva |
| `NEXUM_FCM_CREDENTIALS_FILE` (alias) | Alternativa a la anterior | Ruta **absoluta** bajo `/etc/nexum` (p. ej. `/etc/nexum/fcm-sa.json`) | **Sí** (el archivo) | Cliente OAuth2/FCM vía `os.OpenRoot("/etc/nexum")` | Igual que la anterior |
| `NEXUM_PUSH_TOKEN_KEY_B64` | Sí, si hay registro de dispositivos | Base64 estándar de **32 bytes** CSPRNG | **Sí** | Derivación/verificación del token de push (E059/E060) | Ver `PROCEDIMIENTO_ROTACION_CLAVES_FCM.md` |
| `NEXUM_NOTIFICATION_CURSOR_KEY_B64` | Sí, si hay paginación de notificaciones (E061) | Base64 estándar de **32 bytes** CSPRNG | **Sí** | Firma HMAC del cursor de notificaciones | Ver procedimiento de rotación |

Notas de contrato:

- **Exactamente una** fuente de credenciales. Si `NEXUM_FCM_CREDENTIALS_JSON` y
  `NEXUM_FCM_CREDENTIALS_FILE` están definidas a la vez, el arranque debe
  **fallar cerrado** (configuración ambigua), no elegir una en silencio.
- El alias es una **ruta**, no el contenido. Se abre con `os.OpenRoot` sobre
  `/etc/nexum` para que el sistema operativo rechace rutas relativas, `..` y
  enlaces que escapen del directorio raíz. Un valor que no empiece por
  `/etc/nexum/` se rechaza antes de abrir nada.
- Ambas claves `_B64` deben decodificar a **exactamente 32 bytes**. Base64
  válido de otra longitud es error de configuración, no se recorta ni se deriva.
- `NEXUM_FCM_PROJECT_ID` no es secreto, pero **sí** debe coincidir con el
  `project_id` del JSON; una discrepancia es error de arranque.

## 2. Quién inyecta y cómo

1. **Custodio (humano)** publica el objeto
   `nexum/secrets/<version>/secrets.json` en el bucket privado con:
   `--server-side-encryption AES256`, escritura condicional (`If-None-Match: *`
   para una clave nueva o `If-Match` sobre la versión vigente), y **sin**
   imprimir el contenido. El bucket tiene Block Public Access, SSE-S3,
   versionado y política que deniega el acceso sin TLS.
2. **Terraform** pasa `secretos_version` y el bootstrap construye
   `SECRETOS_URI=<base>/<version>/secrets.json`. El valor no contiene secretos.
3. **Bootstrap de la instancia** (`infra/terraform/modules/computo/user-data.sh.tftpl`):
   - descarga el objeto por el **gateway endpoint de S3** con el rol de instancia
     (no hay NAT ni Internet en este paso);
   - valida que el JSON sea un objeto con `version` (string no vacío) y
     `variables` (objeto no vacío);
   - exige `APP_ENV` entre las variables;
   - rechaza claves fuera de `[A-Z][A-Z0-9_]{1,63}` y valores con `\n`, `\r` o NUL;
   - escribe `/etc/nexum/backend.env` de forma atómica (`write` + `os.replace`)
     con modo **0640** y propietario **root:nexum**;
   - falla con `exit 1` si el bundle falta, está vacío o es inválido: la
     instancia no se declara sana y el ASG la reemplaza.
4. **systemd** carga `EnvironmentFile=-/etc/nexum/backend.env` en el proceso
   `nexum` (usuario y grupo `nexum`). El `-` hace el archivo opcional para
   systemd, pero el bootstrap **sí** lo exige.
5. **No hay Secrets Manager en el arranque**: `secretsmanager` sólo existe como
   interface endpoint en ventana de mantenimiento. El runtime no depende de él.

### 2.1 Estructura del bundle (sin valores)

```json
{
  "version": "v3",
  "variables": {
    "APP_ENV": "<PRODUCTION|...>",
    "AWS_REGION": "us-east-2",
    "DYNAMODB_TABLE": "<tabla>",
    "REDIS_ADDR": "<host:puerto>",
    "REDIS_USER": "<usuario>",
    "REDIS_PASSWORD": "<secreto>",
    "REDIS_TLS": "true",
    "JWT_PRIVATE_KEY_PEM_B64": "<secreto>",
    "JWT_KEY_ID": "<kid>",
    "LOGIN_RATE_LIMIT_HMAC_KEY_B64": "<secreto>",
    "PASSWORD_RESET_SEAL_KEY_B64": "<secreto>",
    "NEXUM_SES_FROM_EMAIL": "<remitente verificado>",
    "E015_EFFECTIVE_AT": "<RFC3339>",
    "NEXUM_FCM_PROJECT_ID": "<project-id>",
    "NEXUM_FCM_CREDENTIALS_FILE": "/etc/nexum/fcm-sa.json",
    "NEXUM_PUSH_TOKEN_KEY_B64": "<secreto>",
    "NEXUM_NOTIFICATION_CURSOR_KEY_B64": "<secreto>"
  }
}
```

- `version` es una **etiqueta opaca**: el bootstrap sólo exige que sea string no
  vacío. Llamar «v3» a esta revisión es una convención documental; el validador
  de arranque no compara el literal. No se debe afirmar que la instancia
  «verificó v3».
- El JSON debe ser **de una sola línea** si se inyecta como valor de
  `backend.env`: el validador rechaza `\n` y `\r` reales dentro del valor. Un
  JSON con saltos de línea se rechaza entero (no se “arregla” en el bootstrap).
- Por eso se **prefiere** `NEXUM_FCM_CREDENTIALS_FILE`: el JSON puede tener
  saltos de línea y comillas sin depender del parseo de `EnvironmentFile`.
- El archivo referenciado por el alias debe publicarse aparte con modo `0640`,
  propietario `root:nexum`, bajo `/etc/nexum`, y **no** forma parte de
  `backend.env`.

### 2.2 Brecha abierta del alias `NEXUM_FCM_CREDENTIALS_FILE`

**Hecho verificado:** el bootstrap actual **sólo** escribe
`/etc/nexum/backend.env` a partir del bundle. **No descarga ni crea** ningún
otro archivo en `/etc/nexum`. Por tanto, hoy el alias apuntaría a un archivo
**inexistente** y el arranque debe fallar cerrado.

Opciones para cerrar la brecha (decisión pendiente, ninguna implementada):

| Opción | Mecanismo | Ventaja | Coste/riesgo |
|---|---|---|---|
| A. Extender el bootstrap | Descargar un segundo objeto S3 (`.../fcm-sa.json`) con `aws s3 cp`, verificar SHA-256, `install -m 0640 -o root -g nexum` | Mantiene el JSON legible y fuera de `backend.env`; patrón idéntico al binario | Cambio en `user-data.sh.tftpl` (Terraform) y en `SECRETOS_URI`/variables; exige Gate |
| B. Inline minificado | Publicar el JSON como valor de `NEXUM_FCM_CREDENTIALS_JSON` en el bundle | No cambia el bootstrap | El validador rechaza `\n`/`\r`: obliga a minificar; el JSON queda en `backend.env` y en el entorno del proceso |
| C. No usar alias | Eliminar `NEXUM_FCM_CREDENTIALS_FILE` del contrato | Menos superficie | Pierde la opción de rotar el JSON sin tocar `backend.env` |

**Regla mientras no se cierre:** si el alias está definido y el archivo no
existe, el arranque FCM debe **fallar** con `credentials_error=unreadable`; no
debe caer a inline ni arrancar sin credenciales. No se declara
`FCM_BUNDLE_READY`.

## 3. Cómo se valida el arranque (log esperado, sin valores)

El proceso debe emitir **un** evento estructurado al iniciar el componente FCM,
sin secretos:

```json
{
  "event": "startup",
  "component": "fcm",
  "build_version": "vX.Y.Z+<sha12>",
  "fcm_project_present": true,
  "credentials_source": "file",
  "credentials_project_match": true,
  "push_key_id": "k1",
  "cursor_key_id": "k1",
  "worker": "fcm_outbox",
  "worker_started": true,
  "scheduler_started": true
}
```

Reglas de validación en arranque (todas fallan cerrado):

| Comprobación | Resultado esperado | Qué se registra si falla |
|---|---|---|
| `NEXUM_FCM_PROJECT_ID` presente y con formato | OK | `config_error=fcm_project_missing` |
| Fuente de credenciales única (JSON o archivo, no ambas) | OK | `config_error=credentials_ambiguous` |
| Ruta del alias absoluta y bajo `/etc/nexum` | OK | `config_error=credentials_path_invalid` (sin imprimir la ruta completa si se considera sensible) |
| Archivo legible y JSON parseable | OK | `credentials_error=unreadable` / `parse_error` |
| `project_id` del JSON == `NEXUM_FCM_PROJECT_ID` | OK | `config_error=project_mismatch` |
| Claves `_B64` decodifican a 32 bytes | OK | `config_error=key_length` (sin valor) |
| Lease de worker adquirido o declarado | `lease_held` true/false | `lease_held=false` no es error |
| Proceso activo | `worker_started=true` | si no, el servicio no debe declararse sano |

**Lo que se registra:** identificadores de clave (`k1`, `k2`), booleanos,
`build_version`, `instance_id`/AZ si se añaden. **Lo que nunca se registra:**
el JSON, la ruta completa del archivo de credenciales si se considera sensible,
el `private_key`, el `client_email` completo, las claves `_B64`, tokens OAuth,
tokens de dispositivo ni el contenido de un push.

Verificación operativa (sin exponer valores):

1. `journalctl -u nexum-backend --since "5 min ago" | grep '"event":"startup"'`
   → una línea por arranque, con `credentials_source` y `*_key_id`.
2. `systemctl show nexum-backend -p Environment` **no** se usa para inspección:
   volcaría secretos. Prohibido en operación.
3. Comprobación de permisos: `stat -c '%a %U:%G %n' /etc/nexum/backend.env` →
   `640 root:nexum`. Sin volcar contenido.

## 4. Qué **no** hacer

| Prohibido | Motivo |
|---|---|
| Imprimir, `echo`, `cat`, `env`, `printenv` o adjuntar el bundle / `backend.env` | Expone credenciales en logs, historial o tickets |
| Pegar valores en un chat, PR, issue, captura o informe | Exfiltración; R-019/R-020 |
| Commitear el bundle, un `tfvars` con claves o el JSON de servicio | Queda en el historial de Git, aunque se borre después |
| Guardar el JSON en `/tmp`, `%TEMP%` del host o en el repositorio | Fuera del control de permisos y con copias |
| Rotar una clave “para probar” sin `APRUEBO_ROTACION_CLAVES_FCM` | Invalida dispositivos/cursos y no hay ventana aprobada |
| Cambiar el bundle sin `instance refresh` y creer que ya se aplicó | El bootstrap escribe `backend.env` **sólo al arrancar**; el proceso no relee el bundle |
| Poner las claves en `-var`, `TF_VAR_*`, `terraform.tfvars` o en el state | El state queda en S3 y en copias de custodia |
| Usar `curl` con el token en la línea de comandos | Visible en `ps`/historial; usar cabecera por archivo temporal efímero o el propio backend |
| Incluir el `private_key` en un mensaje de error o traza | Filtración por observabilidad |
| Desactivar el redactor o subir el nivel de log a debug con cuerpos completos | Rompe la política de logs |

## 5. Compatibilidad con `backend.env` (0640 root:nexum)

| Aspecto | Hecho verificado | Implicación |
|---|---|---|
| Propietario y modo | El bootstrap escribe `0640 root:nexum` y `chmod 0640` | El proceso (`nexum:nexum`) puede leer; “otros” no |
| Lectura por systemd | `EnvironmentFile=-/etc/nexum/backend.env` | Variables disponibles en el proceso; no en unidades ajenas |
| `/etc` en sólo lectura | `ProtectSystem=strict` | El proceso no puede modificar su propia configuración |
| `PrivateTmp=true` | El servicio tiene `/tmp` privado | Un archivo temporal del proceso no es visible para otros |
| Valores con espacios/comillas | El bootstrap **no** entrecomilla; escribe `CLAVE=valor` | Preferir `*_FILE` para JSON; validar en arranque real cualquier valor con espacios |
| JSON con saltos de línea | El validador del bootstrap rechaza `\n`/`\r` | Inline sólo si está minificado; si no, usar el alias de archivo |
| Recarga en caliente | No existe: el bundle se lee en cloud-init | Cualquier cambio exige `instance refresh` o reemplazo de instancia |
| Número de variables | El bundle actual tiene 13; éste añade 4 (o 3 + alias) | Aumenta el tamaño de `backend.env`, no su modo; sigue siendo legible por el servicio |

## 6. Estado de verificación

| Elemento | Estado | Evidencia |
|---|---|---|
| Validación del bundle en bootstrap (estructura, claves, `APP_ENV`, sin `\n`) | **Implementado** | Lectura de `user-data.sh.tftpl` (2026-11-24) |
| Escritura atómica 0640 root:nexum | **Implementado** | Ídem |
| `EnvironmentFile` en la unidad | **Implementado** | `infra/systemd/nexum-backend.service` |
| Cargador Go de las 4 variables + alias `os.OpenRoot` | **`NOT_IMPLEMENTED`** | No hay código FCM en el árbol |
| Evento `startup` de FCM | **`NOT_IMPLEMENTED`** | Idem |
| Publicación del bundle v3 en S3 | **`NOT_RUN`** | Requiere Gate de publicación |
| Creación de `/etc/nexum/fcm-sa.json` (alias) | **`NOT_IMPLEMENTED`** | El bootstrap sólo escribe `backend.env` (§2.2) |
| Arranque real con las variables | **`NOT_RUN`** | Requiere release + refresh |
| Verificación de que un valor con espacios sobrevive a `EnvironmentFile` | **`NOT_RUN`** | Debe probarse antes de usar inline JSON |

**Conclusión:** el mecanismo de inyección existe y es verificable por lectura;
el consumo por el backend FCM **no**. No se declara `FCM_BUNDLE_READY` hasta que
existan cargador, evento de arranque y una instancia real que los emita.
