# I-080 — Matriz de jobs de CI

| Campo | Valor |
|---|---|
| Spec | `sdd/02-infraestructura/I-080-ci-backend` |
| Estado | `BLOQUEADO_RFC_006` — escaneo IaC con AWS-0011 HIGH sin autorización; el job `iac` falla y el PR #28 no es fusionable hasta decisión humana; AWS `NOT_RUN` |
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
- **Excepciones acotadas** en `.trivyignore.yaml`: sólo dos IDs y rutas concretas, cada uno con
  justificación. No hay allowlist global ni `*`.
  - `AWS-0054` (HTTP en ALB interno): decisión D2 ratificada en RFC-005; H-01 sigue abierto.
  - `AWS-0132` (S3 sin CMK): límite `ACEPTADO_CON_LIMITE` del Gate I-070 y su matriz de state.
  - `AWS-0011` (CloudFront sin WAF): **NO exento**. Sin autorización previa; se propone aceptación
    temporal en `RFC-006-EXCEPCION_WAF_I030.md` (**PROPUESTO — NO APROBADO**). Mientras no se apruebe,
    el job `iac` falla con AWS-0011 HIGH y I-080 queda `BLOQUEADO_RFC_006`.
- Cualquier hallazgo HIGH/CRITICAL distinto de las dos excepciones autorizadas **hace fallar el job**.

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

- Ejecución real verificada en PR #28 (run 42511533667) con el escáner IaC: el job `iac` falla ahora
  por **AWS-0011 HIGH** al retirarse la excepción sin autorización; el resto de checks permanece verde.
- Branch protection: **BLOQUEADO_PLAN_GITHUB**. `gh api .../branches/main/protection` devuelve HTTP 403
  en el repositorio privado; no se activa ni se simula. Detalle en `GATES_PR.md` §0.
- `IAC_SCAN` no es `PASS` mientras AWS-0011 no tenga decisión humana (`RFC-006` PROPUESTO).
- La validación `terraform validate` acredita configuración válida, **no** infraestructura desplegada.
- El SBOM y los artefactos no sustituyen pruebas de runtime AWS.
- La cobertura mide sentencias del dominio con `-coverpkg=all` y deduplicación por bloque; no mide
  comportamiento en producción.
