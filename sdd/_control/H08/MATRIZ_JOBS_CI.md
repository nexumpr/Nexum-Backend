# I-080 — Matriz de jobs de CI

| Campo | Valor |
|---|---|
| Spec | `sdd/02-infraestructura/I-080-ci-backend` |
| Estado | `CI_VERDE_EN_PR`; ejecución real en PR #28 (run 42458688828) sobre `6497915f4c8e8d90a1f7f60c7f67d6353c6a69e0`; AWS `NOT_RUN` |
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
| `iac` | Validacion Terraform e IaC | `fmt -check`, `init -backend=false`, `validate` (bootstrap y `envs/h08i`), contrato Terraform e IaC, regresión FinOps | Terraform `1.16.6`; pruebas Python del repo | Sí |
| `contratos` | Contratos OpenAPI y herramientas | Validación OpenAPI H06; pruebas de herramientas y validadores | Python del runner | Sí |
| `cadena-suministro` | SBOM de dependencias | SBOM SPDX y publicación como artefacto | syft `v1.57.0` | Sí |

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

- Ejecución real verificada en PR #28 (run 42458688828): los seis jobs y `validar` en `pass` sobre
  `6497915f4c8e8d90a1f7f60c7f67d6353c6a69e0`.
- Branch protection: **BLOQUEADO_PLAN_GITHUB**. `gh api .../branches/main/protection` devuelve HTTP 403
  en el repositorio privado; no se activa ni se simula. Detalle en `GATES_PR.md` §0.
- La validación `terraform validate` acredita configuración válida, **no** infraestructura desplegada.
- El SBOM y los artefactos no sustituyen pruebas de runtime AWS.
- La cobertura mide sentencias del dominio con `-coverpkg=all` y deduplicación por bloque; no mide
  comportamiento en producción.
