# Gate H08-E — Regularización, corrección paralela y cierre funcional de Oleadas 4 y 5

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/GATE_H08E_REGULARIZACION_OLEADAS_4_5.md` |
| Versión | 1.0.0 |
| Fecha | 2026-11-24 |
| Estado | **AUTHORIZED_DEVELOPMENT_ONLY — MERGE / RELEASE / DEPLOY PENDING** |
| Autoridad que lo motiva | Orden ejecutiva del Scrum Master del 2026-11-24 |
| Instrumento de gobierno | **RFC-023** (`APPROVED`) y RFC-019 (E-1/E-2) |
| Aprobador | Scrum Master de Nexum-Backend |
| Alcance | Desarrollo, pruebas locales, PR y CI. **No** merge, **no** release, **no** AWS mutante |

> Este documento registra autorizaciones de **desarrollo**. No autoriza por sí mismo merge, publicación, `terraform apply`, despliegue, rollback, rotación de credenciales ni creación de recursos de pago.

## 1. Baseline verificado (2026-11-24)

| Dato | Valor |
|---|---|
| `main` / `origin/main` | `dbff5f8661010b9b3a6285fa89e08a4cd79ca21d` (PR #142) |
| `origin/liz-oleada4` | `22d4aa9f3e27c7ce4fd2b9f7d04b09734ce2a96e` (15 commits; merge-base `dbff5f8`) |
| `origin/liz-oleada5` | `b57a549a7cef21233aaea11a042c17726d6f42c1` (1 commit sobre `22d4aa9`) |
| Producción | `v0.20.0+4bf6758d372`; binario ARM64 22 755 777 B; SHA-256 `8f681d4ecf547bb40a45008a78bc683fe44aec5b19cdcea9858c8e51459ce94c`; LT v19 |
| ASG | 2/2/2; instancias en `us-east-2a` y `us-east-2b`; 2/2 targets healthy |
| Bundle | v2 (17 variables); v1 conservado |
| SES | Producción activa: 50 000/día, 14/s, HEALTHY, supresión BOUNCE+COMPLAINT, us-east-2 |
| Rollback | `v0.19.2+3098f379d7ab` almacenado; **no listo** (faltan ARN IAM) |
| Cambios locales preexistentes | `opencode.json` (responsable), `go.mod` (promoción `sesv2` a directo, generada por pruebas), `.audit_tmp/probe.txt` (temporal). **No se sobrescriben ni se revierten en esta fase.** |

## 2. Autorizaciones expresas registradas

| # | Frase | Alcance |
|---|---|---|
| 1 | `APRUEBO_REGULARIZACION_OLEADAS_4_5` | Inicio de la fase de regularización |
| 2 | `APRUEBO_RFC_023_GOBIERNO_RELEASE_OLEADAS` | Gobierno de PR agrupados, concurrencia y release desde CI; sin bypass |
| 3 | `APRUEBO_CORRECCION_E052_PERIOD` | Normalización de `period` en E052 |
| 4 | `APRUEBO_DECISION_IDS_NOTIFICACION_E062` | UUID contractuales, deterministas; sin relajar el contrato |
| 5 | `APRUEBO_CORRECCION_REMINDERS_SK_E065_E067` | E064–E067, paginación y pruebas del adaptador DynamoDB |
| 6 | `APRUEBO_CONTRATO_HEALTH_E072_E073` | Cuerpo `Health`, `ErrorEnvelope`, preservando códigos y probes |
| 7 | `APRUEBO_DESARROLLO_SCHEDULER_DURABLE_OLEADA5` | Scheduler durable de recordatorios |
| 8 | `APRUEBO_TRANSVERSAL_E014_E020` | Purga/exportación de familias nuevas, sin operaciones destructivas |
| 9 | `APRUEBO_CORRECCION_ML_155` | Corrección de desviaciones y regeneración del modelo; **no** aprueba el modelo final |
| 10 | `APRUEBO_DESARROLLO_FCM_PRODUCTOR_OUTBOX_Y_SCHEDULER` | Productor, outbox y scheduler; cambios transversales indispensables |
| 11 | `APRUEBO_CORRECCION_DEVSECOPS_OLEADAS_4_5` | Staticcheck, Gosec, Gitleaks y controles CI |
| 12 | `APRUEBO_PREPARACION_IAC_IAM_FCM_Y_ROLLBACK` | Terraform, validaciones locales y propuesta de planes; **sin apply** |
| 13 | `APRUEBO_REVISION_ZIP_SECRETOS` | Inspección confidencial del objeto concreto, sin exponer contenido |
| 14 | `APRUEBO_PREPARACION_PR_Y_EJECUCION_CI_OLEADAS_4_5` | PR autorizados y CI |
| 15 | `APRUEBO_CORRECCIONES_TRANSVERSALES_CONFIRMADAS_54_ENDPOINTS` | Defectos inequívocos; sin decisiones financieras/legales/contractuales nuevas |

**No se ha inventado ninguna firma.** Las frases son las proporcionadas por el Scrum Master en la orden ejecutiva.

## 3. Unidades y propiedad de archivos

| Unidad | Worktree / rama | Propiedad | No puede tocar |
|---|---|---|---|
| U1 — Oleada 4 (E052, E062, rate limit, DevSecOps) | `.worktrees/fix-o4` / `fix/h08e-oleada4-regularizacion` | `internal/application/analytics/*`, `internal/application/notifications/*`, `internal/application/pushoutbox/*`, `internal/adapters/dynamo/*notification*`, `internal/adapters/dynamo/push_*`, `internal/infrastructure/push/*`, `internal/transport/http/*analytics*`, `internal/transport/http/*notification*`, `internal/transport/http/*push*`, `internal/transport/http/limitador_contrato.go`, tests asociados | `cmd/server/main.go`, `router.go`, OpenAPI, `memoria.md`, BASELINE, `infra/**` |
| U2 — ML | `.worktrees/fix-o4` (segunda unidad) | `ml/**`, `internal/application/analytics/recommendations.go`, tests ML | `cmd/server/main.go`, OpenAPI, `memoria.md` |
| U3 — Oleada 5 + scheduler | `.worktrees/fix-o5` / `fix/h08e-oleada5-regularizacion` | `internal/application/reminders/*`, `internal/adapters/dynamo/reminders_store.go`, `internal/transport/http/reminders_handler.go`, `internal/transport/http/health_handler.go`, `cmd/server/e064.go`, `cmd/server/registro_oleada5.go` (solo registro interno), tests asociados | `cmd/server/main.go`, `router.go`, OpenAPI, `memoria.md`, `infra/**` |
| U4 — Transversal E014/E020 | `.worktrees/fix-o5` (segunda unidad) | `internal/application/privacy/*`, `internal/application/export/*`, `internal/adapters/dynamo/*deletion*`, `internal/adapters/dynamo/*export*`, tests asociados | `cmd/server/main.go`, OpenAPI (solicita cambio al orquestador), `memoria.md` |
| U5 — Plataforma IAM/FCM/ZIP | `.worktrees/iac` / `feat/h08i-iam-egreso-propuesta` | `infra/terraform/**`, `sdd/_control/H08/PROPUESTA_EGRESO_FCM.md`, `sdd/_control/H08/REVISION_ZIP_SECRETOS.md`, tests IaC | Código Go, OpenAPI, `memoria.md` |
| U6 — Históricos 54 + gobierno | `.worktrees/fix-54` / `fix/h08e-regularizacion-54-gobierno` | `internal/application/identity/*` (E005/E011), `internal/transport/http/*password*`, `*email_verification*`, `*goal*` (E041), `internal/application/content/*` (E070), `internal/transport/http/httputil` compartido (coordinado), RFC-023, Gate, tests | `cmd/server/main.go`, `router.go`, OpenAPI (solicita cambio), `memoria.md` (orquestador), `infra/**` |
| Orquestador | Worktree principal + integración | `cmd/server/main.go`, `router.go`, OpenAPI JSON/YAML, `memoria.md`, BASELINE, workflows, integración y PRs | — |

**Regla:** ningún archivo compartido se modifica por dos unidades. Si U1/U3/U4/U6 necesita un cambio en `main.go`, `router.go`, OpenAPI, `memoria.md` o BASELINE, lo solicita al orquestador.

## 4. Controles obligatorios

1. `gofmt -l` sin salida.
2. `go build ./...`, `go vet ./...`, `go test ./... -count=1`.
3. `go test -race` en CI Linux (local `NOT_RUN` si no hay gcc).
4. `go mod verify`.
5. Pruebas de integración HTTP, concurrencia y contrato.
6. Gitleaks 8.30.1, Staticcheck v0.8.2, Gosec v2.30.0, Govulncheck v1.9.0, Semgrep, Trivy, SBOM.
7. H06, paridad OpenAPI JSON/YAML, SDD integral, Quality Gate y los 13 checks protegidos.
8. Sin `#nosec` indiscriminado; sin ampliación general de allowlists; sin reducción de umbrales.
9. Sin secretos, tokens, correos personales ni URLs presignadas en código, tests, informes o commits.

## 5. Exclusión expresa de merge, release y AWS

Este Gate **no** autoriza:

- merge de ningún PR;
- publicación de release;
- `terraform plan` con state remoto bloqueante, `apply`, `destroy`, `import`, `state`;
- cambios IAM, SES, CloudFront, DynamoDB, Redis, ASG, LT o S3;
- rollback;
- despliegue o `instance refresh`;
- rotación de credenciales o borrado del ZIP;
- pruebas productivas con cuentas reales;
- aprobación legal de E015;
- decisión financiera de E031;
- Gate final de ML;
- FinOps de egreso FCM.

Cada una requiere su Gate específico con frase, alcance y SHA cuando corresponda.

## 6. Excepción histórica

No se renueva automáticamente ninguna excepción vencida. En particular, RFC-006 (WAF temporal) conserva su fecha y condiciones; si caduca, el job de IaC debe fallar cerrado. Cualquier nueva excepción de seguridad requiere RFC y aprobación propias.

## 7. Criterio de salida de esta fase

La fase termina en:

- `REGULARIZACION_OLEADAS_4_5_LISTA_PARA_GATES` si existen PR con CI verde, HEAD40, pruebas y evidencias, y solo faltan los Gates humanos de merge/publicación/despliegue;
- `REGULARIZACION_PARCIAL_BLOQUEOS_IDENTIFICADOS` si persisten bloqueos externos (ZIP, FinOps, ML, decisiones de producto) con el máximo avance posible.

En ambos casos, **detenerse antes del merge o despliegue**.

## 8. Aprobación

`APRUEBO_REGULARIZACION_OLEADAS_4_5` y `APRUEBO_RFC_023_GOBIERNO_RELEASE_OLEADAS` — Scrum Master, 2026-11-24.
