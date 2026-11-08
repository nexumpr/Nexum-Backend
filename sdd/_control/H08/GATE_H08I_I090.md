# Gate H08-I — I-090 `cd-backend` (implementación local)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/GATE_H08I_I090.md` |
| Fecha | 2026-11-08 |
| Spec | `sdd/02-infraestructura/I-090-cd-backend/` (v0.1.0 DRAFT; spec/plan intactos) |
| Estado | **APRUEBO_GATE_I090_LOCAL REGISTRADO — IMPLEMENTADO_LOCAL (sin AWS)** |
| Main base | `4262972e5d423a01ba8be89642fa423e6a764003` (PR #28, I-080) |
| Rama | `feat/h08i-i090-cd-backend` |
| Autorización | `APRUEBO_GATE_I090_LOCAL` (mensaje humano independiente, 2026-11-08) |

## 1. Alcance

Implementación local del CD de Backend: workflow manual con autorización ligada al SHA, entorno
protegido, promoción del artefacto verificado de CI, despliegue ASG con dos nodos sanos, verificación
HTTP y rollback documentado. Se añade al CI de I-080 la publicación del binario `binario-<sha>` que
el CD promueve (cambio habilitador, sin alterar checks ni umbrales).

**No autoriza** ejecutar el workflow, desplegar, crear el entorno/rol OIDC, `terraform apply/destroy`,
cambiar de plan, gastar ni iniciar I-100.

## 2. Implementado

| Requisito | Evidencia | Estado |
|---|---|---|
| INF-090-01 Matriz de versiones | `POLITICA_RELEASES.md`: SemVer, `/api/v1`, release `vX.Y.Z+<sha12>`, digest y SBOM del mismo run | IMPLEMENTADO_LOCAL |
| INF-090-02 Aprobación | `cd-backend.yml` sólo `workflow_dispatch` sobre `main`, entorno `aws-deploy`, `CD_HABILITADO=true`, `ENVIRONMENT_PROTECTION_CONFIRMED=aws-deploy` con lectura de la API del entorno, frase `AUTORIZO_DESPLIEGUE_<sha>`, `confirmar=SI`, run de CI verde del mismo SHA y sin `main`→deploy | IMPLEMENTADO_LOCAL |
| INF-090-03 Despliegue seguro | `max_size` 2→**3** transitorio, instance refresh `MinHealthyPercentage=100`, aborta sin 2 `InService` y 2 `healthy` antes/después, restaura `max_size=2` y rollback a LT previa | IMPLEMENTADO_LOCAL |
| INF-090-04 Pruebas HTTP | `GET https://<api>/api/v1/health/live` = 200; readiness 200 sólo si `READINESS_REQUIRED=true` (hoy 503 por diseño); sin declarar readiness cumplida por defecto | IMPLEMENTADO_LOCAL |
| INF-090-05 Fallos y rollback | `PLAN_ROLLBACK.md`: rollback app por LT previa; rollback de esquema independiente y compatible hacia atrás | DOCUMENTADO |
| INF-090-06 Cierre | `EVIDENCIA_DEPLOY.md` con release, SHA, run, digest, autorizador, 2 targets, coste y errores; estado `NOT_RUN` | DOCUMENTADO |

## 3. Límites y bloqueos

- **Entorno `aws-deploy`**: `gh api repos/nexumpr/Nexum-Backend/environments` → `total_count: 0`.
  No existe protección configurada; `PENDIENTE_APLICACION_HUMANA`. No se crea por API.
- **Rol OIDC de despliegue**: I-060 sólo lo crea con `apply_environment_protected=true`; no existe y
  `AWS_SCP_NOT_VERIFIED`. El workflow falla cerrado sin rol.
- **Variable `CD_HABILITADO`**: por defecto ausente ⇒ el job no se ejecuta. Igual para
  `ENVIRONMENT_PROTECTION_CONFIRMED`; sin ella el preflight falla antes de OIDC.
- **Capacidad transitoria**: el ASG no permite 2 sanos con `max=2` durante el refresh; el CD eleva
  temporalmente `max` a 3 y lo restaura a 2. Coste incremental no medido (`NOT_VERIFIED`, referencia
  ~0.0084 USD/h); si el presupuesto no lo cubre, el despliegue queda `BLOCKED`.
- **Readiness**: `/api/v1/health/ready` devuelve 503 hasta registrar dependencias reales; no se declara
  readiness cumplida mientras `READINESS_REQUIRED` esté ausente.
- **Ejecución real**: `NOT_RUN`. El workflow no se ha lanzado; no hay despliegue ni coste.
- **Rollback**: documentado, no ensayado (`AWS_NOT_RUN`).
- Siguen abiertos RFC-004, H-01, RFC-006 (WAF, caduca 2026-12-02), Redis sin HA y readiness 503.

## 4. Verificación local

`tests/infra/cd` (contrato del workflow y de la autorización), `herramientas/tests`, Terraform/FinOps/
systemd, Go `build/vet/test`, cobertura de dominio ≥ 80 %, SDD integral y SHA-256. `terraform fmt`,
`init -backend=false` y `validate` sin backend remoto.

## 5. Cierre

Merge requiere autorización humana ligada a PR y HEAD exactos (A-110/R-004). No se declara
`AWS_DEPLOYED`, `DEPLOYED_VERIFIED`, `CD_VERIFIED` ni `PRODUCTION_READY`. I-100 no se inicia sin su
Gate.
