# I-080 — Reporte de evidencia de CI

| Campo | Valor |
|---|---|
| Spec | `sdd/02-infraestructura/I-080-ci-backend` |
| Estado | `CI_VERDE_EN_PR` — ejecución real sobre `6497915f4c8e8d90a1f7f60c7f67d6353c6a69e0` |
| AWS | `NOT_RUN` — este workflow no invoca AWS ni despliega |
| Fecha | 2026-11-08 |

## 1. Qué se ha implementado

- Workflow `.github/workflows/go-backend.yml` con seis jobs: `calidad`, `seguridad`, `secretos`,
  `iac`, `contratos` y `cadena-suministro`.
- Acciones de terceros fijadas por SHA de commit; herramientas Go por versión exacta.
- Cobertura del dominio con umbral 80 % mediante `herramientas/cobertura_dominio.py`.
- Validación Terraform local en CI (`fmt`, `init -backend=false`, `validate`) y pruebas de contrato.
- Artefactos nombrados con `github.sha` (cobertura, dependencias, SBOM).
- Documentos de gobierno: `MATRIZ_JOBS_CI.md` y `GATES_PR.md`.

## 2. Evidencia de la ejecución real (PR #28)

Workflow: `Go Backend CI`, run **42458688828**; workflow documental, run **42458688876**.
SHA evaluado: `6497915f4c8e8d90a1f7f60c7f67d6353c6a69e0`.

| Job / check | Resultado | Duración |
|---|---|---|
| `Formato, vet, build y pruebas` | **pass** | 2m48s |
| `Analisis estatico y vulnerabilidades` | **pass** | 1m56s |
| `Escaneo de secretos` | **pass** | 27s |
| `Validacion Terraform e IaC` | **pass** | 30s |
| `Contratos OpenAPI y herramientas` | **pass** | 5s |
| `SBOM de dependencias` | **pass** | 3m5s |
| `validar` (SDD documental) | **pass** | 6s |

Conclusión global: **success** en los siete checks sobre el SHA indicado. Artefactos publicados con
`cobertura-<sha>`, `dependencias-<sha>` y `sbom-spdx-<sha>` (retención 7 días). La cobertura del
dominio superó el umbral en la ejecución (`COBERTURA_DOMINIO_PASS`).

Nota: si el HEAD del PR cambia (documentación o correcciones), la evidencia debe reasociarse al nuevo
SHA; este registro no se extrapola a commits posteriores.

## 3. Cobertura local observada (no es evidencia de CI)

Medición local con `go test ./tests/... -coverpkg=all` y deduplicación por bloque:

| Paquete de dominio | Cobertura |
|---|---|
| `cache` | 100.0 % |
| `dynamo` | 100.0 % |
| `financial` | 84.0 % |
| `identity` | 100.0 % |
| `money` | 93.8 % |
| **Total dominio** | **92.0 %** |

La cifra se recalcula en cada ejecución de CI; la tabla es una referencia del entorno local.

## 4. Límites

- `terraform validate` no acredita infraestructura desplegada.
- El SBOM no sustituye pruebas de seguridad dinámicas.
- La cobertura de sentencias no mide comportamiento en producción.
- Branch protection sigue `PENDIENTE_APLICACION_HUMANA`.
