# Plan de pruebas operativas — I-140

| Campo | Valor |
|---|---|
| Spec | `I-140-operacion` |
| Versión | 0.2.0 |
| Estado | **IMPLEMENTADO_LOCAL — NO DESPLEGADO** |
| Drills | 12 |
| Ejecución AWS | `NOT_RUN` (`DRILLS_AWS_NOT_RUN`) |

> Un drill local no demuestra dos EC2 reales, DynamoDB persistente ni Redis distribuido. Eso exige
> el Gate de despliegue. Estados posibles: `LOCAL_PASS`, `LOCAL_FAIL`, `NOT_RUN_AWS`,
> `BLOCKED_DEPENDENCY`.

| ID | Componente | Escenario | Criterio | Rollback | AWS | Estado |
|---|---|---|---|---|---|---|
| `caida-instancia-a` | computo | 1 target sano; el ALB envia todo a B | Servicio responde 200 con 1 nodo y el ASG repone | Restaurar capacidad con la LT vigente | Sí | **NOT_RUN_AWS** |
| `continuidad-b` | computo | B atiende el 100% | Sin 5xx atribuibles y 2 targets sanos al final | Reponer A desde la LT | Sí | **NOT_RUN_AWS** |
| `reemplazo-target` | balanceo | target fuera del pool durante el refresh | HealthyHostCount vuelve a 2 | Cancelar el refresh | Sí | **NOT_RUN_AWS** |
| `redis-no-disponible` | cache | lecturas sin cache | Auth fail-closed y sin 5xx sostenidos | Restablecer endpoint anterior | Sí | **NOT_RUN_AWS** |
| `dynamo-throttling` | datos | throttling controlado | Backoff efectivo sin Scan | Detener la carga sintetica | Sí | **NOT_RUN_AWS** |
| `timeouts-reintentos` | transporte | timeouts acotados | Reintentos no duplican efectos | Ninguno: solo lectura | No | **LOCAL_PASS** |
| `operacion-financiera-parcial` | datos | escritura parcial simulada | Invariantes de saldo se conservan | Descartar el fixture | No | **LOCAL_PASS** |
| `idempotencia` | datos | repeticion de la misma clave | Respuesta identica y sin duplicados | Ninguno: solo lectura | No | **LOCAL_PASS** |
| `revocacion-rotacion-jwt` | identidad | tokens previos invalidados | Sesiones revocadas no se reactivan | Restaurar claves en ventana | No | **LOCAL_PASS** |
| `health-readiness` | observabilidad | readiness 503 con dependencia caida | Liveness 200 y readiness 503 correcto | Ninguno: solo lectura | No | **LOCAL_PASS** |
| `perdida-observabilidad` | observabilidad | logs no llegan | Alarma de ausencia de logs detectable | Restaurar agente | Sí | **BLOCKED_DEPENDENCY** |
| `riesgo-rollback` | despliegue | rollback a LT previa | Vuelve a 2 targets sanos | Reaplicar LT previa | No | **LOCAL_PASS** |

## Condiciones

- Datos **sintéticos** en todos los drills; ninguno usa datos personales.
- Prohibido destruir tablas, apagar el core o dejar el ASG por debajo de 2.
- Los drills con `Sí` en AWS permanecen `NOT_RUN_AWS` hasta el Gate de despliegue.
- `perdida-observabilidad` queda `BLOCKED_DEPENDENCY` por el agente CloudWatch (I-020).

## Validación local

```powershell
py -3.13 herramientas/verificar_operacion_i140.py drills --datos tests/fixtures/operacion/drills.json
```

Rechaza drills faltantes, estados desconocidos, `demuestra_aws=true`, acciones destructivas y
drills con AWS ejecutados en local.