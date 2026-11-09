# Máquina de estados del runtime — I-120

| Campo | Valor |
|---|---|
| Spec | `sdd/02-infraestructura/I-120-encendido-apagado` |
| Versión | 0.1.0 |
| Estado | **IMPLEMENTADO_LOCAL — NO DESPLEGADO** |
| Evidencia AWS | `AWS_NOT_RUN` |
| Implementación | `herramientas/nexum_operacion.py` + `infra/terraform/modules/operacion` |

> Este documento describe el diseño implementado y probado en local. **No** acredita que exista
> runtime AWS: no se ha ejecutado `terraform plan/apply`, no se ha creado ningún recurso y el estado
> real del entorno es `DESCONOCIDO` hasta que exista evidencia autorizada.

## 1. Estados

| Estado | Significado | ¿Operativo? | ¿Publicable? |
|---|---|---|---|
| `OFF` | Runtime efímero deshabilitado. Los datos y el state persisten. | No | No |
| `STARTING` | Aprovisionando; aún sin 2 targets sanos. | Parcial | **No** |
| `ON_2_HEALTHY` | Único estado operativo validable: 2 targets sanos. | Sí | Sí |
| `DRAINING` | Drenando peticiones y cerrando ventana. | Degradado | No |
| `ERROR` | Fallo detectado; requiere diagnóstico humano. | No | No |
| `COST_REVIEW` | Revisión de costo/saldo sin apagar el core. | Sí | Sí |

`COST_REVIEW` existe porque el ahorro **no** puede ejecutarse automáticamente: los umbrales de saldo
solo generan aviso y escalado al responsable (spec §10). Low saldo no autoriza apagar el core ni pasar
a plan Paid.

## 2. Transiciones permitidas

| Desde | Hacia permitidos |
|---|---|
| `OFF` | `STARTING` |
| `STARTING` | `ON_2_HEALTHY`, `DRAINING`, `ERROR` |
| `ON_2_HEALTHY` | `DRAINING`, `COST_REVIEW`, `ERROR` |
| `DRAINING` | `OFF`, `ERROR` |
| `COST_REVIEW` | `ON_2_HEALTHY`, `DRAINING`, `ERROR` |
| `ERROR` | `DRAINING`, `STARTING` |

Reglas aplicadas por el operador:

1. `OFF → ON_2_HEALTHY` está **prohibido**: no se puede saltar la verificación de salud.
2. Toda transición exige `--actor` identificable y `--runbook` con el procedimiento aplicado.
3. Repetir el mismo estado es idempotente y no genera transición.
4. `ON_2_HEALTHY` exige `targets_sanos >= 2`. Con 1 nodo el servicio **no** es publicable
   (`GOV-02`): un nodo no es doble instancia.

La transición devuelve, además de `desde`/`hacia`, un `timestamp_utc` (salvo repetición idempotente).
El operador solo produce el **plan**; ejecutar el cambio en AWS requiere el Gate de despliegue.

## 3. Qué puede cambiar cada estado

| Elemento | `OFF` | `ON_2_HEALTHY` |
|---|---|---|
| 2 × EC2 Go (`asg-backend`) | **Core: no se detiene** | 2 targets sanos |
| ALB interno + LCU | **Core** | Sirviendo |
| CloudFront (HTTPS público) | **Core** | Sirviendo |
| Redis OSS | **Core** | Compartido por ambos nodos |
| DynamoDB + PITR | **Core, datos intactos** | Datos intactos |
| Terraform state (S3) | **Core con `prevent_destroy`** | Igual |
| Endpoints `secretsmanager`, `ssm`, `ssmmessages` | Suspendibles (auxiliar) | Solo si hay ventana |
| Endpoints `logs` | **Core, permanente** | Igual |

Según la enmienda §9 de la spec, `OFF` **no aplica al core compartido**: solo a laboratorios y
recursos auxiliares. Nadie, incluido OpenCode, puede apagar un recurso core por su cuenta.

## 4. Guardas ejecutables

`infra/terraform/modules/operacion` (preconditions que fallan cerrado):

- recursos denegados (`denegados != 0`): sin etiqueta `nexum:criticality` válida, auxiliar fuera de
  allowlist, criticidad incoherente o core marcado como suspendible;
- allowlist con nombres inexistentes en el inventario;
- inventario sin core de `computo`, `datos` o `estado` (un inventario incompleto haría creer que
  apagar no tiene coste);
- `destroy_datos` o `destroy_estado` distintos de `false` (variables con validación permanente).

`herramientas/nexum_operacion.py` (códigos de salida 0 correcto, 1 bloqueado, 2 prohibido):

- `destroy-data`, `destroy-state` y `destroy-all` **no existen** como comandos: se rechazan con
  mensaje específico y sin efecto;
- `suspender-aux` / `destruir-aux` solo operan sobre la allowlist; el core, los desconocidos y los no
  etiquetados se deniegan aunque el operador los pida por nombre;
- si el estado observado muestra un recurso core suspendido, se trata como **incidente**, no como
  ahorro;
- el rollback nunca destruye datos ni state, y rechaza snapshots que lo soliciten.

## 5. Evidencia

| Comprobación | Resultado |
|---|---|
| Pruebas del operador (`tests/infra/operacion`) | 54 OK |
| Contrato estático Terraform (`tests/infra/terraform/test_operacion_contrato.py`) | Incluido en la suite de 255+ |
| `terraform fmt` / `init -backend=false` / `validate` | PASS |
| Transiciones reales en AWS | `NOT_RUN` |
| Estado observado del entorno | `DESCONOCIDO` (sin evidencia AWS) |

## 6. Límites

- No se declara `AWS_DEPLOYED`, `RUNTIME_VERIFIED` ni `COSTO_MEDIDO`.
- El mecanismo de secretos sigue bloqueado por `RFC-004`.
- Los backups programados de DynamoDB dependen de I-130 (`NOT_AUTHORIZED`).
- El umbral de saldo no es un hard cap y no detiene recursos.
