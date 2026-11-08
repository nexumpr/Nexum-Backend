# I-100 — Matriz de escaneos y pruebas de seguridad

| Campo | Valor |
|---|---|
| Spec | `sdd/02-infraestructura/I-100-devsecops` |
| Estado | Herramientas de CI `EJECUTADAS`; DAST y pruebas AWS `NOT_RUN` |
| Bloqueo de merge | Cualquier CVE/hallazgo CRITICAL o HIGH explotable, salvo RFC firmada |

## 1. Escaneos automatizados en CI

| Control | Herramienta (versión fija) | Alcance | Bloquea | Estado |
|---|---|---|---|---|
| SAST Go | gosec `v2.30.0` sin exclusiones | `cmd`, `internal`, `pkg` | HIGH/CRITICAL | Ejecutado en CI |
| Prueba negativa SAST | gosec sobre fixture `math/rand` | `tests/fixtures/security/gosecinseguro` | debe fallar con G404 | Ejecutado en CI (`GOSEC_FIXTURE_PASS`) |
| Vulnerabilidades Go | govulncheck `v1.9.0` | módulos y símbolos alcanzables | HIGH/CRITICAL explotable | Ejecutado en CI |
| Lint estático | staticcheck `v0.8.2` | `./...` | cualquier hallazgo | Ejecutado en CI |
| Secretos | gitleaks `v8.30.1` + baseline revisado | historial completo | cualquier secreto | Ejecutado en CI |
| IaC (políticas) | Trivy `0.76.2` con HIGH/CRITICAL | `infra/terraform` | HIGH/CRITICAL | Ejecutado en CI |
| IaC (reglas SDD) | `herramientas/verificar_iac_devsecops.py` | SG públicos, IAM wildcard, S3 sin cifrado, `force_destroy`, ACL pública, state rastreado | cualquier hallazgo | Ejecutado en CI |
| SBOM | syft `v1.57.0` (SPDX JSON) | árbol de dependencias | n/a (evidencia) | Ejecutado en CI |
| Supply chain | acciones fijadas por SHA de commit + SHA-256 del binario + S3 `If-None-Match` | workflows y release | cualquier desviación | Verificado por contrato |

**Checksum ≠ firma:** el proyecto verifica SHA-256 e identidad de artefacto, pero **no** firma
criptográficamente los binarios (sin cosign/Sigstore). No se declara «artefacto firmado»; sólo
«integridad verificada por digest». La prevención de sustitución entre releases se apoya en clave de
release inmutable (`releases/<release_id>/...`), `If-None-Match: *` y digest en el Launch Template.
| Pruebas adversariales | Go tests `tests/security` | JWT, sesión, idempotencia, CORS, headers, body, rate limit | fallo de test | Ejecutado en CI |
| Dependencias GitHub | Dependency Review / Dependabot | PR | HIGH/CRITICAL | **PENDIENTE_DE_DISPONIBILIDAD** (no se asume activo) |

## 2. Pruebas ejecutadas localmente y en CI

- `go test ./tests/security/...`: JWT manipulado/`alg=none`/expirado, 401 sin token, contexto verificado,
  logout, replay de refresh, concurrencia de idempotencia, conflicto 409, aislamiento por usuario, CORS,
  cabeceras, cuerpo > 1 MB y rate limit 429.
- `go test ./...`: suites de dominio, transporte, seguridad y plataforma.
- `herramientas/tests`: escáner IaC (7 casos), prueba negativa de gosec, harness DAST (7 casos) y validadores.
- `herramientas/preparar_dast.py`: prepara un plan DAST (objetivo aislado, autorización por host, datos
  sintéticos, ventana ≤ 4 h, RPS ≤ 5, alcance) y **no ejecuta** nada. Sin `APRUEBO_GATE_DAST` devuelve
  `DAST_BLOCKED_AUTHORIZATION`.

## 3. Pruebas NO ejecutadas (requieren Gate)

| Prueba | Motivo | Gate responsable |
|---|---|---|
| DAST (ZAP/API scan) | Requiere entorno aislado autorizado, datos sintéticos, ventana y rate limits; prohibido contra producción o terceros | Gate de despliegue AWS / I-140 |
| Pruebas HTTP reales | Sin API desplegada (`AWS_NOT_RUN`) | I-090/I-140 |
| Integración DynamoDB/Redis reales | Sin servicios creados | I-040/I-050 + Gate AWS |
| Dependency Review | Requiere dependency graph y plan/visibilidad compatibles; no se declara activo | Decisión humana |
| Pentest manual | Fuera del alcance local; requiere autorización explícita | Responsable |

## 4. Evidencia

- Artefactos CI: `sbom-spdx-<sha>`, `iac-devsecops-<sha>`, `cobertura-<sha>`, `dependencias-<sha>`.
- Resultados por SHA en el PR; no se extrapolan a otros commits.
- Ausencia de secretos: gitleaks sobre historial; baseline limitado al manifiesto de integridad.
- Los escaneos no sustituyen pruebas de runtime ni garantizan ausencia de vulnerabilidades.
