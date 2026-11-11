# RFC-012 — Alineación contractual de E001 con el prototipo Nexum V6.0.2

| Campo | Valor |
|---|---|
| Identificador | `RFC-012` |
| Título | Alineación de `POST /api/v1/auth/register` (E001) con el contrato V6.0.2 |
| Fecha de registro | 2026-11-11 |
| Estado | **AUTORIZADA POR INSTRUCCIÓN DIRECTA DEL RESPONSABLE (2026-11-11)** — ver §8 |
| Autoridad de aprobación | Responsable de Nexum-Backend |
| Alcance | Exclusivamente E001 y su ruptura de compatibilidad declarada |
| Endpoint afectado | `POST /api/v1/auth/register` |
| Contratos afectados | `sdd/03-endpoints/E001-auth-register/*`, `openapi/nexum-api-v1.{yaml,json}`, matriz H06 y matriz de accesos DynamoDB |
| Endpoints que NO cambian | E002–E075 |

> **Naturaleza de esta autorización.** El responsable autorizó la alineación y su excepción
> mediante instrucción directa en la sesión de trabajo del 2026-11-11, con el alcance
> cerrado de §4 (transcripción literal en §8). **No** se emitió una frase separada y este
> documento no la inventa. La autorización de **merge** es distinta y sigue pendiente: se
> solicita con `APRUEBO_MERGE_PR_<N>_SHA_<SHA40>` sobre el HEAD exacto del PR.

## 1. Motivación

El prototipo Android **Nexum V6.0.2** captura en una sola pantalla de alta: identidad,
ingreso habitual, todos los gastos fijos iniciales y el consentimiento de privacidad. El
contrato vigente de E001 sólo registraba identidad y consentimiento con los nombres
`displayName` / `privacyNoticeVersion` / `privacyAccepted`, por lo que el cliente no podía
persistir el ingreso ni los compromisos declarados sin una segunda operación.

## 2. Contrato anterior frente a V6.0.2

| Aspecto | Anterior (H06) | V6.0.2 propuesto |
|---|---|---|
| Nombre | `displayName` | `name` (el almacenamiento conserva `displayName`) |
| Consentimiento | `privacyAccepted`, `privacyNoticeVersion` en la raíz | bloque `privacy { accepted, noticeVersion }` |
| Ingreso habitual | no existía | `income { amount, frequency }` obligatorio |
| Gastos fijos iniciales | no existían | `recurringExpenses[]` obligatorio (admite `[]`, máximo 94) |
| Contexto de cliente | no existía | `client { locale, timeZone }` opcional |
| Respuesta 201 | `UserProfile` (`id`, `email`, `displayName`, `emailVerified`, `createdAt`, `version`) | `RegisterResponse` (`user`, `income`, `recurringPayments`, `consent`, `onboarding`) |
| Persistencia | reserva de email + `PROFILE` + `IDEM#` (3 acciones) | reserva de email + `PROFILE` + `INCOME` + `CONSENT#` + `ONBOARDING` + `RECURRING#`×N + `IDEM#` (6 + N acciones, una transacción) |
| Sesión | no crea sesión | no crea sesión (igual) |
| Política de contraseña | Argon2id, 12–128 | Argon2id, 12–128 (sin cambios) |
| Idempotencia | obligatoria | obligatoria; se añade verificación Argon2id de la contraseña en el replay |

`UserProfile` **no** se modifica: sigue siendo el contrato de E012
(`GET /api/v1/users/me`). E001 deja de referenciarlo y usa un esquema propio.

## 3. Impacto en compatibilidad (medido con la herramienta del repositorio)

`herramientas/comparar_contrato_api.py` es el control de compatibilidad de I-150. Comparando
`origin/main` (`a2a4cca`) con este cambio, devuelve **exactamente cinco rupturas, todas en
E001 y ninguna fuera de E001**:

```
{"codigo": "CAMPO_REQUERIDO_NUEVO", "detalle": "income",            "metodo": "post", "ruta": "/api/v1/auth/register"}
{"codigo": "CAMPO_REQUERIDO_NUEVO", "detalle": "name",              "metodo": "post", "ruta": "/api/v1/auth/register"}
{"codigo": "CAMPO_REQUERIDO_NUEVO", "detalle": "privacy",           "metodo": "post", "ruta": "/api/v1/auth/register"}
{"codigo": "CAMPO_REQUERIDO_NUEVO", "detalle": "recurringExpenses", "metodo": "post", "ruta": "/api/v1/auth/register"}
{"codigo": "ESQUEMA_RESPUESTA_CAMBIADO", "detalle": "201: #/components/schemas/UserProfile -> #/components/schemas/RegisterResponse", "metodo": "post", "ruta": "/api/v1/auth/register"}
```

Rupturas fuera de `/api/v1/auth/register`: **0**.

La compatibilidad expresada es de **contrato de transporte**, no de datos: los perfiles
creados con el contrato anterior siguen siendo legibles y no se migran ni se reescriben.

## 4. Excepción solicitada (alcance cerrado)

Se solicita autorización **únicamente** para estas dos clases de ruptura, en la ruta y el
método indicados:

| Código | Ruta | Método | Detalle |
|---|---|---|---|
| `CAMPO_REQUERIDO_NUEVO` | `/api/v1/auth/register` | `post` | campos `name`, `income`, `privacy`, `recurringExpenses` |
| `ESQUEMA_RESPUESTA_CAMBIADO` | `/api/v1/auth/register` | `post` | `201` pasa de `UserProfile` a `RegisterResponse` |

**No se solicita ni se autoriza aquí:** ningún cambio de ruta, método, `operationId`,
seguridad, scopes, parámetros ni respuestas de error; ninguna ruptura de E002–E075; ninguna
modificación de `UserProfile`; ninguna nueva versión de API (`/api/v1` se mantiene).

## 5. Mecanismo de activación (I-150, sin desactivar controles)

1. Registro `sdd/_control/H08/EXCEPCIONES_COMPATIBILIDAD_API.json` con `alcance =
   API_COMPATIBILIDAD`, la RFC, el código, la ruta, el método, el aprobador, este documento
   como `gate_documento` y una vigencia.
2. El job `contratos` de `go-backend.yml` pasa `--excepciones` al comparador. El comparador
   **sigue** comparando todos los contratos y sigue fallando cerrado ante cualquier ruptura
   no cubierta, cualquier RFC fuera de alcance de API y cualquier autorización vencida.
3. La excepción sólo aplica si este documento contiene la aprobación y el identificador de
   la RFC. Si la vigencia caduca, el job vuelve a fallar.

Controles que permanecen intactos: `validar_h06.py` (75 operaciones), `paridad` YAML/JSON,
`comparar_contrato_api.py` para el resto de rupturas, pruebas de `herramientas/tests` y de
`tests/infra/*`, y la exigencia de Gate humano para el merge y para cualquier mutación AWS.

## 6. Riesgos residuales

| Riesgo | Mitigación |
|---|---|
| Un cliente Android antiguo envíe el payload anterior y reciba `400` | El prototipo vigente es V6.0.2; los campos heredados se rechazan de forma explícita y el error identifica el campo. |
| Un cliente antiguo lea el `201` esperando `UserProfile` | El `201` de E001 no lo consume el prototipo actual; `GET /users/me` (E012) sigue devolviendo `UserProfile`. |
| Replay de una `Idempotency-Key` emitida antes del cambio | El `payloadHash` cambió de forma canónica: la clave antigua devuelve `409`, nunca un replay incorrecto. Ventana máxima de 24 h. |
| Divergencia entre el contrato y el código | `register_contract_test.go` valida YAML y JSON; `go test ./...` cubre el DTO. |
| Coste | 0.0000 USD: no se crean recursos, tablas ni índices. |

## 7. Trazabilidad

- Código: `internal/domain/identity/{initial_finance,client_context,registration,id}.go`,
  `internal/application/identity/register.go`,
  `internal/transport/http/register_handler.go`,
  `internal/adapters/dynamo/registration_store.go`.
- Pruebas: `tests/unit/domain/{registration,initial_finance}_test.go`,
  `tests/unit/application/register_service_test.go`,
  `tests/unit/dynamo/registration_store_test.go`,
  `tests/unit/transport/{register_handler,register_contract}_test.go`,
  `tests/integration/register_http_test.go`,
  `tests/security/register_adversarial_test.go`.
- Contrato: `openapi/nexum-api-v1.{yaml,json}`;
  matrices `MATRIZ_OPERACIONES_H06.csv`, `MATRIZ_COMPATIBILIDAD_75.md`,
  `DYNAMODB_ACCESOS_H06.md`.
- Persistencia: `NEXUM_CORE_<ENTORNO>`; sin tablas ni índices nuevos; claves ya cubiertas
  por la política IAM vigente (`USER#*`, `EMAIL#*`, `IDEM#*`).

## 8. Registro de aprobación

| Campo | Valor |
|---|---|
| Aprobador | Responsable de Nexum-Backend |
| Fecha | 2026-11-11 |
| Instrumento | Instrucción directa en la sesión de trabajo de OpenCode |
| Alcance concedido | exactamente las dos clases de ruptura de §4 |
| Frase separada | **no emitida**; esta RFC no la inventa ni la simula |
| Token de merge | independiente y **pendiente**: `APRUEBO_MERGE_PR_<N>_SHA_<SHA40>` |

Transcripción literal del alcance autorizado por el responsable (instrucción de la sesión):

> «Configura EXCEPCIONES_COMPATIBILIDAD_API.json con el formato exacto que acepta el
> validador del repositorio. Limita estrictamente las excepciones a: CAMPO_REQUERIDO_NUEVO
> de POST /api/v1/auth/register. ESQUEMA_RESPUESTA_CAMBIADO del HTTP 201. [...] Activa el
> parámetro --excepciones del comparador exclusivamente mediante el mecanismo previsto por
> I-150. No desactives controles ni conviertas errores en PASS artificialmente.»

Lo que **no** autoriza esta instrucción: cambios en otros endpoints, cambios de seguridad,
desactivar el comparador, mutaciones AWS, ni el merge del PR. Esas materias conservan sus
propios Gates.

## 9. Revocación y caducidad

- La excepción caduca el **2026-12-31**; a partir de esa fecha el comparador vuelve a
  rechazar la ruptura y obliga a una nueva decisión (nueva versión de API o RFC renovada).
- Se revoca de inmediato si aparece cualquier ruptura no cubierta, si el contrato publicado
  deja de coincidir con el código o si el responsable lo indica.
