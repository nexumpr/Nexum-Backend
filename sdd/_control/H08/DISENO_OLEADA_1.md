# Diseño técnico — Oleada 1 (E012–E071 seleccionados)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/DISENO_OLEADA_1.md` |
| Fecha | 2026-11-18 |
| Base | `main` `88ef920` |
| Gate | `GATE_H08E_OLEADA_1.md` |
| Estado | **PROPUESTO** — no autoriza implementación hasta la frase del Gate §15 |

Este documento fija la forma de trabajo y los contratos internos para que tres líneas escriban en
paralelo sin conflictos. No sustituye al OpenAPI: si hay discrepancia, gana el OpenAPI vigente
(salvo RFC-020 aprobada).

## 1. Andamiaje (PR 0, orquestador)

Archivos que **sólo** escribe el orquestador:

| Archivo | Contenido |
|---|---|
| `cmd/server/registro_oleada1.go` | `dependenciasOleada`, `registrarOleada1`, `montarOleada1` y el registro ordenado por ID |
| `cmd/server/main.go` | una llamada a `montarOleada1(...)` después de E007 y `_ "time/tzdata"` |
| `internal/transport/http/identity_support.go` | ETag `"vN"`, parseo de `If-Match` (428/400), validación de `locale`/IANA, envoltorio de límite de tasa |
| `internal/infrastructure/cache/public_cache.go` | `GetOrLoadJSON` con TTL y fail-open si Redis no está |
| `internal/application/content_version.go` | constantes `contentVersion`, catálogos y textos públicos |
| `cmd/server/registro_oleada1_test.go` | exactamente 15 rutas registradas y ninguna de Oleada 2 |

Contrato del registro (determinista y fail-closed en arranque):

```go
type dependenciasOleada struct {
    Router *nexumhttp.Router
    Dynamo *dynamodb.Client
    Redis  *redis.Client
    Tabla  string
    Logger *slog.Logger
}

type montajeOleada struct {
    ID     string // "E012"
    Metodo string // http.MethodGet
    Ruta   string // "/users/me" (relativa a /api/v1)
    Montar func(*dependenciasOleada)
}

var registrosOleada1 = map[string]montajeOleada{}

func registrarOleada1(m montajeOleada) {
    if m.ID == "" || m.Metodo == "" || m.Ruta == "" || m.Montar == nil {
        panic("registro de oleada invalido")
    }
    if _, existe := registrosOleada1[m.ID]; existe {
        panic("endpoint duplicado en la oleada: " + m.ID)
    }
    registrosOleada1[m.ID] = m
}

func montarOleada1(d *dependenciasOleada) {
    ids := make([]string, 0, len(registrosOleada1))
    for id := range registrosOleada1 {
        ids = append(ids, id)
    }
    sort.Strings(ids)
    for _, id := range ids {
        registrosOleada1[id].Montar(d)
    }
}
```

Cada endpoint añade en su propio `cmd/server/eNNN.go`:

```go
func init() {
    registrarOleada1(montajeOleada{ID: "E012", Metodo: http.MethodGet, Ruta: "/users/me", Montar: montarE012})
}
```

Justificación de `init()` frente a un registro explícito centralizado: un registro central
obligaría a editar un archivo compartido en cada uno de los 15 PR (conflicto y riesgo de que un PR
quede desconectado). `init()` no hace I/O ni lee entorno; sólo llena un mapa en memoria. El mapa
detecta duplicados de ID y el router recibe rutas únicas por método. `montarOleada1` ordena por ID
para que el orden de montaje sea determinista. Un test comprueba que no hay duplicados de
`(metodo, ruta)` y que el conjunto final es exactamente el de los 15 endpoints autorizados.

## 2. Paquetes por línea (sin solapamiento)

### Línea A — perfil, privacidad, onboarding

| Endpoint | Aplicación | Adaptador | Handler | Montaje |
|---|---|---|---|---|
| E012/E013 | `internal/application/profile` | `internal/adapters/dynamo/profile_store.go` | `internal/transport/http/profile_handler.go` | `cmd/server/e012.go`, `e013.go` |
| E015 | `internal/application/privacy` (aviso) | — | `internal/transport/http/privacy_notice_handler.go` | `cmd/server/e015.go` |
| E016/E017 | `internal/application/privacy` (consentimientos) | `internal/adapters/dynamo/consent_store.go` | `internal/transport/http/consents_handler.go` | `cmd/server/e016.go`, `e017.go` |
| E018/E019 | `internal/application/onboarding` | `internal/adapters/dynamo/onboarding_store.go` | `internal/transport/http/onboarding_handler.go` | `cmd/server/e018.go`, `e019.go` |

### Línea B — catálogos e ingreso

| Endpoint | Aplicación | Adaptador | Handler | Montaje |
|---|---|---|---|---|
| E022/E023 | `internal/application/catalogs` | — (estático + caché pública) | `internal/transport/http/catalogs_handler.go` | `cmd/server/e022.go`, `e023.go` |
| E024/E025 | `internal/application/income` | `internal/adapters/dynamo/income_store.go` | `internal/transport/http/income_handler.go` | `cmd/server/e024.go`, `e025.go` |

### Línea C — preferencias y contenido

| Endpoint | Aplicación | Adaptador | Handler | Montaje |
|---|---|---|---|---|
| E068/E069 | `internal/application/preferences` | `internal/adapters/dynamo/preferences_store.go` | `internal/transport/http/preferences_handler.go` | `cmd/server/e068.go`, `e069.go` |
| E070/E071 | `internal/application/content` | — (estático + caché pública) | `internal/transport/http/content_handler.go` | `cmd/server/e070.go`, `e071.go` |

## 3. DTOs exactos (transporte)

Los DTO viven en el handler (no en dominio) y se serializan con `encoding/json`. Ninguno incluye
campos extra. `additionalProperties` se rechaza en la entrada con `DisallowUnknownFields`.

Reglas transversales:

- Los campos **requeridos** se emiten siempre, incluso con valor cero (`false`, `[]`, `0`).
- Los booleanos opcionales de respuesta se emiten siempre que el contrato los documente; **no** se
  usa `omitempty` con `false` porque desaparecería un valor contractual.
- Los campos nullable se emiten como `null` explícito cuando no tienen valor; no se omiten.
- En PATCH, los campos opcionales usan **puntero** (`*bool`, `*string`) para distinguir “ausente”
  de “presente con valor falso/vacío”.
- Las fechas se serializan `RFC3339` UTC; el ETag de salida en E013/E025/E069 es `"v<version>"`.

```go
// E012/E013
type userProfileResponse struct {
    ID            string `json:"id"`
    Email         string `json:"email"`
    DisplayName   string `json:"displayName"`
    EmailVerified bool   `json:"emailVerified"`
    CreatedAt     string `json:"createdAt"`
    Version       int64  `json:"version"`
}

type profilePatchRequest struct {
    DisplayName *string `json:"displayName"`
}

// E015 — todos los campos documentados; content y requiresAcceptance son aditivos A8
type privacyNoticeResponse struct {
    Version            string `json:"version"`
    EffectiveAt        string `json:"effectiveAt"`
    Text               string `json:"text"`
    URL                string `json:"url"`
    Content            string `json:"content"`
    RequiresAcceptance bool   `json:"requiresAcceptance"`
}

// E016
type pageOfConsentResponse struct {
    Items      []consentResponse `json:"items"`
    NextCursor string            `json:"nextCursor,omitempty"`
}

// E017
type consentResponse struct {
    ID            string `json:"id"`
    NoticeVersion string `json:"noticeVersion"`
    Purpose       string `json:"purpose"`
    Granted       bool   `json:"granted"`
    RecordedAt    string `json:"recordedAt"`
}

type consentRequest struct {
    NoticeVersion string `json:"noticeVersion"`
    Purpose       string `json:"purpose"`
    Granted       *bool  `json:"granted"`
}

// E018/E019 — sin omitempty en booleanos ni en completedSteps
type onboardingResponse struct {
    CompletedSteps []string `json:"completedSteps"`
    Done           bool     `json:"done"`
    Version        int64    `json:"version"`
    Microphone     bool     `json:"microphone"`
    Notifications  bool     `json:"notifications"`
    Configured     bool     `json:"configured"`
    CompletedAt    *string  `json:"completedAt"`
}

type onboardingPatchRequest struct {
    CompletedSteps []string `json:"completedSteps"`
    Microphone     *bool    `json:"microphone"`
    Notifications  *bool    `json:"notifications"`
    Configured     *bool    `json:"configured"`
}

// E024/E025
type incomeResponse struct {
    Amount    string `json:"amount"`
    Currency  string `json:"currency"`
    Frequency string `json:"frequency"`
    Version   int64  `json:"version"`
}

type incomeUpdateRequest struct {
    Amount    *string `json:"amount"`
    Currency  *string `json:"currency"`
    Frequency *string `json:"frequency"`
}

// E068/E069 — timeZone se emite siempre como alias exacto de timezone
type preferencesResponse struct {
    Locale   string `json:"locale"`
    Timezone string `json:"timezone"`
    Version  int64  `json:"version"`
    TimeZone string `json:"timeZone"`
}

type preferencesPatchRequest struct {
    Locale   *string `json:"locale"`
    Timezone *string `json:"timezone"`
    TimeZone *string `json:"timeZone"`
}
```

Detalles vinculantes:

- `consentRequest.Granted` es puntero para que un JSON sin `granted` se rechace como 422 en lugar
  de interpretarse como `false`; `granted:false` es una decisión válida y se persiste.
- `onboardingPatchRequest.CompletedSteps` es requerido por el contrato; un `null` explícito se
  rechaza (400/422), un `[]` es válido y limpia los pasos.
- `onboardingResponse.CompletedAt` se emite `null` mientras `done=false`; al pasar a `done=true` se
  fija la marca UTC y se conserva mientras siga `done=true`.
- `preferencesPatchRequest`: si `timezone` y `timeZone` llegan con valores distintos → 422; si sólo
  llega uno, se aplica; si ninguno, el PATCH no cambia zona.
- `privacyNoticeResponse.RequiresAcceptance` se emite siempre. `Content` se emite siempre con el
  texto íntegro de la versión aprobada; no hay camino 503 ni texto provisional.
- `incomeUpdateRequest`: los tres campos son requeridos; un `null` o ausencia → 422.

Las páginas usan `items` siempre (array no nulo) y `nextCursor,omitempty`.

### 3.1 E015 — aviso servido por la API (sin web ni repositorio externo)

`internal/application/privacy` contiene el aviso versionado como recurso del backend:

```go
type Notice struct {
    Version            string
    EffectiveAt        time.Time
    Text               string
    URL                string
    Content            string
    RequiresAcceptance bool
}

func AvisoVigente() Notice
```

- Archivo embebido: `internal/application/privacy/aviso_2026_11_v2.txt` con `//go:embed`.
  Contenido exacto: **6158 bytes UTF-8**, salto de línea final, SHA-256
  `29b27611c6127e85ec3aca7601e38dec1fc188f305ffee6109d71e9cf1218a06`.
- `Version = "2026-11-v2"`; `URL = "https://d2lpmqy5w6x9es.cloudfront.net/api/v1/privacy/notices/current"`;
  `RequiresAcceptance = true`.
- `EffectiveAt` es una **constante** fijada en el release que publica la versión (UTC, no
  retroactiva); nunca se calcula con `time.Now()` por petición. Un test verifica que no es cero.
- `Text` es el resumen aprobado en el Gate; `Content` es el archivo embebido.
- **Obligatoriedad exacta:** `version`, `effectiveAt`, `text` y `url` son requeridos y se serializan
  sin `omitempty`. `content` y `requiresAcceptance` son opcionales en OpenAPI; esta versión emite
  ambos siempre y el DTO no usa `omitempty` en ninguno, para no perder valores falsos o vacíos.
- Prueba obligatoria `TestE015ContenidoSHA`: recalcula el SHA del archivo y falla si no coincide
  con el valor aprobado. Prueba de contrato: 200 con los seis campos, sin 503.
- **No hay dependencia de `nexumpr/nexum-privacidad`**, no se crea web ni landing y no se modifica
  ningún repositorio ajeno.
- **PENDIENTE_DECISION_HUMANA:** canal de contacto de privacidad efectivo. El texto no promete un
  medio específico; E015 no se declara funcional hasta que el responsable publique/confirme el canal
  y apruebe el contenido con la frase de D-E015.
- La **adopción del texto** exige la frase separada de D-E015; sin ella el PR de E015 no se fusiona.
  El desarrollo (handler, DTO, pruebas) no se bloquea.

## 4. Atributos DynamoDB

Se reutilizan los items existentes; **no** se redefinen ni se duplican.

| Item | Atributos leídos | Atributos escritos |
|---|---|---|
| `USER#<id>/PROFILE` | `userId`, `email`, `displayName`, `emailVerified`, `createdAt`, `version`, `status` | E013: `displayName`, `updatedAt`, `version` (ADD 1) |
| `USER#<id>/INCOME` | `amountCents`, `currency`, `frequency`, `version` | E025: `amountCents`, `currency`, `frequency`, `monthlyEquivalentCents`, `updatedAt`, `version` |
| `USER#<id>/ONBOARDING` | `completedSteps`, `microphone`, `notifications`, `configured`, `completedAt`, `version` | E019: los cinco + `updatedAt`, `version` |
| `USER#<id>/PREFERENCES` | `locale`, `timezone`, `version` | E068: defaults si no existe; E069: `locale`, `timezone`, `timeZone`, `updatedAt`, `version` |
| `USER#<id>/CONSENT#<RFC3339Nano>#<uuid>` | — | E017: `id`, `noticeVersion`, `purpose`, `granted`, `recordedAt`, `source`, `createdAt` |

Compatibilidad:

- `microphone`/`notifications` existentes son strings `pending|granted|denied` (E001). Se
  mantiene el string; la API los proyecta a boolean.
- `completedSteps` puede no existir en items antiguos: se trata como `[]`.
- `PREFERENCES` puede no existir: E068 lo crea con defaults; E069 también antes de comparar.
- Los consentimientos de E001 (`CONSENT#<version>`) se leen, no se modifican.

## 5. Condiciones DynamoDB (sin valores no usados)

Toda `ConditionExpression` incluye **sólo** placeholders que aparecen en su expresión. Regla de
revisión: por cada `ExpressionAttributeValues`, buscar su placeholder en `ConditionExpression` y
en `UpdateExpression`; si no aparece, el test falla (`TestTransaccionSinValoresNoUsados`).

Patrón de actualización:

```go
UpdateItemInput{
  Key: {PK: USER#<id>, SK: PROFILE|INCOME|ONBOARDING|PREFERENCES},
  UpdateExpression: "SET ... , updatedAt = :now ADD #version :one",
  ConditionExpression: "attribute_exists(PK) AND #version = :expected",
  ExpressionAttributeNames: {"#version": "version"},
  ExpressionAttributeValues: {":expected": N, ":one": N, ":now": S, ...},
}
```

Si `ConditionalCheckFailed`:
- E013/E025/E069 → **412** (ETag obsoleto) porque el cliente envió una versión.
- E019 → reintento único leyendo de nuevo; si vuelve a fallar, **409**.

## 6. Caché pública

`public_cache.go`:

```go
func GetOrLoadJSON(ctx context.Context, client *redis.Client, key string, ttl time.Duration, load func() ([]byte, error)) ([]byte, error)
```

- Si Redis es `nil` o falla, se ejecuta `load()` (fail-open).
- Claves: `nexum:v1:catalog:categories:2026-11-v1`, `nexum:v1:catalog:frequencies:2026-11-v1`,
  `nexum:v1:content:faq:2026-11-v1`, `nexum:v1:content:tips:2026-11-v1`.
- No se cachean datos de usuario.
- `Cache-Control: public, max-age=300` en las cuatro rutas públicas.

## 7. Límite de tasa

`identity_support.go` expone:

```go
func limitarPorIP(limiter *cacheinfra.AuthRateLimiter, clase string, next http.HandlerFunc) http.HandlerFunc
func limitarPorUsuario(limiter *cacheinfra.AuthRateLimiter, clase string, next http.HandlerFunc) http.HandlerFunc
```

- 120 req/min; identificador hasheado por el limitador existente.
- Si `limiter == nil`: se omite (se registra `429_NOT_EMITTED` en la documentación del endpoint).
- Si Redis falla: fail-open (se sirve la petición), salvo que el endpoint sea de autenticación
  (ninguno de esta oleada lo es).
- 429 con `Retry-After: <segundos>` y `ErrorEnvelope` `TOO_MANY_REQUESTS`.

## 8. Orden de PR por línea e inventario total

Inventario: **17 PR** = 1 documental (este Gate/RFC/diseño) + 1 de andamiaje + 15 de endpoint.
No se abre ningún PR adicional para corregir CI: cada fallo se resuelve dentro de su PR.

| Línea | PR 1 | PR 2 | PR 3 | PR 4 | PR 5 | PR 6 | PR 7 |
|---|---|---|---|---|---|---|---|
| A | E012 | E013 | E018 | E019 | E016 | E017 | E015 (si el contenido se aprueba) |
| B | E022 | E023 | E024 | E025 | — | — | — |
| C | E068 | E069 | E070 | E071 | — | — | — |

Cada PR se abre contra `main` con el andamiaje ya fusionado. Si un PR depende de otro de la misma
línea (E013 de E012, E019 de E018, E017 de E016, E025 de E024, E069 de E068), la rama se crea
desde la anterior **o** se rebasa sobre `main` cuando la anterior ya está fusionada; nunca se
duplica el código base.

E015 se sirve desde el backend con el aviso embebido y probado por SHA. **No** se cuenta como
funcional hasta que el responsable apruebe el contenido con la frase de D-E015; si no lo aprueba,
el PR de E015 no se fusiona y el contador máximo es 25/75. No hay 503 ni texto provisional.

## 9. Pruebas progresivas (sin stubs)

### 9.1 PR de andamiaje (0 endpoints)

Debe poder fusionarse con **cero** endpoints de Oleada 1. Pruebas:

- registro válido de un montaje (ID, método, ruta, función no nula);
- `panic` ante ID duplicado;
- `panic` ante `(método, ruta)` duplicado, aunque el ID sea distinto;
- orden de montaje determinista (IDs ordenados);
- sólo se aceptan IDs de la lista autorizada de 15; un ID futuro (E072+) se rechaza;
- `init()` no hace I/O ni lee variables de entorno;
- con cero registros, `montarOleada1` no registra ninguna ruta y el router responde 404.

No se registran handlers ficticios ni rutas de relleno para hacer pasar pruebas.

### 9.2 PR de cada endpoint

Cada PR prueba **exclusivamente** su endpoint y sus dependencias:

- ruta y método registrados por su `init()`;
- contrato, errores, aislamiento, CAS/concurrencia y persistencia;
- no exige que existan los otros 14 endpoints;
- no usa stubs de otros endpoints.

### 9.3 Ensayo de integración final

Se ejecuta cuando los 15 PR funcionales están preparados, antes del primer merge de endpoint:

- `tests/integration/oleada1_routes_test.go` exige **exactamente 15** rutas de Oleada 1, sin
  duplicados y sin rutas de Oleada 2;
- arranca el binario real y comprueba códigos de contrato (401/400/200 según ruta);
- regresión E001–E011;
- este test **no** se incluye en el andamiaje ni en PR individuales; se añade en el commit de
  integración cuando el conjunto está completo.

### 9.4 Pruebas transversales

- `tests/unit/transport/oleada1_contract_test.go`: serialización exacta de cada DTO y rechazo de
  campos desconocidos.
- `tests/unit/dynamo/oleada1_conditions_test.go`: ninguna expresión declara valores no usados.
- `tests/security/oleada1_authz_test.go`: usuario B no accede a recursos de A; tokens revocados
  rechazados por el middleware.

## 10. Entrega Android (al cierre)

Se generará `sdd/_control/H08/ENTREGA_ANDROID_OLEADA_1.md` con BASE_URL, los 15 pares
método/ruta, modelos request/response exactos, headers (`Authorization`, `If-Match`,
`Idempotency-Key` sólo donde el contrato lo exige), códigos de error, ejemplos sanitizados y
dependencias de onboarding/perfil/ingreso/preferencias. Sin tokens ni credenciales.
