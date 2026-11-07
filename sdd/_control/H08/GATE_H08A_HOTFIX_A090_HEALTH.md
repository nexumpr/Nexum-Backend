# Gate H08-A — Hotfix A-090 (salud: liveness y readiness)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/GATE_H08A_HOTFIX_A090_HEALTH.md` |
| Fecha | 2026-11-06 |
| Defectos | **D-02** rutas no normativas · **D-06** `HealthRegistry` desconectado y readiness 200 sin dependencias |
| Spec | A-090 (observabilidad y salud); dependencias A-030 (router) y A-080 (degradación) |
| Base | `main` en `15462546e648868220de29594837c121ee12b8fb` |
| Rama | `fix/h08a-a090-health` |
| Estado | **APROBADO** (`APRUEBO_GATE_HOTFIX_A090_HEALTH`) |

## 1. Corrección

- Rutas normativas: **`GET /api/v1/health/live`** y **`GET /api/v1/health/ready`** servidas por
  `HealthRegistry` y registradas en el router real (`RegisterHealth`).
- `main` conecta el registro al runtime y llama a `BeginShutdown()` antes del drenado.
- Liveness: 200 si el proceso atiende; **no** consulta DynamoDB ni Redis.
- Readiness: **503** durante apagado, **503** si no hay dependencias críticas registradas, **503** si
  alguna falla o excede el timeout, 200 sólo si todas responden bien.
- Timeout acotado por comprobación con contexto; si una comprobación ignora el contexto, la sonda no
  se bloquea (los pendientes quedan `UNKNOWN` y la respuesta es 503).
- Respuesta sin mensajes de error de dependencias: sólo `UP`/`DOWN`/`UNKNOWN` por nombre.

## 2. Probes heredados

`/health` y `/ready` se conservan **como compatibilidad**, delegando en el mismo registro cuando está
conectado. Retirarlos afectaría a pruebas de A-030 y al bootstrap de I-020; su retirada requiere
decisión de control de cambios. Documentado, no silencioso.

## 3. Estado de dependencias reales

`run()` registra **cero** dependencias: DynamoDB y Redis aún no están integrados (I-040/I-050). Por
diseño, **readiness devuelve 503** hasta que existan comprobaciones reales. No se declara disponibilidad
productiva con dependencias simuladas: las pruebas usan funciones controladas y así se etiquetan.

## 4. Pruebas

`tests/unit/transport/health_normative_test.go`: live 200 con dependencia caída; ready 503 sin
dependencias; ready 200 con dependencia sana; ready 503 con dependencia caída; ready 503 y < 900 ms con
dependencia que ignora el contexto; ready 503 tras `BeginShutdown`; sin fuga de detalles internos; 405
con `Allow` al usar POST sobre ruta GET. Vía HTTP real en puerto efímero.

## 5. Evidencia local

`gofmt` limpio · `go build` PASS · `go vet` PASS · `go test ./...` PASS · `gosec` 0 · `govulncheck`
PASS · SDD PASS · OpenAPI 75. `go test -race` en Windows: **NOT_RUN** (requiere CGO); se ejecuta en CI
Linux.

## 6. Exclusiones

Sin cambios en I-030, RFC-004, Terraform, AWS ni en las 75 operaciones de negocio. Merge requiere
autorización humana independiente sobre el PR y SHA.
