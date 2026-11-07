# Gate H08-I — I-050 `redis-distribuido` (implementación local)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/GATE_H08I_I050.md` |
| Fecha | 2026-11-07 |
| Spec | `sdd/02-infraestructura/I-050-redis-distribuido/` (v0.1.0 DRAFT; implementación según I-000 + A-080) |
| Estado | **AUTORIZADO_LOCAL — IMPLEMENTADO_LOCAL (sin AWS)** |
| Precondición | I-040 integrada: PR #24 MERGED, `main` `51e01f6` |
| Rama | `feat/h08i-i050-redis` |
| Autorización | `APRUEBO_GATE_I050_LOCAL` (2026-11-07) |
| ADR de proveedor | `sdd/_control/H08/ADR_REDIS_PROVEEDOR.md` |

## 1. Alcance autorizado

Implementar **localmente** el módulo Terraform de Redis OSS, sus pruebas, la validación FinOps y el registro
documental, con commit, push y **un único PR**. Corregir en ese mismo PR sólo los defectos de I-050.

**No autorizado:** crear/modificar/eliminar recursos AWS, `terraform plan/apply/destroy`, cambio de plan,
gasto personal, cambiar la arquitectura Redis sin decisión normativa, sustituir Redis OSS, I-060–I-150,
endpoints de negocio, cambios a los 75 contratos, aprobar RFC-004 o H-01, ni fusionar sin autorización de PR+HEAD.

## 2. Decisión de servicio (Gate §5)

Existe elección ratificada: I-000 (D6, «SKU Redis» `CUMPLIDA_CON_EVIDENCIA`) fija **ElastiCache Redis OSS
`cache.t4g.micro`** con precio `VERIFIED_AWS`. Por tanto **se implementa esa alternativa** y se produce el ADR
comparativo exigido por I-050 §6 (`ADR_REDIS_PROVEEDOR.md`), que descarta Valkey, serverless, MemoryDB y la
instancia autogestionada con motivo y coste. La **matriz ADR obligatoria** (disponibilidad, seguridad, coste 45
días, plan Free, recuperación, compatibilidad SDD y riesgos por alternativa) está en su §6; el coste elegido es
**17.2800 USD + 0.1000 de transferencia inter-AZ**, ya presupuestado en I-000.

## 3. Decisiones de implementación (trazadas a la spec)

| Requisito | Implementación | Fuente |
|---|---|---|
| INF-050-01 | `aws_elasticache_replication_group` con `engine = "redis"`, `engine_version = "7.1"`, `node_type = "cache.t4g.micro"`; motor validado y no sustituible sin ADR | spec + I-000 |
| INF-050-02 | `num_cache_clusters = 1`, `automatic_failover_enabled = false`, `multi_az_enabled = false`; precondition que impide failover con un nodo y salida `alta_disponibilidad = false` | spec §2 |
| INF-050-03 | `transit_encryption_enabled = true`, `at_rest_encryption_enabled = true`, **usuario RBAC dedicado** (`aws_elasticache_user` + `aws_elasticache_user_group`), contraseña por variable `sensitive`, SG de I-010 (6379 sólo desde backend), subnet group con las 2 subredes privadas | spec §3 |
| INF-050-04 | `parameter_group` familia `redis7` con `maxmemory-policy = volatile-lru` (sólo se expulsan claves con TTL); prefijo de claves `nexum:*` coherente con A-080 | spec §4 |
| INF-050-05 | `snapshot_retention_limit = 0`: Redis es caché, DynamoDB es la verdad; el fail-close por operación vive en la app (A-080) | spec §5 |
| INF-050-06 | Prueba compartida real `NOT_RUN`; el diseño garantiza un endpoint único consumido por ambas instancias vía SG/subnet group compartidos | spec §6 |
| INF-050-07 | Sin réplicas ni backups: 17.2800 USD/45 d (1080 × 0.016), sin partidas nuevas | spec §7 + I-000 |

**Límites explícitos:** no se crea SG propio, no se abre 0.0.0.0/0, no se usa `auth_token` en texto, no se
declara alta disponibilidad y no se introduce KMS propio (el cifrado en reposo lo gestiona el servicio).

## 4. Riesgos

| ID | Estado | Detalle |
|---|---|---|
| R01 ElastiCache no habilitado en la cuenta | **MITIGADO parcialmente** | Motor y precios verificados en I-000; la creación real sigue `NOT_RUN`. Un `AccessDenied`/SCP sólo se verá en despliegue |
| R02 Autogestión con tercera instancia | **NO APLICABLE** | Se eligió el servicio administrado |
| R03 Redis sin TLS/ACL | **RESUELTO en diseño** | TLS + RBAC obligatorios por precondition; verificación real `NOT_RUN` |
| R04 Pérdida de caché o throttle genera inconsistencia | **APP (A-080)** | DynamoDB es la verdad; fail-close en auth/rate-limit y degradación en lecturas públicas |
| Punto único de fallo | **ACEPTADO_EN_DISEÑO** | Declarado en salidas y ADR; añadir réplicas exige recotización y Gate |
| Entrega del secreto RBAC | **BLOQUEO DE DESPLIEGUE** | RFC-004 sigue PROPUESTO; el endpoint `secretsmanager` de I-010 sólo existe en ventana |

## 5. Evidencia local

| Control | Resultado |
|---|---|
| `terraform fmt -check -recursive` | exit 0 |
| `terraform init -backend=false` (`TF_DATA_DIR` externo) | OK, lock 5.100.0 sin cambios |
| `terraform validate` | **Success** |
| `tests/infra/terraform` | 148 pruebas OK (17 de contrato I-050) |
| `tests/infra/finops` | 193 pruebas OK (8 nuevas de alineación I-050) |
| `validar_sdd_integral.py --check-git` | PASS |
| `validar_h06.py` | PASS (75 operaciones) |
| `go build/vet/test` | PASS (sin cambios de código Go) |

**CI aplicable:** Terraform local, CI Go y CI SDD con resultado real; **CI Terraform = `NOT_IMPLEMENTED`**
(dueño I-080).

**`terraform validate` acredita configuración válida; NO acredita despliegue.** `AWS_NOT_RUN`,
`INFRA_NOT_DEPLOYED`, coste incurrido 0.0000 USD.

## 6. Dependencias abiertas

- **RFC-004** (secretos del bootstrap): sin él no se puede entregar la contraseña RBAC en despliegue.
- **I-060**: política IAM acotada a `elasticache:Connect` sobre el grupo/subnet group.
- **I-010**: el endpoint `secretsmanager` sólo existe en ventana de mantenimiento.
- **I-080**: CI de Terraform.
- **H-01**, I-060–I-150 y H08-E: sin cambios.
