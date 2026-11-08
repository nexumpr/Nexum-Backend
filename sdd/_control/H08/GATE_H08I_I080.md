# Gate H08-I — I-080 `ci-backend` (implementación local)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/GATE_H08I_I080.md` |
| Fecha | 2026-11-08 |
| Spec | `sdd/02-infraestructura/I-080-ci-backend/` (v0.1.0 DRAFT; spec/plan intactos) |
| Estado | **APRUEBO_GATE_I080_LOCAL REGISTRADO — IMPLEMENTADO_LOCAL (sin AWS)** |
| Main base | `21eb9cb540e4d4a39a0851a2c3f730fd78a0846f` (PR #27, I-070) |
| Rama | `feat/h08i-i080-ci-backend` |
| Autorización | `APRUEBO_GATE_I080_LOCAL` (mensaje humano independiente, 2026-11-08) |

## 1. Alcance

Implementación local del CI de Backend: workflow de GitHub Actions con seis jobs, umbral de cobertura
del dominio, validación Terraform en CI, validación de contratos OpenAPI, SBOM, inventario de
dependencias y documentación de gates de PR.

**No autoriza** ejecutar despliegues, `terraform apply/destroy`, `plan` contra AWS, credenciales cloud,
cambios de branch protection en el repositorio, ni iniciar I-090.

## 2. Implementado

| Requisito | Evidencia | Estado |
|---|---|---|
| INF-080-01 Eventos | `pull_request` a `main`/`release/**` y `push` a `main`; `gofmt`, `go vet`, `staticcheck`, `govulncheck`, `go test -race`, cobertura ≥ 80 %, `gosec`, gitleaks, dependencias, contrato OpenAPI H06, Terraform (`fmt`/`init -backend=false`/`validate`) y **escaneo de seguridad IaC real con Trivy HIGH/CRITICAL y prueba negativa**; acciones por SHA y herramientas por versión exacta; AWS-0011 bajo RFC-006 aprobada y **vigencia automatizada** (`verificar_vigencia_rfc006.py`) | IMPLEMENTADO_LOCAL |
| INF-080-02 PR | `GATES_PR.md`: seis checks requeridos, revisión obligatoria, sin force push, artefactos por `github.sha`; aplicación de branch protection **PENDIENTE_APLICACION_HUMANA** | DOCUMENTADO |
| INF-080-03 Seguridad CI | `permissions: contents: read`; sin `pull_request_target`, sin secretos, sin credenciales cloud, sin Docker | IMPLEMENTADO_LOCAL |
| INF-080-04 Pruebas | Cobertura de dominio 92 % local (umbral 80 %); pruebas de contrato Terraform/FinOps, OpenAPI y herramientas en CI; sin servicios cloud | IMPLEMENTADO_LOCAL |
| INF-080-05 Fallos | Sin `continue-on-error`, `|| true` ni `-exclude`; gitleaks con `--exit-code 1`; excepciones sólo por RFC | IMPLEMENTADO_LOCAL |
| INF-080-06 Versiones | Go desde `go.mod`; `go build -trimpath -buildvcs=true`; Terraform 1.16.6; SBOM e inventario de dependencias por SHA | IMPLEMENTADO_LOCAL |

## 3. Límites y bloqueos

- **Branch protection no aplicada**: `gh api .../branches/main/protection` devuelve **HTTP 403**
  («Upgrade to GitHub Pro or make this repository public») en el plan actual del repositorio privado. No se
  declara protección activa, no se cambia de plan y no se genera cargo; el procedimiento queda documentado
  en `GATES_PR.md` §0. Los checks de CI sí se ejecutan y se exigen por revisión humana.
- **Ejecución real del workflow**: verificada en PR #28, run 42458688828, SHA
  `6497915f4c8e8d90a1f7f60c7f67d6353c6a69e0`, con los seis jobs en `pass` y `validar` en `pass`. La
  evidencia se detalla en `REPORTE_EVIDENCIA_CI.md` §2; si cambia el HEAD, debe reasociarse.
- **Sin AWS**: no se invoca cuenta, no se lee state remoto y no se crea ningún recurso.
- **OIDC y credenciales de release** pertenecen a I-090 y a su Gate.
- **Cobertura**: mide sentencias del dominio; no sustituye pruebas de integración AWS/Redis/HTTP.
- **Escáner IaC**: Trivy `0.76.2` con severidad HIGH/CRITICAL y `--exit-code 1`. Tres excepciones
  acotadas en `.trivyignore.yaml` (ID + ruta + justificación, sin comodines): `AWS-0054` (D2/RFC-005,
  H-01 abierto), `AWS-0132` (límite SSE-S3, Gate I-070) y `AWS-0011` (**RFC-006 aprobada** el
  2026-11-08 con `APRUEBO_RFC_006_WAF_TEMPORAL`; caducidad 2026-12-02 o Gate de despliegue).
- **AWS-0011 (HIGH)**: la aceptación es **temporal y condicionada**; no autoriza implementar WAF ni
  desplegar AWS. Si caduca o se retira la excepción, el job `iac` vuelve a fallar. El despliegue sigue
  bloqueado por RFC-004, H-01 y las condiciones de RFC-006 §5.

## 4. Verificación local

`python3 -m unittest discover -s tests/infra/ci` (22 pruebas de contrato), pruebas Terraform (245),
FinOps, systemd, herramientas, Go `build/vet/test` y validador SDD integral. `terraform fmt -check`,
`init -backend=false` y `validate` en bootstrap y `envs/h08i`.

## 5. Cierre

Merge requiere autorización humana ligada a PR y HEAD exactos (A-110/R-004). No se declara
`AWS_DEPLOYED`, `CI_VERIFIED` ni `PRODUCTION_READY` desde este documento. I-090 no se inicia sin su
Gate.

**Estado de seguridad:** `AWS-0011` queda bajo RFC-006 aprobada (aceptación temporal, caducidad
2026-12-02). El despliegue AWS continúa **BLOQUEADO** por RFC-004, H-01 y las condiciones de RFC-006 §5;
la aprobación no autoriza WAF ni despliegue.
