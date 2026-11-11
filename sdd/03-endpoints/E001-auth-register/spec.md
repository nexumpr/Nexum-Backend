# E001 — Registrar usuario, ingreso habitual y compromisos fijos iniciales (V6.0.2)

| Propiedad | Contrato H06 → V6.0.2 |
|---|---|
| Método y ruta | `POST /api/v1/auth/register` (sin cambios) |
| Origen H04 | Clase `M` (M maqueta; S soporte; E esperado; O opcional) |
| Alcance | **REQUIRED_DESIGN**; contrato actualizado por RFC-012 |
| Versión de contrato | **V6.0.2** (sustituye de forma intencional al contrato anterior de E001) |
| Seguridad | `public` (endpoint público, HTTPS obligatorio, rate limiting Redis) |
| Respuesta éxito | **HTTP 201**; `RegisterResponse` (nuevo esquema dedicado) |
| Request | `RegisterRequest` (nuevo contenido, mismo nombre de esquema) |
| Idempotency-Key | OBLIGATORIO |
| Persistencia | DynamoDB `NEXUM_CORE_<ENTORNO>`; una sola `TransactWriteItems`; sin `Scan` |
| Releases | Una función/endpoint nuevo por release con aprobación humana |

## 1. Caso de uso y criterio funcional

El alta de cuenta del prototipo Nexum V6.0.2 captura, en una única pantalla,
**identidad + ingreso habitual + todos los gastos fijos iniciales + consentimiento**.
E001 debe registrar ese conjunto como **una sola operación atómica**: o se persiste todo,
o no se persiste nada.

**Precondiciones:** ruta pública; `Content-Type: application/json`; `Idempotency-Key`
válida de 16–128 caracteres imprimibles ASCII; rate limit de Redis disponible (si no lo
está, el endpoint responde `503` y no escribe nada).

**Postcondiciones:** existe reserva de correo, perfil, ingreso habitual, consentimiento
versionado, onboarding inicial, todos los compromisos fijos declarados y el registro de
idempotencia. La respuesta 201 se produce con valores generados por el servidor y es
reproducible byte a byte en un replay legítimo. **E001 no crea sesión**: no emite tokens
ni cookies (la propiedad `session` del prototipo es condicional y se omite mientras
E002/E003 no estén implementados).

## 2. Entrada HTTP, parámetros y reglas

**Cabeceras:** `Accept: application/json`, `Content-Type: application/json`,
`Idempotency-Key` (obligatoria), `X-Request-ID` (opcional, trazabilidad). El endpoint no
requiere `Authorization`.

**Payload `RegisterRequest`:**

| Campo | Tipo / ejemplo | Requerido | Regla |
|---|---|:---:|---|
| `name` | `"Leonardo Prueba"` | Sí | `signupName` del prototipo. Trim, 1–100 caracteres, sin caracteres de control. Sustituye a `displayName` en el contrato público; el almacenamiento conserva `displayName`. |
| `email` | `"registro.v602@example.com"` | Sí | RFC válido, máximo 254, normalizado a minúsculas, unicidad por reserva transaccional. |
| `password` | `"ExamplePassphrase#2026"` | Sí | 12–128 caracteres. Argon2id. Nunca se persiste en claro ni se registra. |
| `income.amount` | `"12000.00"` | Sí | Cadena decimal MXN con exactamente dos decimales, estrictamente mayor que 0. Prohibido número JSON. Internamente centavos `int64`. |
| `income.frequency` | `"monthly"` | Sí | Enum cerrado: `weekly`, `fortnightly`, `monthly`, `bimonthly`, `quarterly`, `semiannual`, `annual`. No se admite `daily` ni `once`. |
| `recurringExpenses` | `[...]` | Sí | Array completo de la pantalla; **`[]` es válido**. Máximo 94 filas. No se truncan filas. |
| `recurringExpenses[].concept` | `"Internet"` | Sí | Trim, 1–100 caracteres, sin controles. |
| `recurringExpenses[].amount` | `"699.00"` | Sí | Cadena decimal MXN, dos decimales, mayor que 0. |
| `recurringExpenses[].frequency` | `"monthly"` | Sí | Mismo enum de siete valores. |
| `recurringExpenses[].category` | `"Servicios"` | Sí | Derivada por `guessFixedCategory` en el cliente; el servidor valida el catálogo cerrado `Vivienda, Servicios, Transporte, Comida, Otros` y persiste el valor. No existen identificadores numéricos. |
| `recurringExpenses[].type` | `"fixed"` | Sí | Único valor admitido. Se rechaza cualquier otro. |
| `privacy.accepted` | `true` | Sí | Debe ser `true`; el consentimiento es afirmativo, nunca por defecto. |
| `privacy.noticeVersion` | `"2026-11"` | Sí | Versión exacta mostrada al usuario, 1–50 caracteres. El servidor **no** la sustituye por la vigente. |
| `client.locale` | `"es-MX"` | No | BCP-47 simplificado, máximo 35. |
| `client.timeZone` | `"America/Mexico_City"` | No | Zona IANA validada contra la base de zonas horarias, máximo 64. Se usa para cálculos futuros; no fija fechas de vencimiento. |

**Campos prohibidos (rechazo 400 por campo desconocido):** `signupConfirm` (Android compara
las contraseñas en local), `signupIncomeMonthly`, `signupFixedMonthly`, `userId`, fecha de
nacimiento, teléfono, género, RFC, domicilio, fotografía, cuentas bancarias, contraseña
duplicada y los campos heredados `displayName`, `privacyAccepted`, `privacyNoticeVersion`.

**Parámetros:** `header:Idempotency-Key` (requerido). JSON UTF-8, `application/json`,
propiedades desconocidas rechazadas. Moneda MXN; importes serializados como cadena decimal
y operados como `int64` centavos.

## 3. Salida, seguridad y errores

**Respuesta `RegisterResponse` (HTTP 201):**

| Campo | Tipo / ejemplo | Requerido |
|---|---|:---:|
| `user.id` | `"018f90ef-08e5-7a68-b09f-2b67a47c92f0"` | Sí |
| `user.name` | `"Leonardo Prueba"` | Sí |
| `user.email` | `"registro.v602@example.com"` | Sí |
| `user.emailVerified` | `false` | Sí |
| `user.createdAt` | `"2026-11-10T12:00:00Z"` | Sí |
| `income.amount` | `"12000.00"` | Sí |
| `income.frequency` | `"monthly"` | Sí |
| `income.monthlyEquivalent` | `"12000.00"` | Sí |
| `recurringPayments[].id` | `"018f90ef-08e5-4a68-b09f-000000000101"` | Sí |
| `recurringPayments[].concept` | `"Internet"` | Sí |
| `recurringPayments[].amount` | `"699.00"` | Sí |
| `recurringPayments[].frequency` | `"monthly"` | Sí |
| `recurringPayments[].category` | `"Servicios"` | Sí |
| `recurringPayments[].type` | `"fixed"` | Sí |
| `consent.noticeVersion` | `"2026-11"` | Sí |
| `consent.acceptedAt` | `"2026-11-10T12:00:00Z"` | Sí |
| `consent.acceptedByServer` | `true` | Sí |
| `onboarding.microphone` | `"pending"` | Sí |
| `onboarding.notifications` | `"pending"` | Sí |
| `onboarding.configured` | `false` | Sí |

`recurringPayments` devuelve **todas** las filas realmente persistidas y se serializa como
`[]` cuando no hay ninguna. La respuesta no contiene contraseña ni hash, ni tokens.
`Location: /api/v1/users/me`. `X-Idempotent-Replay: true` solo en un replay.

**Códigos especificados:** `201`, `400` (`INVALID_REQUEST`), `409` (`CONFLICT`, correo
duplicado o clave reutilizada con otro payload/contraseña), `422`
(`BUSINESS_RULE_VIOLATION`), `429` (`RATE_LIMITED` con `Retry-After`), `500`
(`INTERNAL_ERROR`), `503` (`DEPENDENCY_UNAVAILABLE`). Error común:
`{"error":{"code":"...","message":"...","traceId":"...","details":[]}}`. No revelar
recursos ajenos, hashes, JWT, correo existente, configuración AWS ni datos financieros en
logs.

**Request JSON de ejemplo (sintético):**

```json
{
  "name": "Leonardo Prueba",
  "email": "registro.v602@example.com",
  "password": "ExamplePassphrase#2026",
  "income": { "amount": "12000.00", "frequency": "monthly" },
  "recurringExpenses": [
    { "concept": "Internet", "amount": "699.00", "frequency": "monthly",
      "category": "Servicios", "type": "fixed" },
    { "concept": "Transporte", "amount": "300.00", "frequency": "weekly",
      "category": "Transporte", "type": "fixed" }
  ],
  "privacy": { "accepted": true, "noticeVersion": "2026-11" },
  "client": { "locale": "es-MX", "timeZone": "America/Mexico_City" }
}
```

**Response JSON de ejemplo (sintético):** ver §3 del OpenAPI `RegisterResponse`; los
identificadores y marcas de tiempo son generados por el servidor.

## 4. Acceso físico e invariantes

Una sola `TransactWriteItems` sobre `NEXUM_CORE_<ENTORNO>` con seis acciones fijas
(`EMAIL#<sha256>`/`OWNER`, `USER#<id>`/`PROFILE`, `USER#<id>`/`INCOME`,
`USER#<id>`/`CONSENT#<noticeVersion>`, `USER#<id>`/`ONBOARDING`,
`IDEM#<sha256>`/`RECORD`) más una por compromiso (`USER#<id>`/`RECURRING#<uuid>`), en ese
orden y con la idempotencia al final. Ninguna clave crea particiones nuevas: la política
IAM vigente (`USER#*`, `EMAIL#*`, `IDEM#*`) ya las autoriza.

Invariantes: correo normalizado único con reserva transaccional; contraseña Argon2id;
estado de correo no verificado; ingreso y compromisos en centavos exactos; consentimiento
sellado por el servidor; onboarding pendiente; máximo 94 compromisos (100 acciones − 6);
ninguna escritura parcial; ningún `Scan`. El equivalente mensual se calcula con el motor
exacto de A-070 (factores 52/12, 2, 1, 1/2, 1/3, 1/6, 1/12 y redondeo HALF_UP en centavos).

Los compromisos iniciales son **compromisos programados**, no gastos ejecutados: el alta no
crea ningún `MOVEMENT` y no se descuentan dos veces en resúmenes futuros. El ingreso
registrado no equivale a movimientos bancarios históricos.

`payloadHash` cubre identidad, ingreso, compromisos en su orden de envío, consentimiento y
contexto de cliente. La contraseña **no** forma parte del hash (el sal Argon2id no es
determinista); su igualdad se prueba en el replay contra el verificador Argon2id guardado
en el registro de idempotencia.

## 5. Pruebas de aceptación obligatorias (H08)

- **T01:** solicitud válida produce el código y el esquema exactos del OpenAPI.
- **T02:** JSON malformado, campos desconocidos (incluidos los heredados y `signupConfirm`),
  tipo incorrecto, importe con formato distinto de dos decimales y longitud excedida →
  400/422 sin secretos en logs.
- **T03:** `[]`, una fila y varias filas de `recurringExpenses` persisten exactamente lo
  declarado; 94 filas se aceptan y 95 se rechazan completas con `422`.
- **T04:** las siete frecuencias producen el equivalente mensual exacto esperado.
- **T05:** pérdida temporal de DynamoDB/Redis → `503`; nunca escritura parcial.
- **T06:** repetición idéntica de `Idempotency-Key` → respuesta estable; payload distinto o
  contraseña distinta con la misma clave → `409`; carrera multiinstancia sin duplicados.
- **T07:** fallo condicional en una sola fila de compromiso cancela la transacción completa.
- **T08:** los perfiles creados con el contrato anterior siguen siendo legibles.

## 6. Trazabilidad, evidencia y estado

- Inventario origen: `../01-arquitectura-general/TRAZABILIDAD_75_RUTAS.md` (mismo E-ID);
  matriz H06 `MATRIZ_OPERACIONES_H06.csv`.
- Alineación contractual: `sdd/_control/H08/RFC-012-ALINEACION_E001_V602.md`.
- Dependencias: H04 A-030/A-040/A-050/A-060/A-070/A-080; H05 I-040/I-050/I-150; seguridad
  GOV-06/07/11/15/16.
- **Contrato:** V6.0.2 sincronizado en OpenAPI YAML y JSON. **Implementación local:**
  verificada con `go build`, `go vet` y `go test`. **Pruebas HTTP/Dynamo/Redis reales:**
  `NOT_RUN` hasta el despliegue autorizado; no afirmar `PASS`.
- **Responsabilidad:** Backend independiente del equipo móvil. La API cumple contrato sin
  solicitar aprobación de Android.
