# I-080 — Matriz de jobs de CI

| Campo | Valor |
|---|---|
| Spec | `sdd/02-infraestructura/I-080-ci-backend` |
| Estado | `IMPLEMENTADO_LOCAL` — RFC-006 aprobada; CI en verificación sobre el HEAD de la corrección; AWS `NOT_RUN` |
| Workflow | `.github/workflows/go-backend.yml` |
| Herramientas | Acciones fijadas por SHA de commit; herramientas Go por versión exacta |

## 1. Disparadores

- `pull_request` contra `main` y `release/**`.
- `push` a `main`.
- `permissions: contents: read` global; sin permisos de escritura ni credenciales cloud.
- `concurrency` por workflow y ref, con cancelación de ejecuciones previas.

## 2. Jobs y comprobaciones

| Job | Nombre del check | Comprobaciones | Herramienta (versión fija) | Bloquea merge |
|---|---|---|---|---|
| `calidad` | Formato, vet, build y pruebas | `gofmt -l`, `go vet`, `staticcheck`, `go build -trimpath -buildvcs=true`, `go test`, `go test -race`, cobertura de dominio ≥ 80 % | Go de `go.mod`; staticcheck `v0.8.2`; `herramientas/cobertura_dominio.py` | Sí |
| `seguridad` | Analisis estatico y vulnerabilidades | `go mod verify`, inventario `go list -m all`, `govulncheck`, `gosec` sin exclusiones | govulncheck `v1.9.0`; gosec `v2.30.0` | Sí |
| `secretos` | Escaneo de secretos | gitleaks sobre historial completo con baseline revisado | gitleaks `v8.30.1` | Sí |
| `iac` | Validacion Terraform e IaC | `fmt -check`, `init -backend=false`, `validate` (bootstrap y `envs/h08i`), **escaneo de seguridad Trivy HIGH/CRITICAL con prueba negativa**, contrato Terraform e IaC, regresión FinOps | Terraform `1.16.6`; Trivy `0.76.2` vía setup-trivy `v0.3.1` | Sí |
| `contratos` | Contratos OpenAPI y herramientas | Validación OpenAPI H06; pruebas de herramientas y validadores | Python del runner | Sí |
| `cadena-suministro` | SBOM de dependencias | SBOM SPDX y publicación como artefacto | syft `v1.57.0` | Sí |

### 2.1 Escaneo de seguridad IaC (INF-080-01)

El job `iac` ejecuta un escáner independiente de configuraciones inseguras, además de `terraform validate`:

- **Trivy `0.76.2`** (`config`) sobre `infra/terraform` (roots y módulos), severidad **HIGH/CRITICAL** y
  `--exit-code 1`. Instalado con `aquasecurity/setup-trivy` fijado por SHA de commit.
- **Prueba negativa**: el mismo escáner analiza `tests/fixtures/iac/inseguro` (fixture público y aislado,
  fuera de la infraestructura desplegable) y `herramientas/verificar_fixture_iac.py` exige exit 1 y al
  menos un hallazgo HIGH/CRITICAL real. Un fallo por ruta o JSON inválido no se acepta como detección.
- **Excepciones acotadas** en `.trivyignore.yaml`: tres IDs y rutas concretas, cada uno con
  justificación. No hay allowlist global ni `*`.
  - `AWS-0054` (HTTP en ALB interno): decisión D2 ratificada en RFC-005; H-01 sigue abierto.
  - `AWS-0132` (S3 sin CMK): límite `ACEPTADO_CON_LIMITE` del Gate I-070 y su matriz de state.
  - `AWS-0011` (CloudFront sin WAF): **RFC-006 APROBADA** el 2026-11-08 (`APRUEBO_RFC_006_WAF_TEMPORAL`).
    Aceptación temporal con controles compensatorios, condiciones para el primer despliegue y caducidad
    `2026-12-02` (registrada también como `expired_at` en el ignore). No autoriza desplegar AWS ni
    implementar WAF; al caducar, el escáner vuelve a fallar.
- Cualquier hallazgo HIGH/CRITICAL distinto de las tres excepciones autorizadas **hace fallar el job**.
- **Vigencia automatizada**: `herramientas/verificar_vigencia_rfc006.py` se ejecuta en el job `iac` y falla
  si la RFC-006 no está `APROBADA`, si está revocada, si la fecha actual supera la caducidad, si la
  caducidad de la RFC y la de `.trivyignore.yaml` no coinciden o si existe un Gate de despliegue AWS
  aprobado. La excepción no puede sobrevivir a su autorización.

## 3. Artefactos

| Artefacto | Nombre | Retención | Contenido |
|---|---|---|---|
| Cobertura | `cobertura-<sha>` | 7 días | `coverage.out` (sin datos personales) |
| Dependencias | `dependencias-<sha>` | 7 días | `go list -m all` |
| SBOM | `sbom-spdx-<sha>` | 7 días | `sbom.spdx.json` |

Todos los nombres incorporan `github.sha` (revisión de origen). No se suben estados de Terraform,
credenciales, `.env`, secretos ni respuestas con datos personales.

## 4. Política de fallos

Cualquier `fail` de un job o de un paso bloquea el merge. No se admite `continue-on-error`, `|| true`
ni supresión de hallazgos para obtener verde. Las excepciones requieren RFC firmada con vigencia y
riesgo residual (`GATES_PR.md` §4).

## 5. Coste

Sólo minutos de GitHub Actions del plan del repositorio. No se invoca AWS, no se crean recursos y no
hay gasto cloud en este workflow. El `terraform init -backend=false` descarga el provider desde el
registro público; no accede a state remoto ni a la cuenta.

## 6. Límites declarados

- Ejecución real verificada en PR #28 (run 42513104875) sin la excepción: el job `iac` falló con
  AWS-0011 HIGH y exit 1, confirmando que el escáner detecta el riesgo. Con RFC-006 aprobada, la
  excepción se reincorpora acotada y el job vuelve a verde en el nuevo HEAD.
- Branch protection: **BLOQUEADO_PLAN_GITHUB**. `gh api .../branches/main/protection` devuelve HTTP 403
  en el repositorio privado; no se activa ni se simula. Detalle en `GATES_PR.md` §0.
- `IAC_SCAN` es `PASS` sólo con las tres excepciones autorizadas; AWS-0011 caduca el 2026-12-02.
- La validación `terraform validate` acredita configuración válida, **no** infraestructura desplegada.
- El SBOM y los artefactos no sustituyen pruebas de runtime AWS.
- La cobertura mide sentencias del dominio con `-coverpkg=all` y deduplicación por bloque; no mide
  comportamiento en producción.

## Grafo modernizado (RFC-008, 2026-11-09)

Estado: `IMPLEMENTADO_LOCAL` — sin AWS. La matriz anterior describe los seis jobs
originales; este apartado documenta el grafo vigente.

| Job | Nombre visible | Se ejecuta cuando | Artefacto |
|---|---|---|---|
| `detectar` | Deteccion de cambios | siempre | — |
| `calidad` | Formato, vet, build y pruebas | `go` | `binario-<sha>`, `cobertura-<sha>`, `mutacion-<sha>` |
| `seguridad` | Analisis estatico y vulnerabilidades | `go` | `dependencias-<sha>` |
| `secretos` | Escaneo de secretos | siempre | — |
| `iac` | Validacion Terraform e IaC | `infra` o `sdd` | `iac-devsecops-<sha>` |
| `contratos` | Contratos OpenAPI y herramientas | `api`, `sdd` o `go` | — |
| `cadena-suministro` | SBOM de dependencias | `go` | `sbom-spdx-<sha>` |
| `sast-semgrep` | SAST Semgrep | `go` o `sdd` | `semgrep-<sha>` |
| `integracion` | Integracion HTTP y concurrencia | `go` | `integracion-<sha>` |
| `quality-gate` | PR Quality Gate | siempre (`if: always()`) | `quality-gate-<sha>` |
| `hardening-report` | Hardening Report | siempre (`if: always()`) | `hardening-report-<sha>` |
| `ci-success` | CI Success | siempre (`if: always()`) | — |

Reglas del grafo:

- `detectar` es la raiz; toda validacion depende de ella.
- Si `detectar` falla, las validaciones se ejecutan igualmente (fail-closed) y el
  Quality Gate falla porque el resultado de `detectar` no es `success`.
- `quality-gate` depende de los nueve jobs anteriores y evalua `toJSON(needs)`.
- `hardening-report` depende del gate y se genera tambien con controles en rojo.
- `ci-success` solo depende de `quality-gate` y `hardening-report`.
- Ningun control admite `continue-on-error`.
- `cd-backend.yml` no forma parte de este grafo y no se ejecuta en PR ni push.
- `AWS_NOT_RUN`, `DEPLOY_NOT_RUN`, `DAST_NOT_RUN`, `MUTATION = BLOCKED_TECNICO_MUTACION`.
