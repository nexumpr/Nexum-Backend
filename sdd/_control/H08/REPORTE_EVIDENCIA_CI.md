# I-080 — Reporte de evidencia de CI

| Campo | Valor |
|---|---|
| Spec | `sdd/02-infraestructura/I-080-ci-backend` |
| Estado | `BLOQUEADO_RFC_006` — AWS-0011 HIGH sin autorización; el job `iac` falla a propósito y el PR #28 no es fusionable |
| AWS | `NOT_RUN` — este workflow no invoca AWS ni despliega |
| Fecha | 2026-11-08 |

## 1. Qué se ha implementado

- Workflow `.github/workflows/go-backend.yml` con seis jobs: `calidad`, `seguridad`, `secretos`,
  `iac`, `contratos` y `cadena-suministro`.
- Acciones de terceros fijadas por SHA de commit; herramientas Go por versión exacta.
- Cobertura del dominio con umbral 80 % mediante `herramientas/cobertura_dominio.py`.
- Validación Terraform local en CI (`fmt`, `init -backend=false`, `validate`) y pruebas de contrato.
- Escaneo de seguridad IaC con Trivy `0.76.2` (HIGH/CRITICAL, `--exit-code 1`) y prueba negativa con
  fixture aislado; excepciones acotadas en `.trivyignore.yaml` con justificación por ID y ruta.
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

### 2.1 Escaneo de seguridad IaC — ejecución real local

| Campo | Valor |
|---|---|
| Herramienta | Trivy `0.76.2` (release oficial de Aqua Security) |
| Comando | `trivy config --severity HIGH,CRITICAL --ignorefile .trivyignore.yaml --exit-code 1 infra/terraform` |
| Resultado | **FAIL (bloqueo de seguridad)** — 1 hallazgo HIGH no autorizado: `AWS-0011` (CloudFront sin WAF), exit 1 |
| Prueba negativa | `tests/fixtures/iac/inseguro` → Trivy exit **1**, 6 hallazgos HIGH y `FIXTURE_IAC_PASS` |
| CI | El job `iac` ejecuta ambos pasos; con AWS-0011 presente el check queda **rojo** hasta decisión humana |

Hallazgos HIGH/CRITICAL sobre `infra/terraform` y su tratamiento:

| ID | Severidad | Archivo | Tratamiento |
|---|---|---|---|
| `AWS-0054` | CRITICAL | `modules/balanceo/main.tf` | Excepción con RFC-005 ratificada: ALB interno HTTP tras CloudFront HTTPS; H-01 sigue abierto |
| `AWS-0132` | HIGH | `bootstrap/main.tf` | Excepción con decisión del Gate I-070 (`ACEPTADO_CON_LIMITE`): SSE-S3 sin CMK |
| `AWS-0011` | HIGH | `modules/balanceo/main.tf` | **SIN AUTORIZACIÓN**. No exento. Aceptación temporal propuesta en `RFC-006` (**PROPUESTO — NO APROBADO**). Bloquea `IAC_SCAN` y el merge |

`TERRAFORM_VALIDATE` acredita sintaxis/configuración válida; no sustituye el escáner. El escáner no usa
credenciales AWS ni backend remoto y no ejecuta `plan`/`apply`.

> **Cambio de criterio (2026-11-08):** la versión anterior de este reporte declaraba `IAC_SCAN PASS`
> manteniendo AWS-0011 en `.trivyignore.yaml` sin RFC aprobada. Eso era incorrecto: se retiró la
> excepción y el escaneo ahora falla, conforme a `GATES_PR.md` §4. No se declara `PASS` mientras el
> hallazgo siga sin decisión humana.

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

## 4. Clasificación de pruebas: ejecutado vs pendiente de servicios reales

| Ámbito | Qué se valida realmente | Estado |
|---|---|---|
| Dominio Go (money, financial, identity, cache, dynamo keys) | Unitarias con datos deterministas; sin red | **EJECUTADO** |
| Contratos OpenAPI 3.1 | Validación estática del documento y rutas | **EJECUTADO** |
| Adaptadores en memoria (DynamoDB/Redis simulados) | Contratos de puertos y concurrencia local | **EJECUTADO** (no es integración cloud) |
| Terraform (fmt/validate + Trivy + contratos) | Configuración, seguridad estática y contratos | **EJECUTADO** (sin AWS) |
| DynamoDB real | Persistencia, índices físicos, transacciones reales | **PENDIENTE** — I-040/I-090 y Gate de despliegue |
| Redis real | RBAC, TTL, invalidación entre nodos | **PENDIENTE** — I-050/I-090 y Gate de despliegue |
| HTTP extremo a extremo en AWS | Balanceo, TLS, 2 nodos | **PENDIENTE** — I-030/I-090; `AWS_NOT_RUN` |

No se declara integración cloud `PASS` a partir de mocks o adaptadores en memoria.

## 5. Límites

- `terraform validate` no acredita infraestructura desplegada.
- El SBOM no sustituye pruebas de seguridad dinámicas.
- La cobertura de sentencias no mide comportamiento en producción.
- Branch protection: **BLOQUEADO_PLAN_GITHUB** (HTTP 403 verificado en repositorio privado).
