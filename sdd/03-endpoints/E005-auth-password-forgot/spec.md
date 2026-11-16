# E005 — Solicitar recuperación de contraseña

| Propiedad | Contrato H06 |
|---|---|
| Método y ruta | `POST /api/v1/auth/password/forgot` |
| Origen H04 | Clase `E` (M maqueta; S soporte; E esperado; O opcional) |
| Alcance | **REQUIRED_DESIGN**; documentado, no implementado |
| Seguridad | `public` (público/credencial específica del cuerpo) |
| Respuesta éxito | **HTTP 202**; `Accepted` |
| Request | `EmailRequest` |
| Idempotency-Key | No exigido por este contrato |
| Control de versión | CAS/versión interna cuando corresponda |
| Persistencia | AWS DynamoDB/Redis, sin Scan; ver `DYNAMODB_ACCESOS_H06.md` |
| Releases | H08+, una función/endpoint nuevo por release con aprobación |

## 1. Caso de uso y criterio funcional

Respuesta 202 indistinguible para correos existentes/no existentes; token de un uso y expiración; envío mediante adaptador verificado.

**Precondiciones:** ruta pública validada o JWT de usuario activo; el servidor siempre deriva `userId` desde su identidad autenticada. Toda referencia `{...}` debe verificarse contra recursos propiedad del usuario, salvo recursos públicos. Los estados del usuario borrado o revocado impiden nuevas escrituras. **Postcondiciones:** resultado reproducible por HTTP sin duplicidad ni efectos ocultos fuera de transacción.

## 2. Entrada HTTP, parámetros y reglas

**Payload `EmailRequest`:**

| Campo | Tipo / ejemplo | Requerido |
|---|---|:---:|
| `email` | `"ejemplo@example.invalid"` | Sí |

**Parámetros:** ninguno explícito. JSON UTF-8, `application/json`, propiedades desconocidas rechazadas. El dominio valida además rangos y transiciones, aunque el tipo JSON sea válido. Moneda inicial MXN; importes serializados decimal string y operados internamente como `int64` centavos.

## 3. Salida, seguridad y errores

**Respuesta `Accepted`:**

| Campo | Tipo / ejemplo | Requerido |
|---|---|:---:|
| `status` | `"accepted"` | Sí |
| `requestId` | `"018f90ef-08e5-7a68-b09f-2b67a47c92f0"` | Sí |

**Códigos especificados:** `202`, `400`, `409`, `422`, `429`, `500`, `503`. Error común: `{"error":{"code":"...","message":"...","traceId":"...","details":[]}}`. No revelar recursos ajenos, hashes, JWT, correo inexistente, configuración AWS ni datos financieros en logs. Fijar límites de velocidad según grupo de riesgo en `POLITICAS_TRANSVERSALES_H06.md`.

**Request JSON de ejemplo (sintético, sin credenciales reales):**

```json
{
  "email": "ejemplo@example.invalid"
}
```

**Response JSON de ejemplo (sintético):**

```json
{
  "status": "accepted",
  "requestId": "018f90ef-08e5-7a68-b09f-2b67a47c92f0"
}
```

## 4. Acceso físico e invariantes

Email-reserva / USER#id PROFILE y SESSION#id / REFRESH#<digest>/SESSION; token hash con TTL y expiración funcional.

Invariante: Respuesta 202 indistinguible para correos existentes/no existentes; token de un uso y expiración; envío mediante adaptador verificado. Ninguna operación puede hacer `Scan` global ni consultar PK de un usuario diferente. ETag/versionado, cancelación de transacciones, caché y TTL deben seguir H04/H05. El contrato OpenAPI es `openapi/nexum-api-v1.yaml` y su espejo JSON; ambos son la autoridad de transporte de H06.

## 5. Pruebas de aceptación obligatorias (para H08)

- **T01:** Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- **T02:** JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- **T03:** Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- **T04:** Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- **T05:** Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.

## 6. Trazabilidad, evidencia y estado

- Inventario origen: `../01-arquitectura-general/TRAZABILIDAD_75_RUTAS.md` (mismo E-ID); matriz H06 `MATRIZ_OPERACIONES_H06.csv`.
- Dependencias: H04 A-030/A-040/A-050/A-060/A-070/A-080; H05 I-040/I-050/I-150; seguridad GOV-06/07/11/15/16.
- **Contrato diseñado:** documentado en H06. **Pruebas HTTP/Dynamo/Redis y despliegue:** NOT_RUN; solo H08+, no afirmar `PASS`.
- **Responsabilidad:** Backend independiente del equipo móvil. La API cumple contrato sin solicitar aprobación de Android.

## Enmienda aprobada — RFC-016 (A1, 2026-11-16)

Gate: `APRUEBO_RFC_016_CONTRATOS_E005_E075` y `APRUEBO_ENMIENDA_A1`.

- Contrato tras la enmienda: referencias de persistencia corregidas a `USER#<userId>/SESSION#<sessionId>` y `REFRESH#<sha256(refreshToken)>/SESSION`; sin cambio de tabla ni de codigo.
- Compatibilidad: cambio aditivo; ningun cliente existente (E001-E004) se ve afectado.
- Autoridad: el OpenAPI actualizado es la fuente de transporte.

## Enmienda A11 — proveedor de correo: Amazon SES (2026-11-16)

Gate: `APRUEBO_RFC_016_CONTRATOS_E005_E075` (§4, decisión del Scrum Master). Las enmiendas A1-A8
no citaban el proveedor; esta precisión documenta la decisión ya ratificada.

- Proveedor: **Amazon Simple Email Service (SES)**, región `us-east-2`.
- Flujo: validar `email` → transacción de admisión (token + trabajo durable) → intento de envío →
  `202 Accepted {status, requestId}`.
- El `202` significa **solicitud aceptada y persistida**; nunca «correo entregado».
- Errores: `400`, `409`, `422`, `429`, `500`, `503` con envelope común.
- No enumeración: misma respuesta, mismo cuerpo y mismo trabajo para cuenta existente e
  inexistente; límites por IP y por correo (HMAC); prueba estadística de tiempos (α = 0,01).
- Token: 32 bytes CSPRNG `base64url`, un solo uso, TTL 30 min, solo su SHA-256 en `RESET#`.
