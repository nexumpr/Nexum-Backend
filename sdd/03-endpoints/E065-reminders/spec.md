# E065 — Crear recordatorio

| Propiedad | Contrato H06 |
|---|---|
| Método y ruta | `POST /api/v1/reminders` |
| Origen H04 | Clase `E` (M maqueta; S soporte; E esperado; O opcional) |
| Alcance | **REQUIRED_DESIGN**; documentado, no implementado |
| Seguridad | `bearer` (Bearer JWT) |
| Respuesta éxito | **HTTP 201**; `Reminder` |
| Request | `ReminderCreate` |
| Idempotency-Key | OBLIGATORIO |
| Control de versión | CAS/versión interna cuando corresponda |
| Persistencia | AWS DynamoDB/Redis, sin Scan; ver `DYNAMODB_ACCESOS_H06.md` |
| Releases | H08+, una función/endpoint nuevo por release con aprobación |

## 1. Caso de uso y criterio funcional

Zona horaria IANA válida, programación real sujeta a permisos/costo; no programar envío sin opt-in.

**Precondiciones:** ruta pública validada o JWT de usuario activo; el servidor siempre deriva `userId` desde su identidad autenticada. Toda referencia `{...}` debe verificarse contra recursos propiedad del usuario, salvo recursos públicos. Los estados del usuario borrado o revocado impiden nuevas escrituras. **Postcondiciones:** resultado reproducible por HTTP sin duplicidad ni efectos ocultos fuera de transacción.

## 2. Entrada HTTP, parámetros y reglas

**Payload `ReminderCreate`:**

| Campo | Tipo / ejemplo | Requerido |
|---|---|:---:|
| `title` | `"Ejemplo de concepto"` | Sí |
| `dueAt` | `"2026-11-05T17:00:00Z"` | Sí |
| `timezone` | `"America/Mexico_City"` | Sí |

**Parámetros:** `header:Idempotency-Key` (requerido). JSON UTF-8, `application/json`, propiedades desconocidas rechazadas. El dominio valida además rangos y transiciones, aunque el tipo JSON sea válido. Moneda inicial MXN; importes serializados decimal string y operados internamente como `int64` centavos.

## 3. Salida, seguridad y errores

**Respuesta `Reminder`:**

| Campo | Tipo / ejemplo | Requerido |
|---|---|:---:|
| `id` | `"018f90ef-08e5-7a68-b09f-2b67a47c92f0"` | Sí |
| `title` | `"Ejemplo de concepto"` | Sí |
| `dueAt` | `"2026-11-05T17:00:00Z"` | Sí |
| `timezone` | `"America/Mexico_City"` | Sí |
| `active` | `true` | Sí |
| `version` | `1` | Sí |

**Códigos especificados:** `201`, `400`, `401`, `403`, `404`, `409`, `422`, `429`, `500`, `503`. Error común: `{"error":{"code":"...","message":"...","traceId":"...","details":[]}}`. No revelar recursos ajenos, hashes, JWT, correo inexistente, configuración AWS ni datos financieros en logs. Fijar límites de velocidad según grupo de riesgo en `POLITICAS_TRANSVERSALES_H06.md`.

**Request JSON de ejemplo (sintético, sin credenciales reales):**

```json
{
  "title": "Ejemplo de concepto",
  "dueAt": "2026-11-05T17:00:00Z",
  "timezone": "America/Mexico_City"
}
```

**Response JSON de ejemplo (sintético):**

```json
{
  "id": "018f90ef-08e5-7a68-b09f-2b67a47c92f0",
  "title": "Ejemplo de concepto",
  "dueAt": "2026-11-05T17:00:00Z",
  "timezone": "America/Mexico_City",
  "active": true,
  "version": 1
}
```

## 4. Acceso físico e invariantes

USER#<sub> / DEVICE#<id>, NOTIFICATION#<UTC>#<id>, REMINDER#<id>, PREFERENCES; GSI2 solo worker due.

Invariante: Zona horaria IANA válida, programación real sujeta a permisos/costo; no programar envío sin opt-in. Ninguna operación puede hacer `Scan` global ni consultar PK de un usuario diferente. ETag/versionado, cancelación de transacciones, caché y TTL deben seguir H04/H05. El contrato OpenAPI es `openapi/nexum-api-v1.yaml` y su espejo JSON; ambos son la autoridad de transporte de H06.

## 5. Pruebas de aceptación obligatorias (para H08)

- **T01:** Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- **T02:** JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- **T03:** Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- **T04:** Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- **T05:** Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- **T06:** Repetición idéntica de Idempotency-Key -> respuesta estable; payload diferente con misma key -> 409; carrera multiinstancia.
- **T07:** Opt-in/opt-out efectivo; no enviar push con token revocado; no requerir aceptación del equipo Android.

## 6. Trazabilidad, evidencia y estado

- Inventario origen: `../01-arquitectura-general/TRAZABILIDAD_75_RUTAS.md` (mismo E-ID); matriz H06 `MATRIZ_OPERACIONES_H06.csv`.
- Dependencias: H04 A-030/A-040/A-050/A-060/A-070/A-080; H05 I-040/I-050/I-150; seguridad GOV-06/07/11/15/16.
- **Contrato diseñado:** documentado en H06. **Pruebas HTTP/Dynamo/Redis y despliegue:** NOT_RUN; solo H08+, no afirmar `PASS`.
- **Responsabilidad:** Backend independiente del equipo móvil. La API cumple contrato sin solicitar aprobación de Android.
