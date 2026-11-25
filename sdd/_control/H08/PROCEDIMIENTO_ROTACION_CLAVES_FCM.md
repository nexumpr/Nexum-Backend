# Procedimiento de rotación de claves FCM y de cursor de notificaciones

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/PROCEDIMIENTO_ROTACION_CLAVES_FCM.md` |
| Versión | 1.0.0 |
| Fecha | 2026-11-24 |
| Claves afectadas | `NEXUM_PUSH_TOKEN_KEY_B64` y `NEXUM_NOTIFICATION_CURSOR_KEY_B64` |
| Estado | **`PROCEDIMIENTO_DOCUMENTADO · NO_EJECUTADO · FCM_LOADER_NOT_IMPLEMENTED`** |
| Gate requerido | `APRUEBO_ROTACION_CLAVES_FCM` (no concedida) |
| Ventana | Mantenimiento declarado; dos nodos sanos antes y después |
| Prohibido en esta unidad | Generar claves reales, publicar bundle, `terraform apply`, `instance refresh`, tocar AWS |

> **No se ha rotado ninguna clave.** Este documento describe el procedimiento y
> sus Gates. Ninguna de sus fases se ejecutó. No contiene valores, ni claves de
> ejemplo utilizables, ni identificadores reales.

## 1. Por qué estas dos claves se rotan distinto

| Clave | Uso previsto | Si cambia sin transición | Rotación segura |
|---|---|---|---|
| `NEXUM_PUSH_TOKEN_KEY_B64` | Derivar/verificar el identificador del token de push almacenado (E059/E060). Protege el token en reposo y evita guardarlo en claro | Los registros existentes dejan de localizarse: un dispositivo ya registrado no se encuentra y se duplica o se pierde el envío | **Coexistencia obligatoria** (clave nueva + anterior) hasta que no queden registros con la clave vieja |
| `NEXUM_NOTIFICATION_CURSOR_KEY_B64` | Firmar el cursor opaco de `GET /api/v1/notifications` (E061) | Los cursores emitidos antes dejan de verificar: la página siguiente responde error de cursor | Coexistencia **opcional**; si no se hace, el cliente reinicia la paginación (no hay pérdida de datos) |

La rotación **no** es un cambio de contraseña de usuario ni de JWT: no revoca
sesiones, no cambia `sessionEpoch` y no afecta a E001-E014.

## 2. Decisiones que deben cerrarse antes de implementar

`FCM_LOADER_NOT_IMPLEMENTED`: el árbol no tiene todavía el cargador ni el
almacenamiento. Estas decisiones son **requisito de diseño**, no un hecho:

| ID | Decisión | Opciones | Recomendación | Estado |
|---|---|---|---|---|
| D-ROT-01 | ¿El token de push se guarda como HMAC (búsqueda) o cifrado (recuperación)? | HMAC con `key_id` / cifrado AEAD con `key_id` | **HMAC con `key_id`** (no se necesita recuperar el token; sólo comparar) | `PENDIENTE` |
| D-ROT-02 | ¿Coexistencia de clave anterior? | Sí (dos claves) / No (rotación break-glass) | **Sí**, mínimo 30 días o hasta que no haya registros con `k_old` | `PENDIENTE` |
| D-ROT-03 | ¿Cómo se identifica la versión de clave en los datos? | Campo `key_id` (`k1`,`k2`) junto al valor | **Campo `key_id`**, nunca inferido por prueba y error | `PENDIENTE` |
| D-ROT-04 | Cursor: ¿coexistencia? | Aceptar `k_old` y `k_new` durante 24–72 h / invalidar de inmediato | **Aceptar ambas 72 h**; el cursor lleva `kid` y `exp` | `PENDIENTE` |
| D-ROT-05 | ¿Se permite rotación de emergencia sin coexistencia? | Sí, con pérdida de cursores y re-registro / No | **Sí**, declarando el efecto y con Gate específico | `PENDIENTE` |

Sin estas decisiones implementadas, la rotación **no puede ejecutarse**: no
habría forma de leer datos antiguos.

## 3. Nombres de variables durante la transición

El bundle es un mapa plano `CLAVE=valor`. Para sostener dos claves a la vez se
requiere este contrato (a implementar en el cargador):

| Variable | Fase | Contenido |
|---|---|---|
| `NEXUM_PUSH_TOKEN_KEY_B64` | Siempre | Clave **actual** (con la que se firma/deriva) |
| `NEXUM_PUSH_TOKEN_KEY_ID` | Siempre | Identificador corto de la actual (`k1`, `k2`, …) |
| `NEXUM_PUSH_TOKEN_KEY_PREVIOUS_B64` | Sólo coexistencia | Clave anterior, sólo para **verificar/leer** |
| `NEXUM_PUSH_TOKEN_KEY_PREVIOUS_ID` | Sólo coexistencia | Identificador de la anterior |
| `NEXUM_NOTIFICATION_CURSOR_KEY_B64` | Siempre | Clave actual del cursor |
| `NEXUM_NOTIFICATION_CURSOR_KEY_ID` | Siempre | Identificador de la actual |
| `NEXUM_NOTIFICATION_CURSOR_KEY_PREVIOUS_B64` | Sólo coexistencia | Clave anterior del cursor |
| `NEXUM_NOTIFICATION_CURSOR_KEY_PREVIOUS_ID` | Sólo coexistencia | Identificador de la anterior |

Reglas:

- **Firmar/emitir siempre con la actual.** Verificar/leer con actual **y**
  anterior mientras exista la anterior.
- Un `key_id` desconocido en datos o cursor es error controlado (no se prueba
  con todas las claves en bucle).
- Las claves son 32 bytes CSPRNG en base64 estándar; el identificador es
  metadata, no secreto.
- Sin estos nombres en el cargador, **no** se intenta una rotación “a mano”.

## 4. Procedimiento (fases y Gates)

### Fase 0 — Precondiciones (lectura, sin mutación)

1. Dos instancias `InService/Healthy`, ASG `2/2/2`, sin `instance refresh`
   activo, dos targets `healthy`.
2. Release vigente identificado (`vX.Y.Z+sha12`) y versión de bundle vigente
   (S3 `VersionId`) registradas.
3. Copia de custodia del bundle actual y de su `VersionId` **fuera** del
   repositorio, cifrada, con SHA-256 anotado (sin abrir el contenido en claro).
4. Capacidad verificada: la rotación exige reemplazar las dos instancias. Si
   `us-east-2b` no tiene capacidad `t4g.micro` (incidente recurrente), se
   planifica la ventana con la restricción temporal a `us-east-2a`, declarando
   la pérdida temporal de multi-AZ.
5. `APRUEBO_ROTACION_CLAVES_FCM` concedida con: claves afectadas, ventana,
   duración de coexistencia, responsable y frase de rollback.

### Fase 1 — Generación (custodio, fuera de OpenCode)

1. Generar **dos** claves nuevas de 32 bytes con CSPRNG del sistema operativo.
2. Codificar en base64 estándar y asignar `k2` (o el siguiente número).
3. **No** imprimirlas en pantalla compartida, ni pegarlas en el chat, ni
   guardarlas en el repositorio o en `%TEMP%` del host.
4. Conservarlas sólo en el gestor/custodia autorizado hasta publicarlas.

### Fase 2 — Publicación del bundle con coexistencia

1. Partir del bundle vigente y **preservar todas** las variables existentes
   (`APP_ENV`, AWS, DynamoDB, Redis, JWT, SES, E015, FCM project, alias de
   credenciales) sin cambios.
2. Añadir la clave nueva como actual + `_ID`, y mover la anterior a
   `_PREVIOUS_B64` + `_PREVIOUS_ID`.
3. Publicar con `--server-side-encryption AES256` y condición `If-Match` sobre
   la versión vigente (o `If-None-Match: *` si se cambia de prefijo).
4. Verificar sólo metadatos: `VersionId`, `ETag`, tamaño, fecha. **Nunca** el
   contenido.
5. Registrar el `VersionId` nuevo. Si algo falla, el objeto anterior sigue
   siendo la versión vigente: no hay estado intermedio.

### Fase 3 — Rollout para que las instancias lean el bundle

El bundle se lee **en cloud-init**, no en caliente. Dos caminos:

- **Preferido (sin Terraform):** `instance refresh` con la misma versión de
  Launch Template y `SkipMatching=false`, para forzar el reemplazo aunque las
  instancias ya coincidan con la plantilla.
- **Alternativo:** nueva versión de Launch Template (mismo binario y unidad) y
  refresh normal. Exige plan Terraform, Gate por SHA-256 y actualización de
  `release_objetivo`/ARN si aplica.

Parámetros del refresh (los ya usados en el proyecto):

```text
Estrategia: Rolling
MinHealthyPercentage: 100
MaxHealthyPercentage: 150   (Max 3 transitorio; restaurar a 2 al terminar)
InstanceWarmup: 120
ScaleInProtectedInstances: Refresh
SkipMatching: false
```

Durante el reemplazo: service sin interrupción (siempre ≥ 2 sanas si la
capacidad lo permite), y **no** se cambia el binario ni el contrato.

### Fase 4 — Verificación

| Comprobación | Cómo | Criterio |
|---|---|---|
| Arranque con clave nueva | `journalctl`/CloudWatch: evento `startup` con `push_key_id=k2` y `cursor_key_id=k2` | Ambos nodos, sin valores |
| Clave anterior cargada | `startup` con `previous_key_loaded=true` | Sólo durante coexistencia |
| Dispositivo registrado antes de rotar sigue recibiendo | Envío de prueba a un dispositivo sintético registrado con `k1`; outbox `failed=0` | Entrega aceptada por FCM; `200`/`message_id` |
| Cursor emitido antes de rotar | Llamar `GET /notifications` con el cursor viejo | Aceptado durante coexistencia; tras retirar la anterior, error de cursor controlado (400), nunca 500 |
| Cursor nuevo | Paginar dos páginas con el cursor recién emitido | 200 y sin duplicados |
| Sin fugas | Buscar en logs `k1`/`k2` (ids OK), claves, `Bearer`, JSON de servicio | 0 coincidencias de valores |
| Estado de flota | Dos instancias `InService/Healthy`, ASG `2/2/2`, 2 targets | Restaurado |

### Fase 5 — Retirada de la clave anterior

1. Esperar la ventana de coexistencia aprobada (≥ 30 días para push; 72 h para
   cursor, o lo que fije el Gate).
2. Confirmar que **no quedan** datos con `key_id` anterior:
   - push: consulta por `key_id` (no `Scan`; índice/partición adecuada);
   - cursor: no hay estado persistente, basta el plazo.
3. Publicar bundle **sin** `_PREVIOUS_*`, con `If-Match` sobre la versión en
   vigor.
4. Refresh con los mismos parámetros; verificar `previous_key_loaded=false`.
5. Conservar las versiones S3 antiguas según la política de retención; **no**
   borrar el objeto que contiene la clave retirada hasta que el Gate lo
   autorice (sirve de rollback).

### Fase 6 — Cierre

1. Registrar: fecha, `VersionId` antes/después, IDs de refresh, claves por
   **identificador** (nunca valor), verificaciones y desviaciones.
2. Actualizar `memoria.md` en la siguiente unidad documental autorizada.
3. Declarar `ROTACION_FCM_VERIFIED` sólo si todas las comprobaciones de la
   Fase 4 pasaron con evidencia real. Si no, `ROTACION_FCM_PARTIAL`.

## 5. Rollback

| Punto de fallo | Acción | Efecto |
|---|---|---|
| Fase 2 (bundle inválido o publicación fallida) | No hay cambio: la versión anterior sigue vigente | Ninguno |
| Fase 3 (refresh falla o nodo no arranca) | `cancel-instance-refresh`; si el nodo no arranca, restaurar el bundle anterior (`VersionId` registrado) y refresh | Vuelve a `k1`; los datos escritos con `k2` durante la ventana dejan de leerse si el código no soporta ambas |
| Fase 4 (envíos fallan) | Restaurar bundle anterior + refresh; si el fallo es de credenciales FCM, no de clave, corregir credenciales, no rotar | Los cursores emitidos con `k2` se invalidan |
| Fase 5 (se retiró la anterior y algo depende de ella) | Republicar la versión anterior del bundle (con `_PREVIOUS_*`) + refresh | Recupera lectura; **no** recupera datos borrados |
| Clave comprometida | Rotación break-glass **sin** coexistencia: nueva clave, invalidar cursores, forzar re-registro de dispositivos, revocar credenciales de servicio en Google | Pérdida de push hasta re-registro; requiere Gate de incidente |

**Límite del rollback:** una vez que la clave anterior se elimina del bundle y
del almacén, no se puede “des-rotar” sin volver a publicarla. Por eso la Fase 5
exige confirmación de que no quedan datos dependientes.

## 6. Re-registro de dispositivos

- `POST /api/v1/devices/push-tokens` (E059) es un **upsert** por dispositivo; el
  cliente reenvía el token al abrir la app. No existe push de “re-regístrate”.
- Con coexistencia (`key_id` + clave anterior) **no** hace falta re-registro:
  el servidor sigue localizando el registro antiguo.
- Sin coexistencia (break-glass o decisión D-ROT-05), el re-registro es
  obligatorio y su cobertura es la de los usuarios que abran la app; se declara
  como pérdida temporal de entregas, no como éxito.
- La rotación **no** debe provocar duplicados de dispositivo: el upsert debe
  actualizar el registro existente por `deviceRegistrationId`/`deviceId`, no
  crear uno nuevo por cambiar la clave.

## 7. Prohibiciones

1. No imprimir, copiar ni adjuntar claves en ningún paso.
2. No commitear el bundle, ni un `tfvars`, ni capturas con valores.
3. No rotar durante un incidente abierto (salvo break-glass con Gate).
4. No retirar la clave anterior antes de la ventana aprobada.
5. No borrar versiones S3 del bundle sin Gate.
6. No cambiar el contrato de E059/E061 en la misma unidad que la rotación.
7. No declarar `ROTACION_FCM_VERIFIED` sin eventos reales de ambos nodos.

## 8. Estado

| Elemento | Estado |
|---|---|
| Procedimiento escrito | **Sí** (este documento) |
| Decisiones D-ROT-01..05 | **Pendientes** |
| Cargador de doble clave en Go | **`NOT_IMPLEMENTED`** |
| Gate `APRUEBO_ROTACION_CLAVES_FCM` | **No concedida** |
| Ejecución | **`NOT_RUN`** |
