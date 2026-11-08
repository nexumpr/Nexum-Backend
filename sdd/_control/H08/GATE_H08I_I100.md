# Gate H08-I — I-100 `devsecops` (implementación local)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/GATE_H08I_I100.md` |
| Fecha | 2026-11-08 |
| Spec | `sdd/02-infraestructura/I-100-devsecops/` (v0.1.0 DRAFT; spec/plan intactos) |
| Estado | **APRUEBO_GATE_I100_LOCAL REGISTRADO — IMPLEMENTADO_LOCAL (sin AWS)** |
| Main base | `a67ea4b929e3ca5a9fe246f68f852931e516eb5d` (PR #29, I-090) |
| Rama | `feat/h08i-i100-devsecops` |
| Autorización | `APRUEBO_GATE_I100_LOCAL` (mensaje humano independiente, 2026-11-08) |

## 1. Alcance

Implementación local de DevSecOps: modelo de amenazas, criterios de vulnerabilidad, matriz de escaneos,
escáner IaC propio, pruebas adversariales y conexión con el CI existente. Sin AWS, sin DAST y sin
despliegue.

**No autoriza** DAST contra ningún entorno, pentest, escaneo de terceros, cambios AWS, merge automático
ni inicio de I-110. No modifica I-060, I-080, I-090 ni sus contratos.

## 2. Implementado

| Requisito | Evidencia | Estado |
|---|---|---|
| INF-100-01 Threat model | `MODELO_AMENAZAS.md`: 6 superficies (S1–S6), STRIDE, riesgos priorizados (IDOR, replay, money, SSRF) y límites | IMPLEMENTADO_LOCAL |
| INF-100-02 SAST/SCA | gosec, govulncheck, staticcheck ya en CI; criterios HIGH/CRITICAL y RFC en `CRITERIOS_VULNERABILIDADES.md` | IMPLEMENTADO_LOCAL |
| INF-100-03 Secrets/SBOM | gitleaks con baseline acotado, SBOM SPDX por SHA, acciones fijadas por SHA | IMPLEMENTADO_LOCAL |
| INF-100-04 DAST | Procedimiento y **harness** `herramientas/preparar_dast.py` (objetivo aislado, autorización por host, datos sintéticos, ventana ≤ 4 h, RPS ≤ 5, alcance; no ejecuta) y **NO ejecutado** (requiere `APRUEBO_GATE_DAST` y entorno aislado) | NOT_RUN / BLOCKED_AUTHORIZATION |
| INF-100-05 IaC | Trivy HIGH/CRITICAL + `herramientas/verificar_iac_devsecops.py` (SG público, IAM wildcard, S3 sin cifrado, `force_destroy`, ACL pública, state rastreado) en CI | IMPLEMENTADO_LOCAL |
| INF-100-06 Pruebas adversariales | `tests/security/adversarial_test.go` + `rate_limit_test.go` + fixture gosec negativo: JWT manipulado/`alg=none`/expirado, 401, contexto, logout/replay, idempotencia, 409, aislamiento, CORS, cabeceras, body, 429 local; caracterización de defectos A-030 y coordinación simulada A-080 | IMPLEMENTADO_LOCAL con límites |

## 3. Límites y bloqueos

- **DAST no ejecutado**: prohibido contra producción/terceros; requiere entorno aislado, datos sintéticos
  y ventana autorizada. Permanece `NOT_RUN`.
- **Dependency Review/Dependabot**: `PENDIENTE_DE_DISPONIBILIDAD`; no se declara activo. SCA se cubre con
  govulncheck.
- **Endpoints E001–E075** no implementados: las pruebas de autorización son de componentes, no de API
  completa.
- **Riesgo residual**: un access token sigue válido hasta 10 minutos tras detectar replay (el middleware
  no consulta sesión por request); registrado en el modelo de amenazas, no oculto.
- **D-100-01 / D-100-02 (A-030/A-080)**: rate limit HTTP local por instancia y confianza en
  `X-Forwarded-For` sin proxy validado; el limitador distribuido no está cableado y no hay Redis real ni
  dos nodos. Probado localmente y caracterizado; **no corregido** (pertenece a A-030/A-080). No se declara
  `RATE_LIMIT_DISTRIBUTED_VERIFIED`.
- **DAST**: el harness sólo prepara plan con evidencia de Gate dentro de `sdd/_control/H08/` (frase, host
  y caducidad vigente). Hoy no existe: `DAST_BLOCKED_AUTHORIZATION` y `NOT_RUN`.
- **Escáner IaC**: análisis textual; expresiones dinámicas se reportan como `*_NO_VERIFICABLE`, no PASS.
- **WAF/H-01/readiness/RFC-004** siguen abiertos bajo sus especificaciones.
- `AWS_NOT_RUN`: ningún control se declara verificado en cloud.

## 4. Verificación local

`go test ./tests/security/...`, `go test ./...`, `go vet`, `gofmt`, `herramientas/tests` (incluye 7 del
escáner IaC), Terraform/FinOps/systemd/CI/CD, `terraform fmt`, Trivy, SDD integral y SHA-256.

## 5. Cierre

Merge requiere autorización humana ligada a PR y HEAD exactos (A-110/R-004). No se declara
`AWS_DEPLOYED`, `DAST_VERIFIED`, `PRODUCTION_READY` ni `SECURITY_VERIFIED` global. I-110 no se inicia sin
su Gate.
