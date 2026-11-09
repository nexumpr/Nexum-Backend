# Runbook de encendido, apagado y reconciliación — I-120

| Campo | Valor |
|---|---|
| Spec | `sdd/02-infraestructura/I-120-encendido-apagado` |
| Versión | 0.1.0 |
| Estado | **IMPLEMENTADO_LOCAL — NO DESPLEGADO** |
| Herramienta | `herramientas/nexum_operacion.py` |
| Ejecución AWS | `AWS_NOT_RUN` (denegada en este Gate) |

> Todo se opera por declaración y pipeline, **nunca por clics rutinarios en consola** (spec §10).
> Este Gate **no** autoriza `terraform plan/apply/destroy` ni ninguna llamada AWS mutante.

## 1. Reglas que no se negocian

1. El **core** (2 × EC2 Go, ALB, CloudFront, Redis OSS, DynamoDB, state, logs permanentes) es 24×7 e
   intocable. Ningún comando auxiliar lo detiene, lo escala por debajo de 2 ni lo elimina.
2. `destroy-data`, `destroy-state` y `destroy-all` **no existen**. El operador los rechaza con código 2.
3. Un recurso desconocido o sin etiqueta `nexum:criticality` válida se **deniega** siempre.
4. No se publica el servicio sin **2 targets sanos**: 1 nodo no es doble instancia (`GOV-02`).
5. Toda transición exige actor identificable y runbook; el registro incluye `timestamp_utc`.
6. Los umbrales de saldo generan aviso y escalado, **no** cortes automáticos ni `Paid`.

## 2. Preparación

```powershell
# Estado actual (local, sin AWS): clasificación + residual + doble instancia
py -3.13 herramientas/nexum_operacion.py estado --salida plan.json

# Estado observado: se aporta como archivo, nunca se consulta AWS desde el operador
# {"estado": "ON_2_HEALTHY", "targets_sanos": 2, "recursos_presentes": [...]}
py -3.13 herramientas/nexum_operacion.py estado --observado observado.json
```

El operador no habla con AWS ni con Terraform: produce un plan reproducible y declara
`aws_consultado=false`, `terraform_ejecutado=false`.

## 3. Encender (plan)

```powershell
py -3.13 herramientas/nexum_operacion.py encender `
  --aprobacion APRUEBO_ENCENDER_H08I `
  --actor "<responsable>" --runbook RUNBOOK_START_STOP.md `
  --observado observado.json --salida plan-encender.json
```

Secuencia exigida:

1. Verificar plan Free, créditos y cuota (evidencia externa; sin dato se declara `BLOQUEADO`).
2. `desiredCapacity=2` con `min=desired=max` (I-020).
3. Comprobar `/api/v1/health/live` en los 2 targets y readiness según su Gate.
4. No exponer la URL pública hasta `ON_2_HEALTHY`.
5. Auditoría de costo inicial; no declarar costo 0 sin facturación autorizada.

## 4. Suspender y reanudar auxiliares

Solo alcanza la allowlist revisada (hoy: `interface-endpoint-secretsmanager`,
`interface-endpoint-ssm`, `interface-endpoint-ssmmessages`).

```powershell
py -3.13 herramientas/nexum_operacion.py suspender-aux `
  --aprobacion APRUEBO_SUSPENDER_AUX_H08I --actor "<responsable>" `
  --runbook RUNBOOK_START_STOP.md --seleccion interface-endpoint-ssm

py -3.13 herramientas/nexum_operacion.py reanudar-aux `
  --aprobacion APRUEBO_REANUDAR_AUX_H08I --creditos <saldo declarado>
```

`suspend-aux` es la ventana: se aplica con `terraform plan` revisado y aprobación por ambiente.
`reanudar-aux` exige créditos conocidos; por debajo del umbral de bloqueo devuelve código 1.

## 5. Reconciliar (drift)

```powershell
py -3.13 herramientas/nexum_operacion.py reconciliar --observado observado.json
```

- `CONVERGENTE` solo con inventario observado completo.
- `DRIFT` si falta core o hay recursos no declarados; **no** se recrea nada a ciegas.
- Sin observado se devuelve `BLOQUEADO`: no se asume convergencia.

## 6. Rollback

```powershell
py -3.13 herramientas/nexum_operacion.py rollback --snapshot anterior.json `
  --actor "<responsable>" --runbook RUNBOOK_START_STOP.md
```

Rechaza snapshots con `destroy_datos`, `destroy_estado`, `destroy_all` o recursos no autorizados.
Devuelve siempre `datos_destruidos=false` y `estado_destruido=false`.

## 7. Destruir runtime auxiliar

```powershell
py -3.13 herramientas/nexum_operacion.py destruir-aux `
  --aprobacion APRUEBO_DESTRUIR_AUX_H08I --actor "<responsable>" `
  --runbook RUNBOOK_START_STOP.md
```

`destroy-runtime` está separado de `destroy-data` y `destroy-state` (INF-120-05). Solo afecta
auxiliares en allowlist y reporta el residual que permanece.

## 8. Apagado: qué NO se apaga

| Recurso | Apagado | Motivo |
|---|---|---|
| 2 × EC2 Go | No | `GOV-02`; detener rompe la API |
| ALB | No | No existe API Stop; eliminarlo exige reprovisionar y perder disponibilidad |
| CloudFront | No | Único acceso HTTPS (D2) |
| Redis OSS | No | Caché compartido; eliminarlo cambia el entorno |
| DynamoDB | No | Datos del usuario; `destroy-data` inexistente |
| State S3 | No | `prevent_destroy`; perderlo impide reconciliar |
| Endpoint `logs` | No | Logs permanentes (D6) |
| 3 endpoints de ventana | Sí, en ventana | Auxiliares con allowlist |

Parar una EC2 no elimina ALB, IPv4, Redis, EBS ni facturación de logs. El residual se enumera en
`INVENTARIO_RESIDUAL.md`.

## 9. Incidentes

| Síntoma | Acción |
|---|---|
| Core observado como suspendido | `ErrorUso`: incidente, no ahorro. Escalar y restaurar antes de continuar |
| `denegados != 0` en el plan | Revisar etiqueta `nexum:criticality` y allowlist; no aplicar |
| 1 solo target sano | No publicar; el estado queda `STARTING` |
| Saldo por debajo del umbral de bloqueo | Bloquear auxiliares nuevos; escalar al responsable |
| Snapshot con destroy | Rechazado; no se relaja la guarda para "recuperar" el cambio |

## 10. Evidencia

- `terraform fmt -check`, `init -backend=false`, `validate`: PASS en local.
- 54 pruebas del operador y el contrato estático del módulo: PASS.
- Ejecuciones reales en AWS: `NOT_RUN`. No existen recursos creados ni costo medido.
