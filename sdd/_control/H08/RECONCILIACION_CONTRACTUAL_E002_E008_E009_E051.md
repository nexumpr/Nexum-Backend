# Reconciliación contractual — E002, E008, E009 y E051

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/RECONCILIACION_CONTRACTUAL_E002_E008_E009_E051.md` |
| Versión | 1.0.0 |
| Fecha | 2026-11-24 |
| Autorizaciones | `APRUEBO_RECONCILIACION_CONTRACTUAL_E002_E008_E009_E051` y `APRUEBO_CIERRE_DEFECTOS_RESIDUALES_OLEADAS_4_5` |
| Rama / base | `fix/h08e-deuda-historica` desde `main` `f3bccca` (incluye PR #146) |
| Alcance | Documental y decisiones. **No** aplica cambios de OpenAPI, runtime, AWS ni datos |
| Autoridad de transporte | `sdd/03-endpoints/openapi/nexum-api-v1.yaml` + espejo `.json` (H06, enmendados por RFC-012/013/014/016/019/020/021/022/025) |

## 1. Método

1. Se leyó la respuesta declarada de cada operación en el OpenAPI vigente (YAML y JSON) y se
   contrastó con el código Go de esta rama.
2. Se localizó la **decisión vigente** aplicable a cada diferencia (RFC aprobada, decisión `D-*`
   del Gate de oleada, spec del endpoint o política transversal H06). Una diferencia sin decisión
   localizable se marca `SIN_DECISION_VIGENTE` y no se resuelve por conveniencia.
3. Cada fila produce **una** resolución propuesta, con el instrumento necesario
   (enmienda aditiva de contrato, corrección de runtime, o conservación justificada).
4. Las resoluciones que cambian contrato quedan como **parche propuesto** en
   `dist/deuda/PARCHE_OPENAPI.md` (artefacto local, `dist/` está en `.gitignore`), y el contenido
   íntegro se reproduce en este documento cuando es corto.
5. Ninguna diferencia se cierra "borrando seguridad": cuando la cuota es obligación vigente se
   propone implementarla; retirar `429` del contrato queda expresamente rechazado.

### 1.1 Códigos declarados frente a emitidos (medición)

| Medición | Valor |
|---|---|
| Operaciones del contrato | 75 |
| Operaciones con `409` declarado | 39 |
| Operaciones con `422` declarado | 39 |
| Operaciones con `429` declarado | 73 |
| Operaciones con `403` declarado | 62 |
| Operaciones con `404` declarado | 62 |

La plantilla H06 declaró `403/404/409/422` de forma genérica en la mayoría de operaciones. Por
eso una diferencia aislada en un endpoint no se corrige "solo ahí" sin decidir qué se hace con
la plantilla: retirar un código en una operación concreta sería incoherente con las otras 38 y,
además, el comparador de I-150 lo clasifica como ruptura (`RESPUESTA_ELIMINADA`).

## 2. E002 — `POST /api/v1/auth/login`

### 2.1 Diferencia D-E002-01: `409` y `422` declarados y no emitidos

| Dimensión | Evidencia |
|---|---|
| Contrato vigente | `nexum_e002` declara `200, 400, 401, 409, 422, 429, 500, 503` (OpenAPI YAML, operación `POST /api/v1/auth/login`) |
| Semántica declarada | `409` = `CONFLICT`; `422` = `BUSINESS_RULE_VIOLATION`; ambos con `ErrorEnvelope` y ejemplo genérico de plantilla |
| Runtime real | `internal/transport/http/login_handler.go`: 400 (:61, :71, :76, :86, :117, :176), 401 (:186), 429 (:132), 500 (:166, :196), 503 (:56, :110, :127, :191). **No existe ninguna rama 409 ni 422.** El caso de uso (`internal/application/identity/login.go`) no define error de conflicto ni de regla de negocio: credenciales inválidas y cuenta no `ACTIVE` colapsan en 401 genérico por diseño anti-enumeración |
| Spec del endpoint | `sdd/03-endpoints/E002-auth-login/spec.md` §4 declara exactamente `400, 401, 429, 500, 503`. No menciona 409 ni 422 |
| Decisión vigente | RFC-013 (APPROVED): contrato V6.0.2 con esos cinco códigos. RFC-025 (APPROVED) añadió `401` al OpenAPI. No existe decisión que asigne semántica a `409`/`422` en login |
| Riesgo Android | **Bajo si se conservan**: un cliente generado puede incluir modelos para códigos que nunca ocurren; no hay rama de negocio que dependa de ellos. **Medio si se retiran**: el comparador de I-150 lo marca como ruptura y exige excepción aprobada; además rompe la uniformidad con las otras 38 operaciones |

**Resolución propuesta: CONSERVAR como reservados (sin cambio funcional), con anotación aditiva
en la descripción del contrato.**

Justificación:

1. No hay semántica de negocio definida para `409`/`422` en login; inventarla (por ejemplo, 422
   para correo con formato válido pero no registrado) **rompería la anti-enumeración** que exige
   AC-A-050-03 y el mensaje genérico de 401.
2. Retirarlos es una ruptura de contrato según I-150 y obligaría a una excepción de
   compatibilidad y a un RFC transversal para las 39 operaciones, sin beneficio de seguridad ni
   funcional.
3. La anotación en `description` es **aditiva** (no elimina códigos ni esquemas) y elimina la
   ambigüedad para quien lea el contrato: deja escrito que el runtime no los emite y que están
   reservados para una versión futura que los defina con semántica propia.

Parche exacto: `dist/deuda/PARCHE_OPENAPI.md` §1 (bloques JSON y YAML).

### 2.2 Comprobaciones adicionales de E002

| Código | Declarado | Emitido | Decisión |
|---|---|---|---|
| 400 | Sí | Sí (`login_handler.go:61` y siguientes) | Conforme |
| 401 | Sí (RFC-025) | Sí (`:186`) | Conforme |
| 429 | Sí | Sí (`:132`, código `RATE_LIMITED`, con `Retry-After` en `:131`) | Conforme en código; ver D-429-03 por la ausencia de la cabecera en el contrato |
| 500 / 503 | Sí | Sí | Conforme |
| 409 / 422 | Sí | No | **Conservar como reservados** (D-E002-01) |

## 3. E008 — `GET /api/v1/auth/sessions`

### 3.1 Diferencia D-E008-01: `429` declarado y no emitido

| Dimensión | Evidencia |
|---|---|
| Contrato vigente | `nexum_e008` declara `200, 400, 401, 403, 404, 429, 500, 503`; el `429` se describe `RATE_LIMITED` con `ErrorEnvelope` |
| Runtime real | `internal/transport/http/sessions_handler.go`: 200 (:117), 400 (:78, :86, :168), 401 (:72, :170), 500 (:180), 503 (:59, :175). El comentario del propio archivo (:24-26) declara: «Este recorte no incorpora limitador distribuido (429 no se emite)» |
| Spec del endpoint | `sdd/03-endpoints/E008-auth-sessions/spec.md` §3 lista `429` entre los códigos especificados y exige «Fijar límites de velocidad según grupo de riesgo en `POLITICAS_TRANSVERSALES_H06.md`»; §5 T01 exige que la solicitud válida produzca «el código y esquema exactos» |
| Decisión vigente | **D-429** en `sdd/_control/H08/GATE_H08E_OLEADA_1.md:373-381`: «Middleware compartido sobre `cache.AuthRateLimiter` (script Lua atómico, clave hasheada) … rutas autenticadas: 120 req/min por usuario … Respuesta 429 con `Retry-After` y `ErrorEnvelope`. Si Redis no está configurado, se omite el limitador y se documenta `429_NOT_EMITTED` (no se simula)» |
| Condición de omisión | **No se cumple**: en producción y en la composición real (`cmd/server/main.go:58`) Redis está configurado y se usa para cuotas de login/refresh/recuperación. La omisión autorizada es solo para ausencia de Redis |
| Riesgo Android | **Bajo**: Android debe tratar 429 igual que en E002/E003 (reintentar tras `Retry-After`). No hay cambio de esquema de éxito. El riesgo real es de **abuso**: sin cuota, un token robado puede enumerar sesiones a 120 req/min sin freno |

**Resolución propuesta: CORREGIR EL RUNTIME (implementar el `429`). No se retira del contrato.**

Diseño exacto (no implementado en esta fase: requiere Gate propio por cambiar el comportamiento
de un endpoint publicado y toca `cmd/server/main.go`, reservado al orquestador):

1. **Contrato** — sin cambio de códigos. Parche aditivo de cabecera `Retry-After` en `429`
   (`dist/deuda/PARCHE_OPENAPI.md` §2).
2. **Transporte** — `internal/transport/http/sessions_handler.go`:
   - `NewSessionsHandler(service, tokens, sessions, limitador LimitadorRuta, logger)`.
   - Dentro del `if service != nil && tokens != nil && sessions != nil`:
     `h.protected = LimitarPorUsuario(limitador, "sessions", middleware.AuthenticateBearerWithSession(tokens, sessions)(http.HandlerFunc(h.serve)).ServeHTTP)`.
   - `LimitarPorUsuario` ya existe (`internal/transport/http/identity_support.go:112`) y emite
     `429` + `Retry-After` con el sobre canónico; aplica **después** de la autenticación, de modo
     que la cuota es por `sub` verificado y no por un identificador del cliente.
3. **Composición** — `cmd/server/e008.go` construye
   `cacheinfra.NewAuthRateLimiter(clienteRedis, 120, time.Minute)` (misma fábrica que E051 en
   `cmd/server/e051.go:45`) y la pasa al constructor. `montarE008` debe recibir `clienteRedis`;
   la llamada está en `cmd/server/main.go:167`, **archivo reservado al orquestador** (Gate
   §1.1 de la Oleada 1).
4. **Cuota** — 120 req/min por usuario (D-429). Clase de cuota `sessions`, distinta de
   `session-revoke`, `dashboard`, `budgets`, etc., porque `AuthRateLimiter` incluye la clase en
   la clave (una clase compartida sumaría consumos de endpoints distintos y endurecería la cuota
   real sin decisión).
5. **Fail-open** — Redis caído ⇒ se sirve la lectura (D-429 y A-080 §2.5). La cuota es control de
   abuso, no dependencia de datos; el `503` de E008 sigue reservado para la persistencia.
6. **Pruebas exigibles** — (a) petición 121 en la ventana ⇒ `429` con `Retry-After` y
   `ErrorEnvelope`; (b) Redis caído ⇒ `200` y no `429`; (c) la cuota se cuenta por `sub` y dos
   usuarios distintos no se afectan; (d) sin `Bearer` ⇒ `401` antes que cualquier cuota.
7. **Código de error** — ver D-429-03: el `429` debe llevar `RATE_LIMITED`.

### 3.2 D-E008-02: `403` y `404` declarados y no emitidos

| Dimensión | Evidencia |
|---|---|
| Contrato | `403` y `404` declarados en `nexum_e008` |
| Runtime | No hay rama 403 ni 404: el listado es siempre del titular autenticado y no acepta un recurso ajeno en la ruta |
| Decisión | Plantilla H06 (62/75 operaciones declaran `403` y `404`). La spec §5 T03 condiciona el 404 con «si aplica»; en un listado propio no aplica |
| Riesgo Android | Nulo: un cliente que trate 403/404 como error genérico no se ve afectado |

**Resolución propuesta: CONSERVAR como reservados.** Retirarlos sería ruptura de contrato sin
beneficio y la spec ya los condiciona con «si aplica». Se anotan con el mismo mecanismo aditivo
del §1 del parche si se decide uniformar la plantilla completa (fuera del alcance de esta fase).

## 4. E009 — `DELETE /api/v1/auth/sessions/{sessionId}`

### 4.1 Diferencia D-E009-01: `429` declarado y no emitido

| Dimensión | Evidencia |
|---|---|
| Contrato | `nexum_e009` declara `204, 400, 401, 403, 404, 409, 422, 429, 500, 503` |
| Runtime | `internal/transport/http/session_revoke_handler.go`: 204 (:91), 400 (:78, :121), 401 (:70, :125), 404 (:128), 409 (:133), 500 (:143), 503 (:58, :138). El comentario :23-25 declara «(429 no se emite)» |
| Spec | `sdd/03-endpoints/E009-auth-sessions-sessionId/spec.md` §3 lista `429`; misma exigencia de política transversal |
| Decisión vigente | D-429 (`GATE_H08E_OLEADA_1.md:373-381`), idéntica a E008 |
| Riesgo Android | Bajo en compatibilidad; **alto en abuso**: revocar sesiones es una operación destructiva sobre estado de seguridad y es la que más conviene limitar |

**Resolución propuesta: CORREGIR EL RUNTIME (implementar el `429`).**

1. `NewSessionRevokeHandler(service, tokens, sessions, limitador LimitadorRuta, logger)`.
2. `h.protected = LimitarPorUsuario(limitador, "session-revoke", <cadena autenticada>.ServeHTTP)`.
3. Composición: `cmd/server/e009.go` recibe el limitador ya construido por `main.go` (E009 ya
   recibe `tokens` y `sessions` desde `main.go:165`; solo se añade un parámetro).
4. Cuota: 120 req/min por usuario, clase `session-revoke`.
5. Pruebas: 121ª petición ⇒ `429` + `Retry-After`; Redis caído ⇒ la revocación **no** se degrada
   (se ejecuta); sin `Bearer` ⇒ `401` primero; el `429` no revoca nada (no debe haber efecto
   parcial: la comprobación de cuota va antes del caso de uso).
6. Código de error `RATE_LIMITED` (D-429-03).

### 4.2 D-E009-02: `422` y `403` declarados y no emitidos

| Código | Runtime | Decisión propuesta |
|---|---|---|
| `422` | El `sessionId` con formato inválido produce `400` (:121). No hay regla de negocio que produzca 422 | **Conservar como reservado**; la validación de formato es 400 por decisión de la implementación y coincide con la spec (que no asigna 422 a un caso concreto) |
| `403` | No se emite; la sesión ajena produce `404` idéntico al inexistente (:127-128) para no enumerar | **Conservar como reservado.** Emitir `403` en sesión ajena sería una regresión de seguridad (revelaría existencia) |

## 5. E051 — `GET /api/v1/dashboard`

### 5.1 Diferencia D-E051-01: caché Redis exigida en la documentación y no usada

| Dimensión | Evidencia |
|---|---|
| Contrato | `nexum_e051` declara `200, 400, 401, 403, 404, 429, 500, 503`; **la caché no es una propiedad del contrato HTTP** (no hay cabecera ni campo que la exponga). La respuesta exige `Cache-Control: private, no-store` (`dashboard_handler.go:38`) |
| Runtime | `internal/application/dashboard/dashboard.go:1-2`: «No usa caché Redis: el resumen se calcula con E026 y las metas/movimientos se leen por Query»; `cmd/server/e051.go:26`: «conecta el dashboard sin caché Redis» |
| Spec | `sdd/03-endpoints/E051-dashboard/spec.md:13` («DynamoDB/Redis»), `:18` y `:90` («cache privada Redis invalidada ante mutación financiera»), `:88`, `:97`, `:98`, `:105` |
| Plan | `plan.md:9`, `:10`, `:16`, `:18`, `:25` exigen Redis y TTL/invalidación |
| Tasks | `tasks.md:8` (T04), `:9` (T05), `:10` (T06) exigen caché Redis; `tasks.md:30` ya la declara ausente en el cierre local |
| Decisión vigente | **RFC-022 v1.2.1 D-W3-07** (APPROVED, `APRUEBO_RFC_022_CONTRATOS_OLEADA_3_V1_2_1`), §8.2: «Sin caché de dashboard en esta oleada. Calcula desde DynamoDB en cada petición … No se usa Redis para E051, no se usa `SCAN`, no se amplía el ACL … La caché queda registrada como optimización posterior; requerirá un mecanismo que garantice consistencia con las mutaciones … y su propio Gate» |
| Riesgo Android | Nulo: no cambia ruta, esquema, códigos ni cabeceras. Solo latencia (R-22-07: se medirá) |

**Resolución propuesta: la norma vigente es RFC-022 D-W3-07; se alinea la documentación del
endpoint y NO se reintroduce caché.**

- `spec.md` y `plan.md` **no se editan en esta fase**: la política local los clasifica `ask` y
  AGENTS.md prohíbe cambiar especificaciones sin RFC específica. Se entrega
  `sdd/_control/H08/RFC-026-ENMIENDA_E051_CACHE.md` con el texto exacto a sustituir (secciones,
  líneas y reemplazo literal).
- `tasks.md` y `checklist.md` **sí se alinean** en esta fase (son instrumentos de seguimiento,
  no especificación): T04/T05/T06 y CT04/CT05 quedan marcados como no exigibles por RFC-022
  D-W3-07, con referencia a la norma y a la medición pendiente.

### 5.2 Diferencia D-E051-02: `429` emitido con código distinto al del contrato

| Dimensión | Evidencia |
|---|---|
| Contrato | El `429` de `nexum_e051` (y el de las 73 operaciones que lo declaran) ejemplifica `code: "RATE_LIMITED"` |
| Runtime | `dashboard_handler.go:31` aplica `LimitarPorUsuario(limitador, "dashboard", …)`; el helper emite `writeRegisterError(..., http.StatusTooManyRequests, "TOO_MANY_REQUESTS", …)` en `internal/transport/http/identity_support.go:142` |
| Contraste | Los handlers de autenticación emiten `RATE_LIMITED`: `login_handler.go:132`, `refresh_handler.go:114`, `register_handler.go:123`, `password_change_handler.go:122`, `password_reset_handler.go:102`, `password_forgot_handler.go:90`, `email_verification_request_handler.go:89`, `email_verification_confirm_handler.go:113` |
| Decisión vigente | E002 spec §4 fija `RATE_LIMITED`; el ejemplo del contrato en las 73 operaciones usa `RATE_LIMITED`; `ApiErrorObject.code` no tiene enum, pero el ejemplo y las specs son la referencia publicada |
| Riesgo Android | **Medio**: un cliente que ramifique por `code` para aplicar la espera verá dos códigos distintos para el mismo estado 429. El estado HTTP sí es correcto, por lo que un cliente que ramifique solo por status no se rompe |

**Resolución propuesta: CORREGIR EL RUNTIME en el helper compartido (una línea), no el contrato.**

`identity_support.go:142` pasa de `"TOO_MANY_REQUESTS"` a `"RATE_LIMITED"` (y el mensaje puede
mantenerse). Efecto: alinea de una vez los ~25 endpoints que usan `LimitarPorUsuario`/
`LimitarPorIP` (E022, E023, E024, E025, E027-E031, E032-E036, E038-E051, E070, E071) y deja una
sola semántica para 429 en toda la API. Requiere Gate propio (cambio de comportamiento de
endpoints publicados) y actualizar `identity_support_test.go` para fijar el código, no solo el
estado.

No se propone lo contrario (cambiar el contrato a `TOO_MANY_REQUESTS`) porque obligaría a
enmendar 73 operaciones y a romper la coherencia con las 8 rutas de autenticación ya publicadas.

### 5.3 D-E051-03: `403` y `404` declarados y no emitidos

Misma situación y misma decisión que E008 (§3.2): **conservar como reservados**. Un dashboard es
siempre del titular; no existe recurso ajeno en la ruta.

## 6. Defectos transversales detectados al reconciliar

| ID | Defecto | Evidencia | Resolución propuesta |
|---|---|---|---|
| D-429-01 | D-429 exige middleware de cuota por usuario en rutas autenticadas; **no está aplicado** a E008 ni E009 | `sessions_handler.go:24-26`, `session_revoke_handler.go:23-25` | Implementar (diseño §3.1 y §4.1). Gate propio |
| D-429-02 | La omisión autorizada de D-429 solo procede «si Redis no está configurado»; en la composición real **sí lo está** | `cmd/server/main.go:58`, `:82-83`, `:110-111`, `:125-126` | La omisión actual no está amparada: es un defecto, no una excepción |
| D-429-03 | Dos códigos para el mismo 429 (`RATE_LIMITED` vs `TOO_MANY_REQUESTS`) | §5.2 | Corregir el helper (`identity_support.go:142`) |
| D-429-04 | El contrato **no declara** la cabecera `Retry-After` en el `429`, aunque D-429 la exige y el runtime la emite | `nexum-api-v1.yaml` (0 apariciones de `Retry-After`); `identity_support.go:141`; `login_handler.go:131` | Enmienda aditiva de contrato (parche §2). No cambia códigos ni esquemas |
| D-429-05 | `429` declarado en **73** operaciones; el runtime lo emite en 8 handlers de autenticación y en los ~25 que usan el helper. El resto lo declara por plantilla | §1.1 y §5.2 | Se conserva el contrato; cada endpoint que estrene cuota debe pasar por su Gate. No se retira el código de las operaciones restantes (sería ruptura masiva) |

## 7. Resumen de resoluciones

| Diferencia | Resolución | Instrumento | Estado |
|---|---|---|---|
| D-E002-01 (`409`/`422`) | Conservar como reservados | Anotación aditiva en `description` (parche §1) | `PROPUESTA` |
| D-E008-01 (`429`) | Corregir runtime | Diseño §3.1 + Gate de runtime | `DEFECTO_RUNTIME_ABIERTO` |
| D-E008-02 (`403`/`404`) | Conservar como reservados | Sin cambio | `CONSERVADO` |
| D-E009-01 (`429`) | Corregir runtime | Diseño §4.1 + Gate de runtime | `DEFECTO_RUNTIME_ABIERTO` |
| D-E009-02 (`422`/`403`) | Conservar como reservados | Sin cambio | `CONSERVADO` |
| D-E051-01 (caché) | Norma = RFC-022 D-W3-07; alinear documentación | RFC-026 + `tasks.md`/`checklist.md` | `ENMIENDA_PROPUESTA` |
| D-E051-02 (código 429) | Corregir runtime (helper) | Diseño §5.2 + Gate de runtime | `DEFECTO_RUNTIME_ABIERTO` |
| D-E051-03 (`403`/`404`) | Conservar como reservados | Sin cambio | `CONSERVADO` |
| D-429-04 (`Retry-After`) | Enmienda aditiva de contrato | Parche §2 | `PROPUESTA` |

### 7.1 Lo que NO se hace en esta fase

- No se modifica `sdd/03-endpoints/openapi/*`: los cambios quedan en `dist/deuda/PARCHE_OPENAPI.md`.
- No se modifica código de runtime de E002/E008/E009/E051: los diseños están especificados y
  requieren Gate propio (endpoints publicados).
- No se retira ningún código de error ni ninguna cuota.
- No se toca `cmd/server/main.go`, Terraform, IAM, AWS ni datos.

## 8. Frases de aprobación propuestas (no concedidas)

1. `APRUEBO_RECONCILIACION_E002_RESERVADOS_409_422_Y_PARQUE_OPENAPI`
   — autoriza aplicar el parche §1 (anotación aditiva) y conservar `409`/`422` como reservados.
2. `APRUEBO_CUOTA_D429_EN_E008_E009_Y_CODIGO_RATE_LIMITED`
   — autoriza implementar el limitador por usuario en E008/E009, corregir el helper a
   `RATE_LIMITED` y añadir `Retry-After` al contrato (parche §2).
3. `APRUEBO_ENMIENDA_E051_SIN_CACHE_RFC_022`
   — autoriza aplicar RFC-026 en `spec.md`/`plan.md` de E051.

## 9. Evidencia de verificación de esta unidad

| Comprobación | Comando | Resultado |
|---|---|---|
| Contrato H06 | `python sdd/03-endpoints/herramientas/validar_h06.py` | Ver §9.1 |
| Paridad YAML/JSON | `python herramientas/verificar_publicacion_i150.py paridad` | 75 operaciones, 55 rutas, 102 esquemas, 0 problemas |
| Compilación | `go build ./...` | Ver informe de cierre |
| Vet | `go vet ./...` | Ver informe de cierre |
| Formato del contenido | `gofmt -l internal/application/content` | Ver informe de cierre |

_La tabla se completa con la salida real en el informe de la sesión; no se adelanta `PASS`._

## 10. Trazabilidad

- `sdd/03-endpoints/E002-auth-login/{spec,plan,tasks,checklist}.md`
- `sdd/03-endpoints/E008-auth-sessions/{spec,plan,tasks,checklist}.md`
- `sdd/03-endpoints/E009-auth-sessions-sessionId/{spec,plan,tasks,checklist}.md`
- `sdd/03-endpoints/E051-dashboard/{spec,plan,tasks,checklist}.md`
- `sdd/_control/H08/RFC-013-E002-LOGIN_V602.md`, `RFC-016-CONTRATOS_E005_E075.md`,
  `RFC-019-OLEADA_E007_E011.md`, `RFC-022-CONTRATOS_OLEADA_3.md`,
  `RFC-025-ENMIENDA_E002_401_E005_413.md`
- `sdd/_control/H08/GATE_H08E_OLEADA_1.md` §D-429
- `sdd/_control/H08/MATRIZ_AUDITORIA_54_ENDPOINTS.md`
- `sdd/_control/H08/POLITICA_PRIVACIDAD_BACKUPS.md` (límites de conservación, para E015)
