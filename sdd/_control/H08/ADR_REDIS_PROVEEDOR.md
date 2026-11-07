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

## 6. Matriz ADR obligatoria (comparativa completa)

Precios tomados **solo** del catálogo `infra/finops/precios_us-east-2.json` (`estado_global:
VERIFIED_AWS_PARCIAL`); ninguna cifra se inventa y ninguna oferta de créditos *pendientes* se cuenta como saldo.

| ALTERNATIVA | DISPONIBILIDAD | SEGURIDAD | COSTO 45 DÍAS | FREE PLAN | RECUPERACIÓN | COMPATIBILIDAD SDD | RIESGOS |
|---|---|---|---|---|---|---|---|
| **A. ElastiCache Redis OSS, 1 nodo `cache.t4g.micro`** *(elegida)* | 1 AZ, **sin réplica ni Multi-AZ** → punto único de fallo declarado; el servicio parchea y mantiene el motor | TLS + cifrado en reposo + RBAC dedicado + SG privado de I-010; sin puerto público | **17.2800 USD** (1080 h × 0.016) + **0.1000** de transferencia inter-AZ presupuestada = **17.3800** | Sin asignación gratuita permanente de ElastiCache; cubierto por los 120 USD del plan Free | Sin backups (`retention 0`); RTO = repoblar caché desde DynamoDB; sin pérdida de dato financiero | **Total**: Redis OSS real, compartido por los 2 nodos, TTL/rate-limit (A-080) | SPOF; coste por hora sin tráfico; token RBAC depende de RFC-004 |
| **B. Redis OSS autogestionado en EC2 dedicada** | 1 instancia → SPOF, salvo replicación manual; **no puede vivir en el ASG** (se reemplaza y se pierde servicio) | Parcheo, TLS, AUTH y hardening a cargo del equipo; misma red privada posible | **≈10.0320 USD** (0.0084 × 1080 = 9.0720 + EBS gp3 8 GB × 0.08 × 1.5 = 0.9600), **sin** contar snapshots ni tiempo de operación | La franquicia mensual de `t4g.micro` ya la consumen las 2 instancias de I-020 (1440 h/mes > 750 h); la 3.ª instancia se factura | RDB/AOF + snapshots EBS manuales; recuperación y verificación manuales | Técnicamente posible, pero exige una **tercera instancia no autorizada ni presupuestada** y operación continua | Deriva de presupuesto, fallos por parcheo/versión, pérdida de caché en reemplazos, carga operativa |
| **C. ElastiCache Valkey** | Igual que A | Igual que A | Tarifa distinta; **0.0128 USD/nodo-hora citada en I-050 §8 es fuente secundaria `NOT_VERIFIED`**, no usada aquí | Igual que A (sin franquicia) | Igual que A | **No cumple**: el SDD exige Redis OSS; cambiarlo requiere ADR humano | Cambio normativo sin ratificación |
| **D. ElastiCache Serverless** | Gestión de capacidad automática | TLS/RBAC disponibles | **No modelado**: se factura por ECPU/GB, métrica que I-000 no presupuestó | Sin franquicia; cubierto por créditos pero con predicción peor | Sin backups gestionados en el modelo usado | Funcional, pero incompatible con presupuesto fijo de 45 días | Sobrecoste impredecible; requiere recotización |
| **E. MemoryDB** | Durable, réplica gestionada | TLS + ACL | **No está en el catálogo** (`NOT_VERIFIED`); estructuralmente superior a ElastiCache | Sin franquicia | Durabilidad real (no es caché) | No aporta nada al rol de caché del SDD | Coste fuera de presupuesto; propósito distinto |

**Supuestos explícitos:** 45 días = 1080 h; 1 nodo; sin réplicas; sin snapshots de Redis; transferencia inter-AZ
de 10 GB ya incluida en la partida `REDIS_INTERAZ`; precios del catálogo con fecha 2026-11-06; **no** se asume
ninguna de las cuatro actividades de créditos adicionales (20 USD cada una) porque **no están acreditadas**.

**Descartes justificados:** C por requisito normativo; D y E por coste no modelado; B por introducir una tercera
instancia no autorizada, fuera-del-presupuesto y con carga operativa, además de no poder residir en el ASG sin
perder el servicio.

## 7. Endurecimiento RBAC del servicio elegido

ElastiCache añade un usuario `default` (`on ~* +@all`) a todos los grupos y no permite eliminarlo; la única
forma soportada de neutralizarlo es **crear otro usuario con `user_name = "default"`** y añadirlo al grupo. El
módulo lo hace con `access_string = "off ~* -@all"` y `no-password-required`, y mantiene el usuario de
aplicación con contraseña y ACL restringida al espacio `nexum:`. Detalle y evidencia oficial en
`GATE_H08I_I050.md` §7. Esto no altera la alternativa elegida, su coste ni su topología.
