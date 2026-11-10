# Gate H08-I — Fase base de infraestructura (`desplegar_aplicacion = false`)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/GATE_H08I_FASE_BASE.md` |
| Fecha | 2026-11-10 |
| Raíz | `infra/terraform/envs/h08i` |
| Estado | **PREPARADO — BLOQUEADO POR 3 ARN (D-BASE-01)** |
| Prerrequisito cumplido | bootstrap aplicado y **state migrado** a S3 (`GATE_H08I_STATE_MIGRATION.md` §8) |
| Alcance | Red, DynamoDB, Redis, artefactos, secretos, IAM y observabilidad. **Sin** EC2, sin ALB, sin CloudFront |
| Mutación AWS | **Ninguna** mientras no exista plan con SHA y frase humana |

## 1. Qué se aplicaría (solo tras Gate)

| Módulo | Recursos | Notas |
|---|---|---|
| `red` | VPC, 2 subredes privadas + 2 públicas de borde interno, IGW adjunto, SGs, NACL, gateway endpoints S3/DynamoDB, interface endpoints `logs` permanente y `secretsmanager`/`ssm`/`ssmmessages` por ventana | Sin NAT, sin IPv4 públicas en el backend |
| `dynamodb` | Tabla única `NEXUM_CORE_h08i`, `PAY_PER_REQUEST`, `GSI1`, TTL `expiresAt`, SSE, PITR | GSI2 opt-in desactivado |
| `redis` | ElastiCache Redis OSS `cache.t4g.micro`, 1 nodo, TLS, RBAC con `default` deshabilitado | Sin réplicas ni backups |
| `artefactos` | Bucket S3 privado de releases + políticas por principal | Requiere `artefactos_publicador_arn` y `secretos_gestor_iac_arn` |
| `secretos` | Bucket S3 privado del bundle (RFC-004 opción B) | Requiere `secretos_publicador_arn` |
| `iam` | Rol/instance profile runtime con DynamoDB exacto | Sin OIDC (SCP) |
| `observabilidad` | Log group 7 d, 4 metric filters, 8 alarmas, dashboard, presupuesto | `ALERT_DELIVERY = BLOCKED_RFC007_SNS_KMS` |

`module.computo` y `module.balanceo` quedan con `count = 0` por `desplegar_aplicacion = false`.

## 2. Bloqueo real D-BASE-01 — tres ARN inexistentes

`envs/h08i` exige tres identidades **reales** (vacías hacen fallar el plan cerrado):

| Variable | Consumidor | Uso |
|---|---|---|
| `artefactos_publicador_arn` | `module.artefactos` | publica binarios/SBOM |
| `secretos_publicador_arn` | `module.secretos` | publica el bundle de secretos |
| `secretos_gestor_iac_arn` | `module.artefactos` | gestor IaC sobre el bucket de secretos |

Ninguna existe hoy: la cuenta no tiene roles de publicación y **OIDC está denegado por SCP**
(`RFC-010`), por lo que no se pueden crear roles federados de CI. Opciones evaluadas:

| Opción | Veredicto |
|---|---|
| ARN inventado / cadena vacía | **Prohibido**: fail-closed y riesgo de suplantación de nombre |
| Rol humano `AccountFullAccessRole` como publicador y gestor | Funciona, pero viola mínimo privilegio y mezcla identidades |
| Crear roles dedicados con `sts:AssumeRole` **solo** a la cuenta y políticas acotadas (sin OIDC) | **Recomendada**: mínimo privilegio, sin claves permanentes, ARN determinista |

**Decisión requerida:** autorizar la unidad mínima `RFC-011-IDENTIDADES_PUBLICACION` que cree, en
esta misma raíz y **antes** de los módulos que los consumen:

- `nexum-h08i-artefactos-publicador` (PutObject solo en `releases/*` del bucket de artefactos)
- `nexum-h08i-secretos-publicador` (PutObject solo en `nexum/secrets/*`)
- `nexum-h08i-secretos-gestor-iac` (GetObject/PutObject/ListBucket acotado al bucket de secretos,
  distinto de runtime y de publicador)

Los ARN se referencian como `aws_iam_role.x.arn` (valor conocido tras apply), de modo que no se
hardcodea ningún ARN ni se inventa ninguna identidad.

## 3. Permisos necesarios (hoy DENEGADOS)

Por `D-PERM-01` (las reglas de cadena exacta no se aplican), se usarán patrones **con comodín**,
siempre con `ASK` para que cada ejecución requiera aprobación humana:

```json
{"action": "shell", "resource": "terraform init -backend=true*", "effect": "ask"}
{"action": "tool.use", "resource": "shell:terraform init -backend=true*", "effect": "allow"}
{"action": "shell", "resource": "terraform plan*", "effect": "ask"}
{"action": "tool.use", "resource": "shell:terraform plan*", "effect": "allow"}
```

`terraform apply*` **permanece DENY** hasta que exista el SHA del plan revisado.

## 4. Procedimiento

1. Añadir `envs/h08i/backend.tf` con `key = "envs/h08i/terraform.tfstate"` (clave distinta; **no**
   hay state previo que migrar) y `use_lockfile = true`.
2. `terraform init -backend=true` (con la excepción de §3).
3. `terraform plan -input=false -out=fase-base.tfplan` y verificación de inventario.
4. Calcular SHA-256 y presentar: inventario, `N to add`, `0 to destroy`, y coste FinOps.
5. Gate de apply con el SHA exacto → aplicar → verificar recursos reales.
6. Retirar permisos temporales y registrar evidencia.

## 5. Verificación posterior exigible

- Tabla DynamoDB con `PAY_PER_REQUEST`, GSI1 y TTL.
- Redis con TLS y RBAC (`default` deshabilitado).
- Buckets privados con Block Public Access, versionado donde aplique y política TLS.
- Rol runtime con DynamoDB exacto y sin `*`.
- Endpoints: `logs` activo; los demás sólo en ventana.
- **0** instancias EC2, **0** ALB, **0** distribución CloudFront en esta fase.

## 6. Coste

Presupuesto vigente de I-000: **102.7830 USD** brutos (113.0613 con margen del 10 % sobre
120.0000 verificados). La fase base ya está contenida en ese presupuesto; no se autoriza ningún
recurso fuera del catálogo `VERIFIED_AWS`. Coste real observado: `NOT_VERIFIED`
(Cost Explorer no autorizado). No se declara gasto cero.

## 7. Autorización requerida (dos pasos)

Paso 1 — habilitar plan y resolver identidades:

```
APRUEBO_GATE_FASE_BASE_PLAN
APRUEBO_RFC011_IDENTIDADES_PUBLICACION
```

Paso 2 — aplicar el plan exacto (sólo después de revisar SHA, inventario y coste):

```
APRUEBO_APPLY_FASE_BASE_SHA256_<SHA256_64_HEX>
```

Este documento no se autoaprueba y no autoriza ninguna mutación por sí solo.
