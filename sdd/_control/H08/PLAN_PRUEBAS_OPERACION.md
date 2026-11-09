# Plan de pruebas operativas — I-140

| Campo | Valor |
|---|---|
| Spec | `I-140-operacion` |
| Versión | 0.4.0 |
| Estado | **IMPLEMENTADO_LOCAL — NO DESPLEGADO** |
| Drills | 12 |
| `LOCAL_PASS` | 6, con prueba existente **y ejecutada en el workflow de CI** |
| `NOT_RUN_AWS` | 5 |
| `BLOCKED_DEPENDENCY` | 1 (`perdida-observabilidad`, I-020) |
| Ejecución AWS | `NOT_RUN` (`DRILLS_AWS_NOT_RUN`) |

> El validador comprueba **trazabilidad**: que el archivo existe, que la prueba aparece en él y que el
> nombre de la prueba figura en el workflow de CI (donde se ejecuta con `-run` focalizado). **No**
> re-ejecuta las pruebas ni sustituye el log de CI: la ejecución se acredita con el run de GitHub
> Actions sobre el commit del PR.

| ID | Componente | Escenario | AWS | Estado | Prueba |
|---|---|---|---|---|---|
| `caida-instancia-a` | computo | 1 target sano | Sí | **NOT_RUN_AWS** | — |
| `continuidad-b` | computo | B atiende el 100% | Sí | **NOT_RUN_AWS** | — |
| `reemplazo-target` | balanceo | target fuera del pool | Sí | **NOT_RUN_AWS** | — |
| `redis-no-disponible` | cache | lecturas sin cache | Sí | **NOT_RUN_AWS** | — |
| `dynamo-throttling` | datos | throttling controlado | Sí | **NOT_RUN_AWS** | — |
| `timeouts-reintentos` | transporte | timeouts acotados | No | **LOCAL_PASS** | `TestHTTPServerStatusesAndErrors` |
| `operacion-financiera-parcial` | datos | escritura parcial | No | **LOCAL_PASS** | `TestGoalReversionPreservesInvariant` |
| `idempotencia` | datos | repeticion de clave | No | **LOCAL_PASS** | `TestIdempotencyManagerBehavior` |
| `revocacion-rotacion-jwt` | identidad | tokens invalidados | No | **LOCAL_PASS** | `TestRefreshTokenReplayFamilyRevocation` |
| `health-readiness` | observabilidad | readiness 503 | No | **LOCAL_PASS** | `TestReadinessProbeDegradation503` |
| `perdida-observabilidad` | observabilidad | logs no llegan | Sí | **BLOCKED_DEPENDENCY** | bloqueado por I-020 (agente CloudWatch) |
| `riesgo-rollback` | despliegue | rollback a LT previa | No | **LOCAL_PASS** | `DecisionRollback` |

## Ejecución en CI

El job «Formato, vet, build y pruebas» incluye dos pasos con los nombres de prueba exactos:

- `Evidencia de drills I-140 (Go)`: ejecuta con `go test -run` los cinco tests Go.
- `Evidencia de drills I-140 (Python)`: ejecuta `DecisionRollback` de `test_cd_helpers`.

El resultado verificable es el log del run sobre el HEAD del PR; el JSON solo aporta trazabilidad.

## Condiciones

- Datos **sintéticos**; ningún drill usa datos personales.
- Prohibido destruir tablas, apagar el core o dejar el ASG por debajo de 2.
- Los drills con AWS permanecen `NOT_RUN_AWS` hasta el Gate de despliegue.
- `perdida-observabilidad` queda `BLOCKED_DEPENDENCY` por el agente CloudWatch (I-020).

## Validación local

```powershell
py -3.13 herramientas/verificar_operacion_i140.py drills --datos tests/fixtures/operacion/drills.json
```