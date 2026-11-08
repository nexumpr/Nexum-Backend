# I-090 — Evidencia de despliegue

| Campo | Valor |
|---|---|
| Spec | `sdd/02-infraestructura/I-090-cd-backend` (INF-090-06) |
| Estado | **NOT_RUN** — template sin ejecución real |
| AWS | `NOT_RUN`; ningún recurso creado o modificado |

## 1. Registro por release

| Campo | Valor |
|---|---|
| Release ID | *(pendiente)* `vX.Y.Z+<sha12>` |
| SHA completo | *(pendiente)* |
| Run de CI | *(pendiente)* |
| `sha256_binario` | *(pendiente)* |
| SBOM | `sbom-spdx-<sha>` del mismo run |
| Autorizador | *(pendiente)* — usuario que aprueba el entorno y emite la frase |
| Fecha/hora UTC | *(pendiente)* |
| Entorno | `aws-deploy` |
| Nodos `InService` antes/después | *(pendiente)* 2/2 esperado |
| Targets `healthy` antes/después | *(pendiente)* 2/2 esperado |
| Capacidad transitoria | `max_size` 3 durante el rollout y 2 al final (obligatorio) |
| Health HTTP | *(pendiente)* `GET /api/v1/health/live` → 200; readiness según `READINESS_REQUIRED` |
| Rollback ejecutado | Sí/No y versión previa del Launch Template |
| Coste observado | *(pendiente)* |
| Errores | *(pendiente)* |
| Estado | `NOT_RUN` / `DEPLOYED_VERIFIED` / `FAILED` |

## 2. Reglas de evidencia

- No se declara `DEPLOYED_VERIFIED` sin ejecución real, 2 nodos sanos y health HTTPS 200.
- Un `200` sin operación persistente verificable no acredita el endpoint afectado.
- La evidencia no incluye secretos, state, credenciales ni datos personales.
- El artefacto de evidencia se conserva 30 días con el SHA en el nombre.
- El registro de este archivo no sustituye la salida real de AWS/HTTP.

## 3. Estado actual de los bloqueos

| Bloqueo | Estado |
|---|---|
| Entorno `aws-deploy` con revisores | `PENDIENTE_APLICACION_HUMANA` (0 entornos configurados) |
| Rol OIDC de despliegue | `NOT_VERIFIED` (I-060 exige protección del entorno) |
| `CD_HABILITADO` | Variable no creada |
| `AWS_SCP_NOT_VERIFIED` | Abierto |
| RFC-004 (secretos de bootstrap) | PROPUESTO |
| H-01 (TLSv1 en el borde) | Abierto |
| RFC-006 (WAF) | Aprobada temporalmente, caduca 2026-12-02 |
| Redis sin HA / readiness 503 | Abiertos |

Ningún bloqueo se marca resuelto por la existencia del workflow.
