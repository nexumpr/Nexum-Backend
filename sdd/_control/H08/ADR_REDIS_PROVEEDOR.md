# ADR — Proveedor de Redis para Nexum-Backend

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/ADR_REDIS_PROVEEDOR.md` |
| Fecha | 2026-11-07 |
| Spec que lo exige | I-050 §6 (`ADR_REDIS_PROVEEDOR.md`) e INF-050-01 |
| Autoridad de la decisión | I-000 (D6 `APRUEBO_CIERRE_I000_CON_CONDICIONES`; «SKU Redis» = `CUMPLIDA_CON_EVIDENCIA`) + `infra/finops/ADR_COSTO_CERO_I000.md` |
| Estado | **DECISIÓN TRAZADA A I-000 — implementada en I-050** |
| Implementación | `infra/terraform/modules/redis` (`AWS_NOT_RUN`) |

## 1. Decisión

**ElastiCache Redis OSS**, node-based, **1 nodo `cache.t4g.micro`**, Redis 7.1, sin réplicas ni failover, TLS y
cifrado en reposo, usuario RBAC dedicado, subnet group privado en las dos subredes de I-010 y SG de I-010.

No se usa Valkey (el requisito del SDD es Redis), ni serverless (modelo de coste distinto y no verificado), ni
Redis autogestionado en una tercera instancia EC2. La decisión **no** se toma en I-050: se hereda de I-000.

## 2. Comparativa evaluada

| Alternativa | Disponibilidad en la cuenta | Coste 45 días | Veredicto |
|---|---|---|---|
| **ElastiCache Redis OSS `cache.t4g.micro`** | `redis7` hasta 7.1 verificado (`describe-cache-engine-versions`) y precio `VERIFIED_AWS` | **17.2800 USD** (1080 h × 0.016) | **ELEGIDA**: servicio administrado, parcheo y TLS/AUTH/RBAC sin operar servidores |
| ElastiCache **Valkey** | Disponible en la región | Tarifa distinta (~0.0128) | **DESCARTADA**: el SDD exige Redis; cambiarlo requiere ADR humano explícito |
| ElastiCache **Serverless** | Disponible | No modelado (ECPU) | **DESCARTADA**: el presupuesto I-000 no tiene línea para ECPU |
| Redis OSS **autogestionado** en 3.ª EC2 | Posible | +EC2 +EBS +parcheo y operación | **DESCARTADA**: añade una tercera instancia y superficie operativa no presupuestada; INF-050-01 sólo lo admite como fallback |
| **MemoryDB** | Disponible | Muy superior | **DESCARTADA**: durable, no es caché; coste fuera de presupuesto |

## 3. Trazabilidad de la ratificación

| Evidencia | Dónde |
|---|---|
| Topología con «Redis OSS» como componente obligatorio | `infra/finops/ADR_COSTO_CERO_I000.md`, escenario C |
| Precio `cache.t4g.micro` 0.016 USD/nodo-hora con `estado: VERIFIED_AWS` y `usagetype USE2-NodeUsage:cache.t4g.micro` | `infra/finops/precios_us-east-2.json` |
| «SKU Redis» entre las condiciones `CUMPLIDA_CON_EVIDENCIA` del cierre I-000 | `sdd/_control/H08/DECISION_CIERRE_I000.md` |
| Motor Redis disponible hasta 7.1 | `infra/finops/MATRIZ_VIABILIDAD_AWS_I000.md` |

**Discrepancia detectada y resuelta:** `sdd/02-infraestructura/I-050-redis-distribuido/spec.md` §8 aún dice
`PRICE_SECONDARY_NOT_SKU_PROVEN`, y `infra/finops/escenarios.py` conserva la condición «SKU exacto de
ElastiCache Redis OSS pendiente de revalidacion». Ambas afirmaciones son **anteriores** a la corrección de I-000
rev2/rev3, donde el SKU se verificó por `usagetype` (se había tomado por error la tarifa de Extended Support,
0.026 en vez de 0.016). **La autoridad vigente es el catálogo `VERIFIED_AWS`**, no el texto DRAFT de H05. No se
edita `spec.md` (política `edit` denegada y sin RFC) ni se altera I-000: se deja constancia aquí.

## 4. Consecuencias aceptadas

| Consecuencia | Detalle |
|---|---|
| **Punto único de fallo** | Un nodo; sin failover ni Multi-AZ. Redis es caché: DynamoDB es la fuente de verdad (INF-050-05). El SDK de la app debe aplicar fail-close en auth/rate-limit y degradación en lecturas públicas (A-080) |
| **Sin backups** | `snapshot_retention_limit = 0`; la pérdida de caché es recuperable desde DynamoDB |
| **Coste previsible** | 17.2800 USD/45 d; réplicas o Multi-AZ lo multiplican y exigen recotización y Gate |
| **Secreto de RBAC** | La contraseña del usuario dedicado llega por variable `sensitive`; **no** se versiona. Su entrega en despliegue depende de **RFC-004** (PROPUESTO) y del endpoint `secretsmanager` de I-010, que sólo existe en ventana de mantenimiento |
| **TLS obligatorio** | `transit_encryption_enabled = true`; los clientes Go deben usar `rediss://` con verificación de certificado |

## 5. Qué quedaría pendiente si el responsable no considera ratificada esta decisión

Nada se ha desplegado. Si se prefiere otra alternativa, el cambio se limita a
`infra/terraform/modules/redis/main.tf` (variable `motor`/topología) **antes** de cualquier `apply`, y requeriría
una nueva ADR con recotización. No hay recurso AWS creado que deshacer.
