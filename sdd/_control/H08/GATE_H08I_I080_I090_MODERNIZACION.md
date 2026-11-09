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
| Fixture SAST inseguro | `tests/fixtures/semgrep/inseguro/main.go` |
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
