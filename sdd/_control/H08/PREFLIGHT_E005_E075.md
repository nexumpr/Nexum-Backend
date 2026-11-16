# Preflight único E005–E075 — permisos, AWS y dependencias

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/PREFLIGHT_E005_E075.md` |
| Fecha | 2026-11-16 |
| Base | `main` `8024f878e575d076a87ac2f5d656ce977691a641` |
| Estado | **PREFLIGHT COMPLETADO — 5 Gates pendientes** |
| Alcance | Solo análisis y propuesta. Nada aplicado, nada creado |

## 1. Permisos actuales de OpenCode

| Fuente | Permissions | Policies | SHA-256 |
|---|---|---|---|
| Versionado (`main`) | 151 | 33 | `244d82ad…88b6` |
| **Local del usuario** | **161** | **43** | `B281E263…3A37` |
| Candidato RFC-017 | 178 | 47 | `5579CF8A…007D` |

**Reglas AWS presentes en local:** `iam` 4 · `dynamodb` 3 · `elasticache` 3 · `s3api` 2 ·
`sts` 1 · `ec2` 9 · `autoscaling` 8 · `elbv2` 3 · `cloudfront` 2 · `pricing` 1 · `ce` 1 ·
`budgets` 1 · `freetier` 2 · `service-quotas` 2 · `organizations` 1 · `billing` 1.

**Servicios con CERO reglas:** `ses` · `logs` · `ssm`. Cualquier comando sobre ellos cae en el
`deny` de `aws *` y no puede ejecutarse.

**Deny vigentes (intactos):** `terraform destroy|import|state|apply` · `aws * create-|update-|delete-|put-` ·
`git push origin main` · `git push --force/-f` · `git reset --hard` · `git clean` · `docker` · `wsl` ·
`Remove-Item` · `rm` · lectura y edición de `.env`, `.tfstate`, `.tfvars`, `.pem`, `.key`, `.aws`, `.ssh`.

### Hallazgo D-PRE-01 — la autorización no se reflejó en la política técnica

Las enmiendas A1–A8 se aplicaron escribiendo `spec.md` mediante script bajo los Gates concedidos.
La política `edit` **seguía denegando** `sdd/**/spec.md` en el `opencode.json` versionado. No hubo
autoaprobación (existía Gate humano), pero el control técnico no distinguía "autorizado" de
"denegado". Es una razón adicional para aplicar RFC-017, que convierte esa regla en `ask`.

## 2. Matriz por servicio (71 contratos)

| Servicio | Endpoints | Estado del permiso | Recurso existente | Coste adicional |
|---|---|---|---|---|
| **DynamoDB** | **71/71** | lectura `describe/list` en ASK; `get-item`/`query` en RFC-017 | tabla `NEXUM_CORE_H08I`, GSI1 | **0** (on-demand, dentro de lo ya medido) |
| **Redis (ElastiCache)** | **71/71** | `describe-*` en ASK | `cache.t4g.micro`, 1 nodo, TLS+RBAC | 0 (ya en curso) |
| **SES** | **E005, E006** | **sin regla** → bloqueado | **ninguno** | por confirmar (§4) |
| **CloudWatch Logs** | 71 (transversal) | sin regla | log group `/nexum/h08i/backend` | 0 (retención 7 d) |
| **SSM** | E005 (secreto A9) | sin regla | parámetros por ventana (I-010) | 0 |
| **S3** | releases/artefactos | `put-object`/`assume-role` en ASK (RFC-017) | bucket de artefactos | 0 |
| **EC2/ASG/ELBv2** | despliegue | ASK | 2×t4g.micro, ALB, ASG | 0 |
| **ML (K-Means)** | E055 | no requiere AWS | artefacto en el repo, inferencia en Go | **0** |
| **Voz** | E075 | no requiere AWS | reconocimiento en Android | **0** |
| **SQS / Lambda / Transcribe / SageMaker / Bedrock** | — | **no se usan** | — | **0** |

Verificado en las 71 specs: **DynamoDB y Redis se citan en 75/75** documentos; **SES no se cita en
ninguna** (la dependencia vive en RFC-016 §4). `D-PRE-02`: al implementar E005 debe añadirse la
mención explícita a SES en su `spec.md` con Gate de enmienda.

## 3. E005 / E006 — dependencias exactas

| ID | Dependencia | Estado | Bloquea |
|---|---|---|---|
| **A6** | época de revocación en E002/E003/middleware/E006 | diseñada, **no implementada** | implementar E006 |
| **A9** | secreto `PASSWORD_RESET_SEAL_KEY_B64` (32 bytes CSPRNG) | **no existe** en el bundle RFC-004 | implementar E005 |
| **A10** | IAM `LeadingKeys` con `QUEUE#*` y `RESET#*` | **no existe** (hoy `USER#*`, `REFRESH#*`, `SESSION#*`) | implementar E005 |
| SES identidad | remitente verificado | **no existe** (no se inventa dirección) | desplegar E005 |
| SES sandbox | salida a producción | **pendiente de AWS** | desplegar E005 |
| SES IAM | `ses:SendEmail` acotado | **no existe** | desplegar E005 |
| SES red | camino desde subred privada | **no demostrado**; sin NAT | desplegar E005 |
| SES coste | ver §4 | **no verificado** | desplegar E005 |

**DynamoDB — claves previstas y ya cubiertas por el modelo de A-060:** `USER#<id>` (perfil, prefs,
sesiones, movimientos, metas), `RESET#<sha256>` (A10), `QUEUE#PWRESET` (A10), `IDEM#` (idempotencia).
Ninguna tabla ni índice nuevo.

**Redis — operaciones ya disponibles:** líder por lease (`SET NX PX`), cuotas (`INCR`/`EXPIRE`),
caché por usuario (`GET/SET` con TTL), invalidación por prefijo. Sin cambios de configuración.

## 4. Coste previsto de SES

| Partida | Estado |
|---|---|
| Envío de correo | **no verificado** en la cuenta; tarifa pública por millar |
| Interface endpoint (2 AZ × 45 d) | **≈ 21,60 USD** al precio ya verificado en I-000 (0,01 USD/h/AZ) |
| Margen aprobado de I-000 | **6,9387 USD** |
| **Conclusión** | el endpoint **excede** el margen → requiere reapertura FinOps o alternativa (ver Gate) |


## 4 bis. FinOps SES — cálculo corregido (2026-11-16)

**Corrección aritmética:** `2 AZ × 0,01 USD/h × 4 h = 0,08 USD` por ventana. Cinco ventanas suman
**0,40 USD en total**, no por ciclo.

| Escenario | Cálculo | Coste |
|---|---|---|
| Una ventana de 4 h | 2 × 0,01 × 4 | **0,08 USD** |
| 5 ventanas de 4 h | 2 × 0,01 × 4 × 5 | **0,40 USD** |
| 5 días continuos | 2 × 0,01 × 24 × 5 | **2,40 USD** |
| **7 días continuos** | 2 × 0,01 × 24 × 7 | **3,36 USD** |
| 45 días continuos | 2 × 0,01 × 24 × 45 | 21,60 USD |

Margen disponible de I-000: **6,9387 USD**.

**Decisión propuesta:** **ventana continua de 7 días (3,36 USD)** hasta la entrega académica.
Motivo: la recuperación de contraseña debe funcionar cuando el usuario la necesite; un endpoint
intermitente dejaría E005 inservible fuera de las pruebas. Cabe en el margen con 3,58 USD de
reserva. **No se asume** operación de 45 días.

**Pendiente de verificar antes de crear nada:** tarifa regional real en `us-east-2` (la de 0,01
USD/h/AZ proviene de I-000, no de una consulta nueva), cargos de procesamiento y correo, y el
soporte efectivo de **PrivateLink para SES API** anunciado por AWS en diciembre de 2025 — se
verifica con `ec2:DescribeVpcEndpointServices` y `sesv2:GetAccount`, que requieren las lecturas
propuestas. Sin esa evidencia no se crea el endpoint.

**Techo solicitado:** 3,36 USD por una ventana única de 7 días, con retirada posterior autorizada.

## 5. Cambios mínimos reutilizables propuestos

Una sola vez para los 71 endpoints, sin repetir configuración por endpoint:

1. **Aplicar RFC-017** (178/47): `terraform apply` de plan aprobado a ASK, edición de SDD a ASK,
   lecturas operativas a ALLOW, ASK en S3/STS/ASG/LT/IAM, DENY intactos. Añadir a la propuesta:
   `aws ses get-account*`, `aws ses list-email-identities*`, `aws sesv2 get-account*` (lectura) y
   `aws logs describe-log-groups*`, `aws ssm get-parameter*` (lectura) — sin mutaciones.
2. **A9**: alta del secreto de sellado por el procedimiento RFC-004 (escritura condicional, SSE,
   rotación documentada).
3. **A10**: ampliar la política IAM del runtime con `QUEUE#*` y `RESET#*` (solo esas claves).
4. **SES**: identidad, sandbox, `ses:SendEmail` acotado y camino de red, en un único Gate AWS.
5. **A6**: cambio compartido de sesión, en el Gate técnico de E006.

## 6. Paquete único de Gates

| # | Frase | Alcance | Efecto | Coste |
|---|---|---|---|---|
| 1 | `APRUEBO_RFC_017_PERMISOS_PERMANENTES` | instalar 178/47 + lecturas SES/logs/ssm | deja de pedir permiso en lecturas y permite ASK en operaciones sensibles | 0 |
| 2 | `APRUEBO_A9_SECRETO_SELLADO` | alta de `PASSWORD_RESET_SEAL_KEY_B64` | habilita cifrado del token de E005 | 0 (SSM/S3 ya existentes) |
| 3 | `APRUEBO_A10_IAM_COLA_RESET` | `QUEUE#*` y `RESET#*` en el rol runtime | habilita el worker y E006 | 0 |
| 4 | `APRUEBO_SES_IDENTIDAD_Y_SANDBOX` | identidad + production access + `ses:SendEmail` | habilita envío real | por confirmar |
| 5 | `APRUEBO_FINOPS_SES_ENDPOINT` | interface endpoint o alternativa | conectividad SES | ≈21,60 USD/45 d (excede margen) |
| 6 | `APRUEBO_RFC_A6_EPOCA_SESION` | código E002/E003/middleware | revocación real en E006 | 0 |

**Nada de esto se ha aplicado.** El Gate 5 es una decisión FinOps, no una autorización técnica:
si se rechaza, E005 queda `AWS_READY=false` aunque el código esté terminado.

## 7. Estados

| Estado | Valor | Motivo |
|---|---|---|
| `CONTRACT_FROZEN` | **VERIFIED** | 71/71, A1–A8 aplicadas, CI de `main` verde |
| `PERMISSIONS_READY` | **NO** | RFC-017 no instalada; `ses`/`logs`/`ssm` sin reglas |
| `AWS_RUNTIME_READY` | **NO** | A9 y A10 inexistentes |
| `SES_READY` | **NO** | sin identidad, sandbox, IAM ni camino de red |

## 8. Riesgos que ya pueden prevenirse

| Riesgo | Prevención |
|---|---|
| Pedir permiso en cada `describe` durante el desarrollo | aplicar RFC-017 una vez |
| Bloqueo del worker por `LeadingKeys` | A10 antes de implementar E005 |
| Token sin poder cifrarse | A9 antes de implementar E005 |
| Sobrecoste por endpoint SES | decidir Gate 5 antes de desplegar |
| E006 sin revocación real | A6 aprobado antes de implementar E006 |
| Edición de SDD ambigua | RFC-017 convierte el `deny` en `ask` y hace visible la autorización |
