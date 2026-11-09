# RFC-004 — Mecanismo de secretos para reemplazo automático de instancias

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/RFC-004-SECRETOS_BOOTSTRAP.md` |
| Fecha | 2026-11-06 |
| Alcance | I-000 (diseño E), I-020 (cómputo), I-060 (IAM/secretos) |
| Estado | **APROBADA (2026-11-09)** por `APRUEBO_RFC004_OPCION_B` — implementación local en curso; AWS no ejecutado |
| Bloquea | **Despliegue** de I-020/I-030, no la implementación local |

## 1. Defecto demostrado

El diseño E de I-000 aprobó: `secretsmanager` **sólo en ventanas** y caché cifrada en **EBS**.
Con un Auto Scaling Group (`min = desired = max = 2`) eso **no se sostiene**:

| Escenario | Qué ocurre | Consecuencia |
|---|---|---|
| Reemplazo por fallo (ASG lanza instancia nueva) | La instancia nueva **no tiene** el EBS de la anterior ni su caché; el endpoint `secretsmanager` está deshabilitado fuera de ventana | El proceso no arranca → la capacidad cae por debajo de 2 hasta abrir ventana |
| Reemplazo por instance refresh | Igual: volumen nuevo | Igual |
| Reinicio de la misma instancia | La caché sobrevive | OK |
| Rotación | Requiere ventana | OK si se planifica |
| Pérdida/corrupción de caché | Sin endpoint no hay recuperación | Degradación total de esa instancia |

**Conclusión:** la afirmación "caché en EBS resuelve la recuperación" es **falsa** para instancias
efímeras. Queda registrada como bloqueo de despliegue (no de la implementación local de I-020).

## 2. Opciones evaluadas

| Opción | Mecanismo | Coste 45 d | Disponibilidad en reemplazo | Seguridad | Veredicto |
|---|---|---|---|---|---|
| **A** Hacer permanente `secretsmanager` | 1 interface endpoint × 2 AZ × 1080 h × 0.01 | **+21.6000** (total requerido 134.4673 > 120) | Sí | Alta (KMS gestionado) | **Inviable financieramente** |
| **B** Secretos en S3 cifrado (SSE-S3) leído por gateway endpoint | Objeto creado fuera de banda por el operador; IAM restringe a un prefijo; TLS en tránsito; SSE-S3 en reposo | **0** (gateway endpoint ya existe) | Sí, sin depender de ventanas | Media-alta (sin KMS CMK propio; acceso por rol) | **Recomendada** |
| **C** Secreto embebido en AMI | — | 0 | Sí | **Rechazada** (secreto en artefacto, imposible rotar) | No |
| **D** `ssm` + Parameter Store SecureString permanente | Interface endpoint ssm (+21.60) | +21.60 | Sí | Alta | Inviable (mismo coste que A) |
| **E** Mantener EBS y aceptar caída de capacidad | — | 0 | No | — | Incumple "2 instancias simultáneas" |

## 3. Propuesta (opción B)

1. El operador (humano) deposita el bundle de secretos en
   `s3://<bucket-privado>/nexum/secrets/<version>/secrets.json` con SSE-S3, bucket con Block Public
   Access y política que sólo permite `s3:GetObject` al rol de instancia sobre ese prefijo.
2. El bootstrap (ya implementado en `infra/terraform/modules/computo/user-data.sh.tftpl`) descarga el
   **binario** por S3 y verifica SHA-256; se extiende el mismo patrón al bundle.
3. El servicio escribe el bundle en `/etc/nexum/` con `0640 root:nexum` y lo carga al arrancar; nunca
   se registra en logs.
4. Rotación: el operador publica una versión nueva y ejecuta `instance_refresh`; no requiere endpoint.
5. `secretsmanager` puede seguir usándose **en ventana** para rotación auditada si se desea, pero deja
   de ser requisito de arranque.

**Coste incremental: 0 USD.** No se necesitan endpoints nuevos ni NAT.

## 4. Impacto normativo

- Cambia el mecanismo de entrega de secretos respecto a "Secrets Manager + caché EBS" descrito en
  I-000 §9/ADR. **Requiere aprobación humana** (este RFC) antes de implementarse.
- No cambia GOV-09/GOV-18: sigue habiendo cifrado en reposo, TLS en tránsito, IAM de mínimo
  privilegio, rotación y ausencia de secretos en Git/state/AMI/user-data/logs.
- I-060 debe definir el rol y la política S3; I-020 deja el punto de extensión preparado.

## 5. Decisión solicitada

- `APRUEBO_RFC004_OPCION_B` → se implementa el bundle en S3 en I-020/I-060 (coste 0).
- `APRUEBO_RFC004_OPCION_A_Y_AMPLIO_CREDITOS` → se hace permanente `secretsmanager` (requiere
  cubrir 134.4673 USD, hoy por encima de los 120 verificados).
- `RECHAZO_RFC004` → I-020/I-030 quedan **bloqueados para despliegue** (implementación local válida,
  sin despliegue posible con 2 instancias garantizadas).

## 6. Estado mientras no haya decisión

`BLOQUEO_DE_DESPLIEGUE` registrado en I-020 (`checklist.md`) y en `memoria.md`. La implementación
local de I-020 (Terraform, systemd, build ARM64, pruebas) **no** se ve afectada.


## 7. Análisis ampliado de la opción B (sin aprobar)

| Aspecto | Detalle |
|---|---|
| Cifrado en reposo | SSE-S3 (AES-256) lo aplica AWS; **no hay CMK propia ni KMS API desde la instancia**. Cifrado ≠ control total: sin CMK no hay revocación criptográfica inmediata ni separación de deberes sobre la clave |
| Cifrado en tránsito | TLS extremo a extremo mediante gateway endpoint (el tráfico no sale a Internet) |
| IAM | Rol de instancia con `s3:GetObject` **sólo** sobre `arn:aws:s3:::<bucket>/nexum/secrets/*`; bucket con Block Public Access, versionado y política que deniega `s3:*` fuera del rol |
| Rotación | Se publica una versión nueva (versionado del bucket) y se ejecuta `instance_refresh`; el bootstrap lee `secrets.json` actual |
| Auditoría | Server access logging + CloudTrail de datos (`s3:GetObject`) sobre el prefijo; alerta si un principal inesperado lee |
| Recuperación | Cualquier instancia nueva lee el bundle al arrancar; no depende de EBS ni de ventanas |
| Versionado | Versionado de bucket + campo `version` en el JSON; el bootstrap puede fijar versión por `?versionId=` si se requiere reproducibilidad |
| Si el secreto se compromete | Rotar en Secrets Manager (fuente de verdad), publicar nueva versión y refrescar; el objeto antiguo sigue existiendo hasta expirar (política de ciclo de vida) → **riesgo residual** |
| Costes reales | Almacenamiento (≤1 MB, céntimos/año), solicitudes GET (2 por arranque, 0.0004 USD/mil), **Versionado y logging pueden generar coste**: se acota con lifecycle de 30 días y un único prefijo |
| Coste total | **No es cero absoluto**; es ~0.00 USD en 45 días para el patrón previsto, pero no se declara "gratis": requiere medición en el Gate de despliegue |
| Dependencias I-060 | Rol, política del bucket, KMS opcional, CloudTrail de datos, lifecycle |

**Riesgo que impide declararlo equivalente a Secrets Manager:** sin CMK no hay revocación criptográfica
ni rotación automática; la rotación es operativa (humano + refresh). Debe aceptarse explícitamente.

**Estado:** **APROBADA (2026-11-09)** con `APRUEBO_RFC004_OPCION_B`; implementada en I-020/I-060.

## 8. Bloqueo de despliegue vigente

Hasta decisión humana sobre este RFC, el ASG queda con `min = desired = max = 2` y bootstrap que
**falla explícitamente** si falta `/etc/nexum/backend.env`. Consecuencia asumida: un reemplazo de
instancia fuera de ventana deja la capacidad en 1 hasta abrir ventana. Es un bloqueo de **despliegue**,
no de la implementación local.
