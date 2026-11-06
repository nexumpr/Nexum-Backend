# Gate H08-A — Hotfix A-100 (CI funcional Go y DevSecOps)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/GATE_H08A_HOTFIX_A100_CI_GO.md` |
| Fecha | 2026-11-06 |
| Defecto | **D-08** — no existe CI funcional de Backend; A-100/A-110 exigen checks |
| Spec | A-100 (QA/DevSecOps); A-110 (checks de PR); dependencia I-100 |
| Base | `main` en `10f632799f00c998aaf648e1cb9d27c4b3e5bf52` |
| Rama | `fix/h08a-a100-ci-go` |
| Estado | **APROBADO — autorización humana expresa** |

## 1. Autorización

Frase humana recibida (2026-11-06): `APRUEBO_GATE_HOTFIX_A100_CI_GO`.

Cubre: workflow de CI, controles de calidad/seguridad, pruebas, documentación, rama, commit, push y
PR independiente. **No** cubre merge, A-030, A-090, I-030, RFC-004, AWS ni Terraform.

## 2. Alcance implementado

`.github/workflows/go-backend.yml`, con triggers `pull_request` (hacia `main`) y `push` (a `main`),
`permissions: contents: read`, concurrencia con cancelación y `timeout-minutes`.

| Job | Pasos |
|---|---|
| `calidad` | `gofmt` (falla si hay diferencias), `go vet ./...`, `go build ./...`, `go test ./... -count=1`, `go test -race` en `./cmd/...` y `./tests/unit/...` |
| `seguridad` | `go mod verify`, `govulncheck@v1.9.0`, `gosec@v2.30.0` (sin exclusiones) |
| `secretos` | `gitleaks@v8.30.1 detect --redact --exit-code 1` |
| `cadena-suministro` | `go mod verify` + SBOM SPDX con `syft@v1.57.0`, artefacto retenido 7 días |

Herramientas fijadas a versiones verificadas con `go list -m -versions`; Go fijado por `go.mod`
(1.27.3). Sin Docker, sin AWS, sin OIDC, sin secretos, sin `pull_request_target`.

## 3. Evidencia local previa (2026-11-06)

| Control | Resultado |
|---|---|
| `go build` / `go vet` / `go test ./...` | PASS |
| `govulncheck v1.9.0` | **PASS** (0 vulnerabilidades llamadas; 1 en módulo no invocada) |
| `gosec v2.30.0` | **FAIL — 1 HIGH**: `internal/infrastructure/security/hasher.go:126 G115` (conversión `int → uint32`), `-quiet`, sin supresiones |
| `gitleaks` | Se ejecuta en CI (no instalado localmente) |

## 4. Bloqueo detectado (fuera del alcance de A-100)

`G115` está en código de **A-050**, no en A-100. No se corrige ni se suprime aquí: una exclusión de un
hallazgo HIGH requiere decisión humana y procedimiento SDD. Consecuencia esperada: el job `seguridad`
del PR de A-100 quedará en **FAIL** hasta resolverlo.

Opciones para el responsable:

1. `APRUEBO_FIX_A050_GOSEC_G115` → acotar la conversión con comprobación de rango en `hasher.go`
   (cambio mínimo, rama propia de A-050).
2. `APRUEBO_EXCLUSION_GOSEC_G115` → exclusión documentada (el valor proviene de un SHA-256 fijo de 32
   bytes), registrada en A-100/checklist con justificación y revisión periódica.

## 5. Limitaciones de plataforma

- Repositorio privado con plan Free: `required checks` y branch protection **no disponibles** (403).
  Los checks se ejecutan, pero su obligatoriedad se verifica manualmente antes de cada merge.
- Consumo de Actions: el plan Free incluye minutos para repos privados; este workflow usa
  `ubuntu-latest` estándar y no servicios de pago. No se activa facturación.

## 6. Criterios de cierre

- Jobs ejecutados realmente en GitHub Actions (no sólo YAML).
- `calidad`, `seguridad`, `secretos`, `cadena-suministro` en verde, o bloqueo documentado con decisión.
- SDD PASS, SHA-256 exacto, OpenAPI 75.
- Merge con autorización humana específica sobre el HEAD resultante.
