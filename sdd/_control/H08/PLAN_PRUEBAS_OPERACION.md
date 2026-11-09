# Plan de pruebas operativas — I-140

| Campo | Valor |
|---|---|
| Spec | `I-140-operacion` |
| Versión | 0.3.0 |
| Estado | **IMPLEMENTADO_LOCAL — NO DESPLEGADO** |
| Drills | 12 |
| `LOCAL_PASS` | 6, cada uno con archivo y prueba reales |
| `NOT_RUN_AWS` | 5 (requieren AWS) |
| `BLOCKED_DEPENDENCY` | 1 (`perdida-observabilidad`, I-020) |
| Ejecución AWS | `NOT_RUN` (`DRILLS_AWS_NOT_RUN`) |

> `LOCAL_PASS` exige archivo y prueba existentes y distintos por escenario; el validador comprueba
> que la prueba aparece en el archivo y rechaza evidencias ausentes, falsas o duplicadas. Un drill
> local **no** demuestra dos EC2 reales, DynamoDB persistente ni Redis distribuido.

| ID | Componente | Escenario | Criterio | AWS | Estado | Evidencia local |
|---|---|---|---|---|---|---|
| `caida-instancia-a` | computo | 1 target sano | responde 200 y el ASG repone | Sí | **NOT_RUN_AWS** | — |
| `continuidad-b` | computo | B atiende el 100% | 2 targets sanos al final | Sí | **NOT_RUN_AWS** | — |
| `reemplazo-target` | balanceo | target fuera del pool | HealthyHostCount vuelve a 2 | Sí | **NOT_RUN_AWS** | — |
| `redis-no-disponible` | cache | lecturas sin cache | auth fail-closed sin 5xx | Sí | **NOT_RUN_AWS** | — |
| `dynamo-throttling` | datos | throttling controlado | backoff efectivo sin Scan | Sí | **NOT_RUN_AWS** | — |
| `timeouts-reintentos` | transporte | timeouts acotados | sin efectos duplicados | No | **LOCAL_PASS** | `TestHTTPServerStatusesAndErrors` en `tests/unit/transport/http_server_test.go` |
| `operacion-financiera-parcial` | datos | escritura parcial | invariantes de saldo | No | **LOCAL_PASS** | `TestGoalReversionPreservesInvariant` en `tests/unit/financial/consistency_resilience_test.go` |
| `idempotencia` | datos | repeticion de clave | respuesta identica | No | **LOCAL_PASS** | `TestIdempotencyManagerBehavior` en `tests/unit/financial/consistency_resilience_test.go` |
| `revocacion-rotacion-jwt` | identidad | tokens invalidados | sesiones revocadas no se reactivan | No | **LOCAL_PASS** | `TestRefreshTokenReplayFamilyRevocation` en `tests/unit/security/auth_service_test.go` |
| `health-readiness` | observabilidad | readiness 503 | liveness 200 y readiness 503 | No | **LOCAL_PASS** | `TestReadinessProbeDegradation503` en `tests/unit/telemetry/observability_health_test.go` |
| `perdida-observabilidad` | observabilidad | logs no llegan | alarma de ausencia | Sí | **BLOCKED_DEPENDENCY** | bloqueado por I-020 (agente CloudWatch) |
| `riesgo-rollback` | despliegue | rollback a LT previa | 2 targets sanos | No | **LOCAL_PASS** | `DecisionRollback` en `herramientas/tests/test_cd_helpers.py` |

## Condiciones

- Datos **sintéticos**; ningún drill usa datos personales.
- Prohibido destruir tablas, apagar el core o dejar el ASG por debajo de 2.
- Los drills con AWS permanecen `NOT_RUN_AWS` hasta el Gate de despliegue.
- `perdida-observabilidad` queda `BLOCKED_DEPENDENCY` por el agente CloudWatch (I-020).

## Validación local

```powershell
py -3.13 herramientas/verificar_operacion_i140.py drills --datos tests/fixtures/operacion/drills.json
```

Rechaza drills faltantes, estados desconocidos, `demuestra_aws=true`, acciones destructivas,
`LOCAL_PASS` sin evidencia verificable y evidencias duplicadas entre escenarios.