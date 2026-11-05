# E025 — Configurar ingreso base

| Propiedad | Contrato H06 |
|---|---|
| Método y ruta | `PUT /api/v1/income` |
| Origen H04 | Clase `M` (M maqueta; S soporte; E esperado; O opcional) |
| Alcance | **REQUIRED_DESIGN**; documentado, no implementado |
| Seguridad | `bearer` (Bearer JWT) |
| Respuesta éxito | **HTTP 200**; `Income` |
| Request | `IncomeUpdate` |
| Idempotency-Key | No exigido por este contrato |
| Control de versión | If-Match OBLIGATORIO; discrepancia 412 |
| Persistencia | AWS DynamoDB/Redis, sin Scan; ver `DYNAMODB_ACCESOS_H06.md` |
| Releases | H08+, una función/endpoint nuevo por release con aprobación |

## 1. Caso de uso y criterio funcional

PUT idempotente con versionado; almacenar cents int64, currency MXN y frecuencia explícita.

**Precondiciones:** ruta pública validada o JWT de usuario activo; el servidor siempre deriva `userId` desde su identidad autenticada. Toda referencia `{...}` debe verificarse contra recursos propiedad del usuario, salvo recursos públicos. Los estados del usuario borrado o revocado impiden nuevas escrituras. **Postcondiciones:** resultado reproducible por HTTP sin duplicidad ni efectos ocultos fuera de transacción.

## 2. Entrada HTTP, parámetros y reglas

**Payload `IncomeUpdate`:**

| Campo | Tipo / ejemplo | Requerido |
|---|---|:---:|
| `amount` | `"1000.00"` | Sí |
| `currency` | `"MXN"` | Sí |
| `frequency` | `"monthly"` | Sí |

**Parámetros:** `header:If-Match` (requerido). JSON UTF-8, `application/json`, propiedades desconocidas rechazadas. El dominio valida además rangos y transiciones, aunque el tipo JSON sea válido. Moneda inicial MXN; importes serializados decimal string y operados internamente como `int64` centavos.

## 3. Salida, seguridad y errores

**Respuesta `Income`:**

| Campo | Tipo / ejemplo | Requerido |
|---|---|:---:|
| `amount` | `"1000.00"` | Sí |
| `currency` | `"MXN"` | Sí |
| `frequency` | `"monthly"` | Sí |
| `version` | `1` | Sí |

**Códigos especificados:** `200`, `400`, `401`, `403`, `404`, `409`, `412`, `422`, `428`, `429`, `500`, `503`. Error común: `{"error":{"code":"...","message":"...","traceId":"...","details":[]}}`. No revelar recursos ajenos, hashes, JWT, correo inexistente, configuración AWS ni datos financieros en logs. Fijar límites de velocidad según grupo de riesgo en `POLITICAS_TRANSVERSALES_H06.md`.

**Request JSON de ejemplo (sintético, sin credenciales reales):**

```json
{
  "amount": "1000.00",
  "currency": "MXN",
  "frequency": "monthly"
}
```

**Response JSON de ejemplo (sintético):**

```json
{
  "amount": "1000.00",
  "currency": "MXN",
  "frequency": "monthly",
  "version": 1
}
```

## 4. Acceso físico e invariantes

USER#<sub> / INCOME, MOVEMENT#<UTC>#<id>; periodo y proyección financiera versionada.

Invariante: PUT idempotente con versionado; almacenar cents int64, currency MXN y frecuencia explícita. Ninguna operación puede hacer `Scan` global ni consultar PK de un usuario diferente. ETag/versionado, cancelación de transacciones, caché y TTL deben seguir H04/H05. El contrato OpenAPI es `openapi/nexum-api-v1.yaml` y su espejo JSON; ambos son la autoridad de transporte de H06.

## 5. Pruebas de aceptación obligatorias (para H08)

- **T01:** Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- **T02:** JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- **T03:** Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- **T04:** Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- **T05:** Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- **T06:** Montos exactos en centavos: 0, mínimo, máximo, decimal inválido, cambio de mes y zona IANA.
- **T07:** If-Match omitido -> 428; ETag viejo -> 412; carrera de versiones no pierde actualizaciones.

## 6. Trazabilidad, evidencia y estado

- Inventario origen: `../01-arquitectura-general/TRAZABILIDAD_75_RUTAS.md` (mismo E-ID); matriz H06 `MATRIZ_OPERACIONES_H06.csv`.
- Dependencias: H04 A-030/A-040/A-050/A-060/A-070/A-080; H05 I-040/I-050/I-150; seguridad GOV-06/07/11/15/16.
- **Contrato diseñado:** documentado en H06. **Pruebas HTTP/Dynamo/Redis y despliegue:** NOT_RUN; solo H08+, no afirmar `PASS`.
- **Responsabilidad:** Backend independiente del equipo móvil. La API cumple contrato sin solicitar aprobación de Android.
