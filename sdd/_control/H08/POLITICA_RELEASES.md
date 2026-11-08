# I-090 — Política de releases

| Campo | Valor |
|---|---|
| Spec | `sdd/02-infraestructura/I-090-cd-backend` (INF-090-01, INF-090-02) |
| Estado | `IMPLEMENTADO_LOCAL`; ningún despliegue ejecutado (`AWS_NOT_RUN`) |
| Workflow | `.github/workflows/cd-backend.yml` (solo `workflow_dispatch`) |
| Dependencias | I-020 (ASG), I-030 (ALB/CloudFront), I-060 (OIDC), I-080 (CI/artefactos), I-100 |

## 1. Matriz de versiones

| Elemento | Regla |
|---|---|
| Versión del proyecto | SemVer `vX.Y.Z`; primer release local propuesto `v0.1.0` |
| Versión de API | `/api/v1` fija; un cambio incompatible exige `/api/v2` y RFC |
| Release ID | `vX.Y.Z+<sha12>`, inmutable y ligado al commit |
| Artefacto | `binario-<sha40>` publicado por Go Backend CI desde ese SHA |
| Integridad | `sha256_binario` (64 hex) verificado dos veces: preflight y antes de desplegar |
| SBOM | `sbom-spdx-<sha40>` del **mismo** run de CI |
| Commit | SHA completo de 40 hex; el workflow hace checkout de ese ref |

No se admite construir el binario dentro del despliegue: se promueve el artefacto producido por
CI desde el SHA autorizado.

## 2. Condiciones de aprobación

Un despliegue sólo puede iniciarse con **todas** estas condiciones:

1. `workflow_dispatch` sobre `main`; nunca `push`, `pull_request` ni tag automático.
2. Variable de repositorio `CD_HABILITADO=true` (por defecto ausente ⇒ el job no corre).
3. Entorno `aws-deploy` con revisores obligatorios (ver §4). El workflow exige además
   `ENVIRONMENT_PROTECTION_CONFIRMED=aws-deploy` y **lee la API del entorno**: si no puede verificarlo o no
   hay `required_reviewers`, falla cerrado antes de pedir credenciales.
4. Frase exacta `AUTORIZO_DESPLIEGUE_<sha40>` introducida por el responsable.
5. `confirmar=SI`.
6. `release_id` coherente con el SHA (`vX.Y.Z+<sha12>`) y `lt_version_previa` numérica.
7. Run de CI **completado y verde**, del repositorio y del SHA exactos (verificado por API).
8. Artefacto y SBOM del run de CI indicado, con SHA-256 coincidente.
9. Dos nodos `InService` y dos targets `healthy` **antes** de mutar.

El workflow ejecuta `herramientas/verificar_autorizacion_despliegue.py`,
`verificar_run_ci.py` y `verificar_entorno_protegido.py` en el preflight; cualquier fallo bloquea antes
de `configure-aws-credentials`. La aprobación del entorno es humana e independiente del merge.

## 2.1 Capacidad transitoria del rollout

El ASG de I-020 fija `min = desired = max = 2`. Con `MinHealthyPercentage=100` no es posible reemplazar
una instancia conservando dos targets sanos si `max` sigue en 2. Por eso el CD:

1. verifica que `max_size` es exactamente 2;
2. eleva `max_size` a **3** antes del `instance refresh`;
3. ejecuta el refresh con `MinHealthyPercentage=100`;
4. verifica 2 `InService` y 2 `healthy`;
5. restaura `max_size` a **2** tras el rollout o en el rollback.

Coste incremental: una instancia `t4g.micro` transitoria durante minutos. Referencia de I-020:
~`0.0084 USD/h`; no medido (`NOT_VERIFIED`). Si el presupuesto no cubriera esa capacidad transitoria,
el despliegue queda **BLOCKED** y se solicita decisión humana; no se reduce el mínimo de dos nodos.

## 2.2 Verificación HTTP

- `GET /api/v1/health/live` → **200 obligatorio**.
- `GET /api/v1/health/ready` con `READINESS_REQUIRED=true` → **200 obligatorio**. Hoy readiness devuelve
  503 hasta registrar dependencias reales (I-040/I-050): por defecto la variable está ausente y el
  workflow **registra** el estado sin declarar readiness cumplida.
- HTTPS con `curl --proto '=https' --fail`; no se acepta un 200 sin verificación de TLS.

## 3. Separación de responsabilidades

- **Aplicación:** este workflow (ASG instance refresh + verificación HTTP + rollback).
- **Terraform:** `plan/apply` pertenecen al Gate de infraestructura (I-070/I-080) y **no** se ejecutan
  aquí; el workflow no contiene `terraform`.
- **Endpoints:** I-090 no implementa ni modifica E001–E075.
- **Mobile:** fuera de alcance; no se integra ni aprueba Android.

## 4. Estado real de capacidades (verificado, no asumido)

| Capacidad | Estado | Evidencia |
|---|---|---|
| Workflow declarado | **IMPLEMENTADO_LOCAL** | `.github/workflows/cd-backend.yml` |
| Entorno `aws-deploy` con revisores | **PENDIENTE_APLICACION_HUMANA** | `gh api repos/.../environments` → `total_count: 0` |
| Rol OIDC de despliegue | **NOT_VERIFIED** | I-060 exige `apply_environment_protected=true`; no creado por defecto |
| Proveedor OIDC | `AWS_SCP_NOT_VERIFIED` | I-060 |
| Variable `CD_HABILITADO` | **PENDIENTE** (debe crearla el responsable) | no existe hasta configurarla |
| Variable `ENVIRONMENT_PROTECTION_CONFIRMED` | **PENDIENTE** | sin ella el preflight falla |
| Variable `READINESS_REQUIRED` | **AUSENTE** (readiness 503 por diseño hasta I-040/I-050) | no se declara readiness |
| Ejecución real | **NOT_RUN** | `AWS_NOT_RUN` |

Mientras no existan el entorno protegido, el rol OIDC y la variable, el workflow no puede desplegar:
el preflight falla cerrado.

## 5. Prohibiciones

- `push`→deploy, `pull_request`→deploy, deploy desde forks o desde artefactos no verificados.
- Credenciales estáticas (`AWS_ACCESS_KEY_ID`, secretos) o `contents: write`.
- `terraform apply/destroy`, Docker, cambios de plan o de visibilidad.
- Despliegue sin dos nodos sanos o sin frase de autorización ligada al SHA.
- Publicar artefactos con datos personales, state o secretos.

## 6. Evidencia de cada release

`EVIDENCIA_DEPLOY.md` registra release ID, SHA, run de CI, digest, autorizador, fecha, estado de los
dos targets, resultado HTTP, coste observado y errores. Un despliegue sin evidencia no se declara
`DEPLOYED_VERIFIED`.
