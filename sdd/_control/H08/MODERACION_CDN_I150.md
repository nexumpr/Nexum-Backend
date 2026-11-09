# Moderación de contenido y CDN — I-150 (INF-150-07)

| Campo | Valor |
|---|---|
| Spec | `I-150-publicacion-y-compatibilidad-api` |
| Versión | 0.2.0 |
| Estado | **DISEÑO_LOCAL_COMPLETO — IMPLEMENTACIÓN_CLOUD_PENDIENTE** |
| Moderación operativa | **NO** (`moderacion_operativa = false`) |
| Servicio cloud contratado | Ninguno · `AWS_NOT_RUN` |
| Bloqueo | `BLOCKED_DEPENDENCY`: moderación externa sin servicio aprobado ni precio verificado |

## 1. Operaciones afectadas (escaneo de los 75 contratos)

| Operación | Método y ruta | Campo | Naturaleza |
|---|---|---|---|
| **E028** | `POST /api/v1/movements` | `description` | Texto libre de usuario (concepto del movimiento) |
| **E030** | `PATCH /api/v1/movements/{movementId}` | `description` | Texto libre de usuario en actualización |
| **E075** | `POST /api/v1/voice/parse` | `text` | Texto libre: transcripción de instrucción de voz |

No existe ninguna operación que acepte **imágenes, vídeo o archivos** (`format` `binary`/`base64`/`byte`):
**0 operaciones con medios**.

## 2. Tres niveles que no se confunden

| Nivel | Qué es | Dónde se aplica | Estado |
|---|---|---|---|
| **Validación sintáctica** | tipo, longitud, caracteres de control, UTF-8 válido | en el handler, antes de cualquier efecto | Diseñado, no implementado |
| **Sanitización** | normalización, descarte de HTML/control, límite de bytes | en el handler, antes de persistir | Diseñado, no implementado |
| **Moderación de contenido** | clasificación de contenido inapropiado o abusivo mediante servicio | **antes** de persistir o procesar | **No disponible**: depende de servicio y RFC |

La validación sintáctica y la sanitización **no** sustituyen la moderación.

## 3. Punto de control

`entrada → validación sintáctica → sanitización → MODERACIÓN → persistencia/procesamiento → respuesta`

Ningún texto se persiste ni se procesa antes de la moderación cuando ésta aplique. Si el servicio de
moderación no responde, la operación **falla cerrada** con error contractual (`503` o `422` según el
caso) y **no** se guarda contenido sin evaluar.

## 4. Criterios de aceptación, rechazo y error

| Resultado | Código | Cuerpo |
|---|---|---|
| Aceptado | `201`/`200` | recurso creado con `traceId` |
| Rechazado por validación | `400`/`422` | `ErrorEnvelope` con `code: INVALID_REQUEST` |
| Rechazado por moderación | `422` | `ErrorEnvelope` con `code: CONTENT_REJECTED` |
| Moderación no disponible | `503` | `ErrorEnvelope` con `code: MODERATION_UNAVAILABLE` |
| Límite de tasa | `429` | `Retry-After` |

Ningún rechazo revela la regla exacta ni el servicio usado.

## 5. Datos financieros y personales

- `description` puede contener datos personales o financieros: **nunca** se registra en claro en logs,
  trazas ni métricas; se redacta (A-090).
- `E075.text` no se persiste en crudo; se transforma en intención y se descarta.
- Los ejemplos del contrato y de las pruebas no contienen datos reales.
- El servicio de moderación, si se adopta, no debe entrenar modelos con datos del usuario: se exige
  configuración de no retención y revisión de privacidad.

## 6. Evaluación de servicio cloud (sin contratar)

| Opción | Servicio | Disponibilidad y región | Privacidad | Idioma | Costo | Decisión |
|---|---|---|---|---|---|---|
| A | Reglas locales deterministas + redacción PII (sin servicio) | ya disponible | no sale del proceso | español soportado por reglas propias | 0 adicional | **Recomendada como primer control** |
| B | Amazon Comprehend (PII / toxicidad) | requiere verificación en `us-east-2` y plan Free | datos salen a un servicio gestionado | soporte de español a verificar | por unidad; **precio no verificado** | **Pendiente de RFC y precio** |
| C | Servicio externo de moderación (no AWS) | no aprobado por el SDD | salida de datos a terceros | — | no presupuestado | **Rechazada** |

`AWS_COST_REAL = NOT_VERIFIED` y los créditos vigentes **no** garantizan gratuidad: la opción B exige
RFC con estimación de unidades, coste mensual y análisis de privacidad.

## 7. CDN

CloudFront ya existe en la arquitectura **D2** (HTTPS público → VPC Origin → ALB interno). **No** se crea
una distribución adicional: el contenido de estas operaciones es JSON y no requiere CDN propia. Si en el
futuro se añaden medios, la distribución existente puede servir el contenido con cacheo acotado y
control de acceso.

## 8. Estado y bloqueo

| Elemento | Estado |
|---|---|
| Diseño de validación, sanitización y punto de control | `DISEÑO_LOCAL_COMPLETO` |
| Implementación en handlers | `PENDIENTE` (H08-E `NOT_AUTHORIZED`) |
| Moderación con servicio cloud | **`BLOCKED_DEPENDENCY`** — requiere RFC con servicio, precio y privacidad |
| CDN adicional | `NO_REQUERIDA` (CloudFront D2 existente) |
| `moderacion_operativa` | **`false`** |

Nada de este documento acredita moderación en funcionamiento.
