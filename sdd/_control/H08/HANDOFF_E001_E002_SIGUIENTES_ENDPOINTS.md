# Handoff — estado estable tras publicar E001 y E002 V6.0.2

| Campo | Valor |
|---|---|
| Fecha UTC | 2026-11-12 |
| `main` de referencia | `909cfc9055630fab3b7654fd415c2fab3b4640a1` (más el PR documental de cierre) |
| Base URL productiva | `https://d2lpmqy5w6x9es.cloudfront.net/api/v1/` |
| Cuenta / región AWS | `713797602090` / `us-east-2` |
| Release en producción | `v0.9.0+7d1ebf3d5357` (linux/arm64) |
| Endpoints publicados | `POST /auth/register` (E001) y `POST /auth/login` (E002) |

## Qué está estable hoy

- **Dos nodos** en el ASG `nexum-h08i-asg-backend` (Min 2 / Desired 2 / Max 2), ambos en la
  versión **3** del Launch Template `nexum-h08i-lt-20261111034054695000000001`, `Healthy` y
  con dos targets sanos en el target group del ALB.
- **DynamoDB** `NEXUM_CORE_H08I`: single-table con `PK`/`SK`, TTL físico en `expiresAt`
  (Number), GSI1 disponible. Sin `Scan` en ningún flujo.
- **Redis OSS** compartido con TLS y RBAC; cuotas de login y registro con identificadores
  seudonimizados.
- **Secretos**: bundle privado RFC-004 en
  `s3://nexum-h08i-secretos-713797602090/nexum/secrets/v1/secrets.json`, consumido por el
  bootstrap en el arranque de cada instancia.

## Dónde está cada cosa

| Necesidad | Ruta |
|---|---|
| Contrato HTTP vigente | `sdd/03-endpoints/openapi/nexum-api-v1.yaml` (espejo `.json`; ambos deben quedar sincronizados) |
| Inventario de endpoints | `sdd/03-endpoints/MATRIZ_OPERACIONES_H06.csv` (75 operaciones) |
| Spec de un endpoint | `sdd/03-endpoints/E0NN-<slug>/{spec,plan,tasks,checklist}.md` |
| Arquitectura Go | `internal/{domain,application,ports,adapters,infrastructure,transport,system}`, `cmd/server` |
| Cambios de contrato | `sdd/_control/H08/RFC-0NN-*.md` + `EXCEPCIONES_COMPATIBILIDAD_API.json` |
| Gates y estado | `sdd/_control/H07/ESTADO_Y_GATES.md`, `sdd/_control/H08/GATES_PR.md` |
| Política de releases | `sdd/_control/H08/POLITICA_RELEASES.md`, `RFC-010-PROMOCION_SIN_OIDC.md` |
| Integridad documental | `sdd/_control/H07/BASELINE_SHA256.json` |

## Cómo trabajar el siguiente endpoint

1. Rama desde `main` (`feat/h08e-<slug>`), **una especificación por PR**.
2. Leer su `spec/plan/tasks/checklist` y el bloque correspondiente del OpenAPI; si el
   contrato debe cambiar, **RFC + Gate** antes de tocar `spec.md`/`plan.md` (la política de
   `edit` los protege).
3. Implementar en las capas existentes: dominio puro, aplicación con puertos, adaptadores
   DynamoDB/Redis, handler en `internal/transport/http` y composición en `cmd/server`.
4. Pruebas: unitarias de dominio/aplicación, contrato OpenAPI, adaptador y handler; sin
   mocks en la composición productiva.
5. Validaciones locales: `gofmt`, `go build ./...`, `go vet ./...`, `go test ./... -count=1`,
   `staticcheck`, `govulncheck`, `gosec`, `gitleaks`, Semgrep y los validadores SDD/OpenAPI.
6. Commit + push + **un** PR; corregir los fallos de CI en ese mismo PR hasta verde.
7. Merge solo con autorización humana ligada a PR y SHA40 (`APRUEBO_MERGE_PR_<N>_SHA_<SHA>`).
8. Despliegue **solo** con Gates AWS separados (S3, Terraform/Launch Template, rollout ASG).

## Reglas que no cambian

- `constitution.md` y `AGENTS.md` mandan; el SDD aprobado y los Gates por encima de cualquier
  agente.
- No romper E001 ni E002, ni cambiar `BASE_URL`, ni el prefijo `/api/v1/`.
- Reutilizar la infraestructura existente; no crear tablas, buckets, ALB, CloudFront, VPC ni
  servicios adicionales sin RFC y Gate.
- Dinero: plan Free, sin gasto personal, sin pasar a Paid.
- Sin secretos en Git, logs ni documentos; sin `Docker`/`WSL`; sin `force push`; sin push
  directo a `main`.
- Compatibilidad con Android: los contratos se publican sin depender del equipo móvil, pero
  no se les anuncia un endpoint hasta que esté verificado por HTTP real.

## Deuda diferida (no bloquea)

Drift Terraform en `module.redis.aws_elasticache_user.default_deshabilitado`
(`no-password → no-password-required`), verificación empírica completa de la cadena de
proxies, suite del adaptador DynamoDB con dobles del SDK y endurecimiento del manejo local de
credenciales temporales.
