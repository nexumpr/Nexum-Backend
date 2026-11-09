# Matriz de aceptación de contratos — I-140 (INF-140-04)

| Campo | Valor |
|---|---|
| Spec | `I-140-operacion` |
| Versión | 0.2.0 |
| Estado | **IMPLEMENTADO_LOCAL — NO DESPLEGADO** |
| Trazabilidad | 12 A · 16 I · 75 E |
| Verificación cloud | `NOT_RUN` |

> Documentar una fila no es implementar. Los contratos E001–E075 son OpenAPI: **no** demuestran
> handlers funcionales ni respuestas HTTP reales.

## 1. Especificaciones A (12)

| ID | Requisito | Implementación | Prueba | Evidencia | Estado | Dependencia |
|---|---|---|---|---|---|---|
| A-000 | Tipos y frontera del sistema | pkg/types/system.go, internal/system | tests/unit/system | Gate H08-A | INTEGRADO_MAIN | AWS_NOT_RUN |
| A-010 | Stack Go, HTTP/JSON y puertos | pkg/httputil, internal/ports, pkg/telemetry | tests/unit/platform | Gate H08-A | INTEGRADO_MAIN | AWS_NOT_RUN |
| A-020 | Clean Architecture y statelessness | internal/domain, internal/application | tests/unit/architecture | Gate H08-A | INTEGRADO_MAIN | AWS_NOT_RUN |
| A-030 | Servidor HTTP versionado | internal/transport/http | tests/unit/transport | Gate H08-A | INTEGRADO_MAIN | AWS_NOT_RUN |
| A-040 | Contratos HTTP y paginación | pkg/httputil/contract.go, cursor.go | tests/unit/transport/contracts_test.go | Gate H08-A | INTEGRADO_MAIN | AWS_NOT_RUN |
| A-050 | Seguridad e identidad | internal/infrastructure/security | tests/unit/security | Gate H08-A | INTEGRADO_MAIN | AWS_NOT_RUN |
| A-060 | Modelo DynamoDB | internal/domain/dynamo, internal/infrastructure/dynamo | tests/unit/dynamo | Gate H08-A | INTEGRADO_MAIN | AWS_NOT_RUN |
| A-070 | Consistencia financiera | internal/domain/financial, internal/application/financial | tests/unit/financial | Gate H08-A | INTEGRADO_MAIN | AWS_NOT_RUN |
| A-080 | Caché, paginación y rate limit | internal/infrastructure/cache | tests/unit/cache | Gate H08-A | INTEGRADO_MAIN | AWS_NOT_RUN |
| A-090 | Observabilidad y salud | internal/infrastructure/telemetry, internal/transport/http/health_handler.go | tests/unit/telemetry | Gate H08-A | INTEGRADO_MAIN | AWS_NOT_RUN |
| A-100 | QA y DevSecOps | internal/infrastructure/security/audit.go | tests/unit/devsecops | Gate H08-A | INTEGRADO_MAIN | AWS_NOT_RUN |
| A-110 | Versionado y contribución | pkg/types/version.go, internal/system/governance.go | tests/unit/system | Gate H08-A | INTEGRADO_MAIN | AWS_NOT_RUN |

## 2. Especificaciones I (16)

| ID | Requisito | Implementación | Prueba | Evidencia | Estado | Dependencia |
|---|---|---|---|---|---|---|
| I-000 | Cuenta y FinOps | infra/finops | tests/infra/finops | ESTADO_Y_GATES | INTEGRADO_MAIN | IVA y evidencia AWS |
| I-010 | Red VPC y endpoints | infra/terraform/modules/red | tests/infra/terraform | ESTADO_Y_GATES | INTEGRADO_MAIN | AWS_NOT_VERIFIED |
| I-020 | Cómputo Go ARM64 | infra/terraform/modules/computo | tests/infra/terraform | ESTADO_Y_GATES | INTEGRADO_MAIN | agente CloudWatch |
| I-030 | Balanceo y HTTPS | infra/terraform/modules/balanceo | tests/infra/terraform | ESTADO_Y_GATES | INTEGRADO_MAIN | H-01 |
| I-040 | DynamoDB | infra/terraform/modules/dynamodb | tests/infra/terraform | ESTADO_Y_GATES | INTEGRADO_MAIN | AWS_NOT_VERIFIED |
| I-050 | Redis OSS | infra/terraform/modules/redis | tests/infra/terraform | ESTADO_Y_GATES | INTEGRADO_MAIN | sin HA |
| I-060 | IAM y OIDC | infra/terraform/modules/iam | tests/infra/terraform | ESTADO_Y_GATES | INTEGRADO_MAIN | AWS_SCP_NOT_VERIFIED |
| I-070 | Bootstrap de state | infra/terraform/bootstrap | tests/infra/terraform | ESTADO_Y_GATES | INTEGRADO_MAIN | backend remoto no activo |
| I-080 | CI Backend | .github/workflows/go-backend.yml | tests/infra/ci | ESTADO_Y_GATES | INTEGRADO_MAIN | branch protection |
| I-090 | CD Backend | .github/workflows/cd-backend.yml | tests/infra/cd | ESTADO_Y_GATES | INTEGRADO_MAIN | ARTIFACT_DELIVERY |
| I-100 | DevSecOps | herramientas/verificar_iac_devsecops.py | tests/security | ESTADO_Y_GATES | INTEGRADO_MAIN | D-100-01/02 |
| I-110 | Observabilidad y costos | infra/terraform/modules/observabilidad | tests/infra/terraform | ESTADO_Y_GATES | INTEGRADO_MAIN | RFC-007 |
| I-120 | Encendido y apagado | infra/terraform/modules/operacion, herramientas/nexum_operacion.py | tests/infra/operacion | ESTADO_Y_GATES | INTEGRADO_MAIN | BLOCKED_I130 |
| I-130 | Respaldo y recuperación | infra/terraform/modules/respaldo, herramientas/verificar_backup_restore.py | tests/infra/respaldo | ESTADO_Y_GATES | INTEGRADO_MAIN | RESTORE_AWS_NOT_RUN |
| I-140 | Operación e incidentes | herramientas/verificar_operacion_i140.py | tests/infra/operacion | ESTADO_Y_GATES | IMPLEMENTADO_LOCAL | drills NOT_RUN |
| I-150 | Publicación y compatibilidad API | sdd/03-endpoints (contratos) | NOT_RUN | ESTADO_Y_GATES | NOT_AUTHORIZED | Gate I-150 |

## 3. Contratos E (75)

| ID | Contrato | Implementación | Prueba | Evidencia | Estado | Dependencia |
|---|---|---|---|---|---|---|
| E001 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E002 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E003 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E004 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E005 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E006 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E007 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E008 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E009 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E010 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E011 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E012 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E013 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E014 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E015 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E016 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E017 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E018 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E019 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E020 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E021 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E022 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E023 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E024 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E025 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E026 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E027 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E028 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E029 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E030 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E031 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E032 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E033 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E034 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E035 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E036 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E037 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E038 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E039 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E040 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E041 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E042 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E043 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E044 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E045 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E046 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E047 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E048 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E049 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E050 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E051 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E052 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E053 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E054 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E055 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E056 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E057 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E058 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E059 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E060 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E061 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E062 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E063 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E064 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E065 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E066 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E067 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E068 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E069 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E070 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E071 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E072 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E073 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E074 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |
| E075 | OpenAPI 3.1 | **sin handler funcional** | NOT_RUN | sdd/03-endpoints | CONTRATO_OPENAPI | H08-E NOT_AUTHORIZED |

## 4. Criterios de aceptación cloud (pendientes)

| Criterio | Comprobación prevista | Estado |
|---|---|---|
| HTTPS público | HTTP real contra el dominio CloudFront | `NOT_RUN` |
| CloudFront + ALB | Distribución y origen VPC verificados | `NOT_RUN` |
| Dos targets sanos simultáneos | `HealthyHostCount = 2` y 2 IDs en AZ distintas | `NOT_RUN` |
| Health y readiness | 200 en liveness; readiness según dependencias | `NOT_RUN` |
| Logs y trazabilidad | Logs del servidor y trazas W3C correlacionadas | `NOT_RUN` |
| Persistencia DynamoDB | Escritura y lectura reales de un movimiento sintético | `NOT_RUN` |
| Redis compartido | Clave visible desde ambos nodos | `NOT_RUN` |
| Seguridad | 401/403, aislamiento y revocación reales | `NOT_RUN` |
| Recuperación | Restore aislado medido (I-130) | `NOT_RUN` |
| Control de costos | Billing y créditos verificados | `NOT_RUN` |
| Cliente HTTP independiente | Prueba desde fuera de AWS | `NOT_RUN` |

Todos los criterios cloud exigen el Gate de despliegue; ninguno se declara cumplido.