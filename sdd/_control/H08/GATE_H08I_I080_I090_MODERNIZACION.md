# Gate H08-I — I-080 / I-090 modernización de CI/CD (implementación local)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/GATE_H08I_I080_I090_MODERNIZACION.md` |
| Fecha | 2026-11-09 |
| Spec | `sdd/02-infraestructura/I-080-ci-backend/` (principal) y `I-090-cd-backend/` (consumidor) |
| Instrumento | `sdd/_control/H08/RFC-008-MODERNIZACION_CI_CD.md` |
| Estado | **APRUEBO_GATE_I080_I090_MODERNIZACION_LOCAL REGISTRADO — IMPLEMENTADO_LOCAL (sin AWS)** |
| Base | `main` `1e910d26507abc17257db5933c349e7a985bd8da` (PR #35, I-150) |
| Rama | `feat/ci-modernizacion-grafo` |
| Autorización | `APRUEBO_GATE_I080_I090_MODERNIZACION_LOCAL` (mensaje humano independiente, 2026-11-09) |

## 1. Alcance autorizado

Reescribir el grafo de `.github/workflows/go-backend.yml` conforme a RFC-008: detección
de cambios fail-closed, validaciones paralelas con `needs:` reales, SAST Semgrep,
pruebas de integración de capas reales con arranque del binario, evaluación determinista
de mutation testing, Quality Gate, Hardening Report y nodo terminal `CI Success`.
Incluye el endurecimiento de `sdd-validacion.yml` (acción fijada por SHA) y la conexión
de `tests/infra/ci` al pipeline.

## 2. No autoriza

- Merge del PR asociado.
- `terraform plan/apply/destroy`, `aws` mutante ni lectura AWS.
- Despliegue, publicación de artefactos en AWS ni arranque de instancias.
- Modificar `cd-backend.yml` (permanece intacto), contratos H06, `constitution.md`,
  arquitectura ratificada, Terraform, RFC-004/005/006/007 ni E001–E075.
- Cambiar el plan de GitHub, la visibilidad del repositorio o la protección de ramas.
- Declarar `AWS_VERIFIED`, `HTTP_AWS_VERIFIED` o `TWO_NODES_VERIFIED`.

## 3. Entregables

| Elemento | Ruta |
|---|---|
| Detector de cambios | `herramientas/detectar_cambios_ci.py` |
| Quality Gate | `herramientas/quality_gate_ci.py` |
| Hardening Report | `herramientas/hardening_report.py` |
| Evaluación de mutación | `herramientas/evaluar_mutacion.py` |
| Reglas SAST propias | `herramientas/semgrep/nexum.yml` |
| Verificador del fixture SAST | `herramientas/verificar_fixture_semgrep.py` |
| Fixture SAST inseguro | `herramientas/semgrep/fixtures/inseguro/main.go` |
| Fuzzing del binding | `tests/unit/transport/fuzz_binding_test.go` |
| Integración de capas | `tests/integration/http_integracion_test.go` |
| Contrato del CI | `tests/infra/ci/test_ci_contract.py` |
| Grafo | `.github/workflows/go-backend.yml` |

## 4. Verificación exigida

1. `python3 -m unittest discover -s tests/infra/ci` (contrato del grafo y documentos).
2. `python3 -m unittest discover -s herramientas/tests` (herramientas).
3. Suites de infraestructura sin regresión: `tests/infra/{terraform,finops,operacion,respaldo,publicacion}`.
4. `go build ./...`, `go vet ./...`, `gofmt -l`, pruebas Go y `-race` en CI Linux.
5. `python3 herramientas/validar_sdd_integral.py` y `sdd/03-endpoints/herramientas/validar_h06.py`.
6. `git diff --check` sin salida.
7. CI del PR en verde, incluidos los jobs nuevos.

## 5. Estado de ejecución

- Implementación local completada; resultados reales en el PR y en `REPORTE_EVIDENCIA_CI.md`.
- `AWS_NOT_RUN`, `DEPLOY_NOT_RUN`, `HTTP_AWS_NOT_RUN`, `TWO_NODES_AWS_NOT_VERIFIED`.
- `DAST_NOT_RUN` (sin entorno autorizado).
- `MUTATION = BLOCKED_TECNICO_MUTACION` (sin métricas publicadas).
- Semgrep no ejecutable en el host de desarrollo (Control de aplicaciones de Windows,
  `WinError 4551`); validación efectiva en el runner Linux de CI.

## 6. Fuera de este Gate

El primer despliegue AWS requiere un Gate independiente que declare plan FREE vigente,
créditos, IAM, TLS, SKU de Redis, backups y verificación de dos nodos. Este Gate **no**
habilita ese paso ni lo prepara como ejecución.

## 7. Registro de integracion — 2026-11-09

- `APRUEBO_MERGE_PR_36_SHA_17fc73d45a7528a05f3e17767c9e30b7605d82f5` concedido por el
  responsable; HEAD verificado por igualdad exacta y `--match-head-commit`; **13/13**
  checks en `pass` (`validar` incluido) y `MERGEABLE`/`CLEAN`.
- **PR #36 `MERGED`** (`2026-11-09T15:47:20Z`); merge commit y `main` =
  **`a441a0cada5c22fc3bdf9c094c7d11bc8aa2540c`** (padres `1e910d2` y `17fc73d`);
  `git pull --ff-only` dejo `main` local identico a `origin/main`.
- CI post-merge sobre `main` en `a441a0c`: **Go Backend CI `success`** (run
  `42648177960`) y **SDD documental `success`** (run `42648178034`).
- Estado: **INTEGRADO_MAIN — NO DESPLEGADO**. `AWS_NOT_RUN`, `DEPLOY_NOT_RUN`,
  `HTTP_AWS_NOT_RUN`, `TWO_NODES_AWS_NOT_VERIFIED`, `DAST_NOT_RUN`.

## 8. Correccion D-01 — deteccion en commits de merge (2026-11-09)

- **Defecto observado en la ejecucion post-merge**: en el run `42648177960` solo se
  ejecutaron `detectar`, `secretos` y los consolidadores; el resto quedo `skipped` y el
  Quality Gate lo acepto.
- **Causa raiz**: `git show --name-only <merge>` devuelve **vacio** (git no muestra el
  diff de un merge por defecto) y `archivos_desde_git` aceptaba ese vacio como «sin
  cambios», cortocircuitando antes de la comparacion con el primer padre.
- **Correccion**: se prueba **primero** `git diff --name-only <sha>^1 <sha>`, despues
  `git show`, y una salida vacia **ya no** se acepta como «sin cambios»: se pasa a la
  siguiente estrategia y, si todas quedan vacias, se devuelve `None` para activar el
  perfil completo (fail-closed).
- **Pruebas nuevas**: merge con `show` vacio y diff contra el primer padre con rutas;
  todas las estrategias vacias -> `None`; `workflow_dispatch` -> `None`; el PR usa
  `base...head` como primera estrategia.
- Alcance: unicamente la herramienta de deteccion y sus pruebas. Sin AWS, sin cambios en
  el grafo, sin cambios en `cd-backend.yml`.

## 9. Registro de integracion del PR #37 — 2026-11-09

- `APRUEBO_MERGE_PR_37_SHA_d2708abc4c742642c1ab9641f441aa5efa406c1b` concedido por el
  responsable; HEAD verificado por igualdad exacta y `--match-head-commit`; **13/13**
  checks en `pass` y `MERGEABLE`/`CLEAN`.
- **PR #37 `MERGED`** (`2026-11-09T16:03:34Z`); merge commit y `main` =
  **`2ebc9f2398ad344fa826ed0cf88fe07b4710fedc`** (padres `a441a0c` y `d2708ab`).
- **D-01 verificado de extremo a extremo**: el mismo tipo de commit de merge que en el
  run `42648177960` dejo **8 jobs `skipped`** ahora ejecuta **12/12 en `success`** en el
  run `42650883409` (Go Backend CI) y `42650883410` (SDD documental), sin omisiones y sin
  intervencion manual.
- Deteccion local sobre el merge real: **8 archivos** y perfil completo, donde antes
  devolvia cero.
- Estado: **INTEGRADO_MAIN — NO DESPLEGADO**. `AWS_NOT_RUN`, `DEPLOY_NOT_RUN`,
  `HTTP_AWS_NOT_RUN`, `TWO_NODES_AWS_NOT_VERIFIED`, `DAST_NOT_RUN`,
  `MUTATION = BLOCKED_TECNICO_MUTACION`.
