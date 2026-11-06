# RFC-004 — Mecanismo de secretos para reemplazo automático de instancias

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/RFC-004-SECRETOS_BOOTSTRAP.md` |
| Fecha | 2026-11-06 |
| Alcance | I-000 (diseño E), I-020 (cómputo), I-060 (IAM/secretos) |
| Estado | **PROPUESTO — requiere aprobación humana; no aplicado** |
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
