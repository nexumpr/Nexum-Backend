# RFC-008 — Modernización del grafo CI/CD de Backend

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/RFC-008-MODERNIZACION_CI_CD.md` |
| Fecha | 2026-11-09 |
| Base | `main` `1e910d26507abc17257db5933c349e7a985bd8da` (post PR #35, I-150) |
| Specs afectadas | I-080 (principal), I-090 (consumidor del artefacto), con dependencias I-000, I-020, I-040, I-050, I-060, I-070, I-100, I-110, I-130, I-140, I-150 |
| Estado | **APROBADA (2026-11-09)** por `APRUEBO_GATE_I080_I090_MODERNIZACION_LOCAL` |
| Autoriza | implementación **local** de CI/CD. **No** autoriza merge, AWS, Terraform remoto ni despliegue |

## 1. Problema

El CI de I-080 ejecutaba seis jobs **sin ninguna dependencia** (`needs:`). No existía
detección de cambios, ni punto único de consolidación, ni reporte de hardening, ni SAST
adicional al de gosec/staticcheck. Consecuencias verificadas:

1. Ningún job podía fallar «por omisión»: un control omitido era indistinguible de uno
   superado, porque no había quien evaluara el conjunto.
2. `tests/infra/ci/test_ci_contract.py` (30 pruebas, contrato del propio CI) **no estaba
   conectado a ningún job**: hallazgo **H-03**.
3. `.github/workflows/sdd-validacion.yml` usaba `actions/checkout@v4` sin SHA de commit,
   en contradicción con la regla de acciones fijadas por SHA: hallazgo **H-02**.
4. El pipeline no producía ninguna evidencia consolidada por commit.

## 2. Alcance autorizado (implementación local)

### Etapa A — Detección

`detectar` ejecuta `herramientas/detectar_cambios_ci.py`, que clasifica los archivos
cambiados en los perfiles `go`, `api`, `sdd`, `infra` y `completo` y los publica como
salidas del job.

Reglas de seguridad del detector (**fail-closed**):

- `workflow_dispatch`, imposibilidad de calcular el diff o error de lectura ⇒ `completo`.
- Cambios en `.github/**`, `herramientas/**`, `go.mod`, `go.sum`, `.gitleaks*`,
  `.trivyignore.yaml`, `opencode.json`, `AGENTS.md` o `constitution.md` ⇒ `completo`.
- Una ruta no clasificada que no sea documentación (`.md`, `.txt`, `.rst`) ⇒ `completo`.
- Un cambio de workflows, dependencias o scripts de seguridad **nunca** es trivial.

### Etapa B — Validaciones en paralelo

| Job | Se ejecuta cuando | Contenido |
|---|---|---|
| `calidad` | `go` | gofmt, vet, staticcheck, build, binario ARM64 + SHA-256, test, `-race`, **fuzzing acotado**, cobertura ≥ 80 % del dominio, **evaluación de mutación** |
| `seguridad` | `go` | `go mod verify`, inventario, govulncheck, gosec + fixture negativo |
| `secretos` | **siempre** | gitleaks con baseline y excepciones acotadas |
| `iac` | `infra` o `sdd` | fmt, init `-backend=false`, validate, terraform test, Trivy + fixture negativo, vigencia RFC-006, contratos y FinOps |
| `contratos` | `api`, `sdd` o `go` | validar H06, breaking changes contra `origin/main`, herramientas y **contrato del propio CI** (H-03) |
| `cadena-suministro` | `go` | SBOM SPDX con Syft |
| `sast-semgrep` | `go` o `sdd` | Semgrep OSS 1.184.0 con reglas propias + fixture negativo |
| `integracion` | `go` | pruebas de capas reales con `-race` y arranque del **binario real** sondeado por TCP |

Todas las condiciones incluyen `!cancelled() && (needs.detectar.result != 'success' || …)`:
si la detección falla, las validaciones se ejecutan igualmente.

### Etapa C — Pruebas avanzadas

- **Integration testing**: `tests/integration` compone router + cadena de middleware real
  y verifica por sockets: liveness 200, readiness 503 sin dependencias, 404 no enumerable,
  405 con `Allow`, cabeceras de seguridad, trazabilidad W3C, CORS con allowlist, 413 por
  cuerpo excesivo, campo desconocido y **concurrencia** (40 obreros × 15 iteraciones). El
  job además arranca el binario de producción y repite las sondas por TCP.
  **No** se declara equivalencia con AWS: DynamoDB y Redis reales siguen `NOT_RUN`.
- **Mutation testing**: se evaluó `github.com/avito-tech/go-mutesting`. Resultado
  **`BLOCKED_TECNICO_MUTACION`** con evidencia verificable:
  1. no publica releases etiquetadas (`go list -m -versions` vacío); solo pseudo-versiones;
  2. compila (`go install …@v0.0.0-20261002081621-9ce5f731e321` → código 0) pero en tiempo
     de ejecución invoca `diff`; en este host no existe (`panic: exec: "diff": executable
     file not found`);
  3. los paquetes prioritarios (`internal/domain/money`, `internal/domain/financial`,
     `internal/application/financial`, `internal/application/identity`,
     `internal/infrastructure/security`) **no tienen pruebas en el mismo paquete**; con la
     configuración por defecto (`skip_without_test`) no se ejecutaría ninguna prueba y
     todos los mutantes «sobrevivirían», produciendo un número real pero engañoso.

  `herramientas/evaluar_mutacion.py` recalcula estas condiciones en CI y publica
  `mutacion.json`. **No se publican métricas de mutación** mientras el estado no sea
  `VIABLE`: un porcentaje en este estado sería un dato inventado.
- **Contract testing**: se mantiene el comparador fail-closed y la paridad semántica
  PyYAML de I-150. **Pact se descarta**: el Backend no tiene consumidores bajo control del
  repositorio y añadirlo exigiría contratos de consumidor que no existen.
- **DAST**: `NOT_RUN`. No hay entorno autorizado: el servidor local no expone dependencias
  reales y el DAST sobre AWS exige Gate de despliegue. Se conserva `preparar_dast.py`.

### Etapa D — Consolidación

- **`quality-gate`** (`if: always()`): evalúa `toJSON(needs)` y el perfil, y falla si hay
  `failure`, `cancelled`, `skipped` exigido, resultado ausente/desconocido o artefacto
  requerido no publicado. Acepta `skipped` únicamente cuando el perfil lo justifica y lo
  registra como tal.
- **`hardening-report`** (`if: always()`): produce `hardening-report.md` y `.json` con
  commit, PR, resultados, cobertura, SAST, SCA, secretos, mutación, integración, contratos,
  IaC, SBOM, fuzzing, DAST, omitidos con causa, RFC vigentes, riesgos residuales y estado
  SDD. Registra `NO_DISPONIBLE` en vez de inventar. Se publica como artefacto y en el
  resumen del run.
- **`ci-success`**: nodo terminal; solo declara éxito si `quality-gate` y
  `hardening-report` son `success`.

## 3. Fuera de alcance

Contratos H06/OpenAPI, `constitution.md`, arquitectura ratificada, Terraform, RFC-004,
RFC-005, RFC-006, RFC-007, backend funcional, E001–E075. **No** se contrata ningún
servicio (SonarCloud descartado: exige plan de pago en repositorio privado; CodeQL
descartado por plan/visibilidad), **no** se cambia el plan de GitHub y **no** se toca la
protección de ramas (`BLOQUEADO_PLAN_GITHUB`).

## 4. Permisos y superficie de seguridad

- `permissions: contents: read` + `actions: read` (solo para descargar los artefactos del
  propio run en el gate y el reporte). Sin escritura, sin `id-token`, sin secretos.
- Acciones fijadas por SHA de commit; herramientas por versión exacta
  (Semgrep `1.184.0`, staticcheck `v0.8.2`, govulncheck `v1.9.0`, gosec `v2.30.0`,
  gitleaks `v8.30.1`, Syft `v1.57.0`, Terraform `1.16.6`, Trivy `v0.76.2`).
- Sin `continue-on-error` en controles obligatorios.
- `cd-backend.yml` **no se modifica**: sigue siendo `workflow_dispatch` sobre `main`, con
  `CD_HABILITADO`, entorno `aws-deploy` y frase `AUTORIZO_DESPLIEGUE_<sha>`.

## 5. Hallazgos corregidos en esta RFC

| ID | Hallazgo | Corrección |
|---|---|---|
| H-02 | `sdd-validacion.yml` usaba `actions/checkout@v4` sin SHA | fijado por SHA de commit |
| H-03 | `tests/infra/ci` no se ejecutaba en ningún job | añadido al job `contratos` |
| H-04 | Ningún nodo evaluaba el conjunto de resultados | `quality-gate` + `ci-success` |
| H-05 | No existía evidencia consolidada por commit | `hardening-report` |

## 6. Riesgos

| Riesgo | Control |
|---|---|
| `skipped` mal interpretado | el gate recalcula la exigibilidad desde el perfil; prueba negativa de omisión injustificada |
| Detector defectuoso omite controles | fail-closed: error ⇒ `completo`; el gate falla si `detectar` no es `success` |
| Aumento del consumo de minutos | `detectar` evita jobs; `timeout-minutes` por job; sin servicios externos |
| Semgrep no validado localmente | el binario queda bloqueado por la política de Control de aplicaciones de Windows (`WinError 4551`); la validación real ocurre en el runner Linux de CI |
| Branch protection inexistente | se declara `BLOQUEADO_PLAN_GITHUB`; el gate es lógico y el merge sigue exigiendo autorización humana por SHA |
| Métricas de mutación inventadas | prohibido: se publica `BLOCKED_TECNICO_MUTACION` sin cifras |

## 7. Criterios de aceptación

1. El grafo no tiene ciclos y todo `needs` apunta a un job existente.
2. Todos los controles previos siguen presentes y bloqueantes.
3. El Quality Gate falla ante `failure`, `cancelled`, `skipped` injustificado, resultado
   desconocido y artefacto ausente.
4. El Hardening Report se genera también con controles en rojo.
5. `tests/infra/ci` cubre el contrato nuevo y se ejecuta en CI.
6. SDD y manifiesto SHA-256 exactos; `git diff --check` limpio.
7. Cero llamadas AWS.

## 8. Aprobación

`APRUEBO_GATE_I080_I090_MODERNIZACION_LOCAL` (2026-11-09), registrada en
`sdd/_control/H08/GATE_H08I_I080_I090_MODERNIZACION.md`. Es una autorización **local**:
no autoriza merge, ni AWS, ni Terraform remoto, ni despliegue.

## 9. Defecto D-01 corregido tras el merge

La primera ejecucion post-merge dejo al descubierto que el detector aceptaba como
«sin cambios» la salida vacia de `git show` sobre un commit de merge, omitiendo los ocho
jobs de validacion. Corregido en el mismo instrumento (herramienta de deteccion y sus
pruebas) el 2026-11-09: se compara contra el primer padre y una salida vacia ya no
cortocircuita; si ninguna estrategia devuelve rutas, el perfil pasa a `completo`.
Detalle y evidencia en el Gate §8.
