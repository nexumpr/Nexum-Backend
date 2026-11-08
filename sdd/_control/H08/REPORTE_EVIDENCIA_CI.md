# I-080 — Reporte de evidencia de CI

| Campo | Valor |
|---|---|
| Spec | `sdd/02-infraestructura/I-080-ci-backend` |
| Estado | `IMPLEMENTADO_LOCAL`; ejecución real registrada en el PR de I-080 |
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

## 2. Evidencia a completar con la ejecución real

| Campo | Valor |
|---|---|
| Run ID | *(pendiente de la primera ejecución del PR)* |
| SHA evaluado | *(pendiente)* |
| `calidad` | *(pendiente)* |
| `seguridad` | *(pendiente)* |
| `secretos` | *(pendiente)* |
| `iac` | *(pendiente)* |
| `contratos` | *(pendiente)* |
| `cadena-suministro` | *(pendiente)* |
| Conclusión global | *(pendiente)* |

No se declara `PASS` hasta que exista una ejecución real asociada a un SHA. `IMPLEMENTADO_LOCAL`
significa que los archivos y pruebas locales existen y pasan; **no** que el workflow haya corrido.

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
