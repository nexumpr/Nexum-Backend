# Matriz de contratos INF-120-01..06 — I-120

| Campo | Valor |
|---|---|
| Spec | `sdd/02-infraestructura/I-120-encendido-apagado` |
| Versión | 0.1.0 |
| Estado | **IMPLEMENTADO_LOCAL — NO DESPLEGADO** |
| Gate | `APRUEBO_GATE_I120_LOCAL` (2026-11-09) |
| Ejecución AWS | `AWS_NOT_RUN` · `TWO_NODES_AWS_NOT_VERIFIED` |

> Ningún contrato se declara verificado contra AWS. Todo lo que sigue es
> implementación y prueba **local** con fixtures sintéticos.

## 1. Resumen

| Contrato | Implementación | Pruebas | Estado |
|---|---|---|---|
| INF-120-01 Estados | `MAQUINA_ESTADOS_RUNTIME.md` + `transicion()` en el operador | 83 pruebas de operación | `IMPLEMENTADO_LOCAL` |
| INF-120-02 Reanudación | `reanudar-aux` con 7 prerrequisitos y forecast | `test_operacion_avanzada` | `IMPLEMENTADO_LOCAL` |
| INF-120-03 Suspensión auxiliar | `suspender-aux` con deny por defecto | `test_nexum_operacion` + avanzadas | `IMPLEMENTADO_LOCAL` |
| INF-120-04 DNS/ALB/continuidad | guardas declarativas en `modules/operacion` | contrato Terraform (36) | `IMPLEMENTADO_LOCAL` |
| INF-120-05 Seguridad operacional | DRY_RUN, destroy separado, alcance de plan | avanzadas + contrato | `IMPLEMENTADO_LOCAL` |
| INF-120-06 Pruebas de operación | 20 categorías cubiertas | 83 + 36 + 18 FinOps | `IMPLEMENTADO_LOCAL` |

## 2. INF-120-01 — Estados

- Seis estados: `OFF`, `STARTING`, `ON_2_HEALTHY`, `DRAINING`, `ERROR`, `COST_REVIEW`.
- `OFF` se clasifica como estado **auxiliar** (`es_estado_auxiliar`); si el estado
  observado del core es `OFF`, el operador lo marca `off_sobre_core=true` y lo trata
  como incidente, nunca como ahorro.
- Registro obligatorio por transición: estado anterior, estado nuevo, actor,
  timestamp UTC, entorno, recurso o grupo, motivo, identificador de operación,
  autorización aplicable, resultado, evidencia y referencia de rollback.
- Transiciones inválidas rechazadas (`OFF → ON_2_HEALTHY` incluida); repetición
  idempotente; sin actor o runbook no hay transición.
- Pruebas: `test_registro_completo_para_inf_120_01`, `test_off_no_es_estado_del_core`,
  `test_estado_observado_off_marca_incidente`, `test_transiciones_validas`,
  `test_off_no_salta_a_operativo`, `test_transicion_exige_actor_y_runbook`.

## 3. INF-120-02 — Reanudación segura

`reanudar-aux` exige frase `APRUEBO_REANUDAR_AUX_<ENTORNO>` y verifica:

1. autorización del ambiente; 2. inventario y allowlist; 3. créditos conocidos;
4. estado de cuenta (`plan_activo`); 5. forecast frente al crédito restante;
6. dependencias sin impacto `BLOQUEANTE`; 7. state preservado; 8. rollback disponible.

Sin créditos o sin forecast el resultado es `BLOQUEADO` (código 1). Dos targets
sanos **no** autorizan encender ni apagar el core: solo determinan si el servicio
es publicable (`dos_nodos.alcance = disponibilidad_global_del_backend`).

## 4. INF-120-03 — Suspensión auxiliar

`suspender-aux` aplica, en este orden: etiqueta `nexum:criticality=auxiliary`,
allowlist, coherencia de clasificación, impacto declarado, alcance del plan y
autorización. Cualquier fallo **deniega** (código 2). Nunca acepta un recurso
core aunque se pida por nombre.

Pruebas negativas con fixtures: EC2 core, ALB core, Redis, DynamoDB, CloudFront,
state, auxiliar autorizado, recurso sin etiqueta, recurso desconocido y auxiliar
con impacto `BLOQUEANTE` (`test_core_declarado_cubre_lo_intocable`,
`test_no_se_puede_suspender_core`, `test_recurso_no_declarado_se_deniega`,
`test_impacto_bloqueante_rechaza_suspension`).

## 5. INF-120-04 — DNS, ALB y continuidad

Guardas declarativas, sin cambios de DNS ni certificados:

- el core incluye `alb-interno` y `cloudfront-distribucion` con `suspendible=false`;
- cualquier cambio de plan sobre ellos se rechaza (`cambios_sobre_core`);
- capacidad menor que 2 se rechaza en Terraform y en el operador;
- la allowlist no contiene el ALB, CloudFront ni el certificado.

`TWO_NODES_AWS_NOT_VERIFIED`: la comprobación real de 2 targets queda pendiente.

## 6. INF-120-05 — Seguridad operacional

| Capacidad | Estado local |
|---|---|
| `status` / `estado` | disponible (solo lectura de archivo observado) |
| `plan` | disponible (plan local) |
| `apply` | **`EJECUCION_DESHABILITADA`**, código 2 |
| `suspend-aux` | plan únicamente (`dry_run`) |
| `resume-aux` | plan únicamente |
| `reconcile` | solo lectura y detección de drift |
| `rollback` | plan; nunca destruye datos ni state |
| `destroy-aux` | plan; jamás alcanza el core |
| `destroy-all` / `destroy-data` / `destroy-state` | **no existen** |

- `--force` y `--yes` se rechazan explícitamente: la autorización exige Gate con
  procedencia verificable, no un booleano.
- `cargar_autorizacion` valida frase, entorno, operación y aprobador, y devuelve
  `habilitacion_ejecucion = DENEGADA_EN_I120_LOCAL`: un archivo local no habilita
  efectos reales.
- Separación de responsabilidades prevista: `read-only`, `plan`, `apply-aux`,
  `apply-core`. No se crean roles nuevos: I-060/I-090 no se modifican y el
  contrato queda registrado como dependencia.

## 7. INF-120-06 — Pruebas de operación

| # | Categoría | Prueba |
|---|---|---|
| 1 | Transiciones válidas e inválidas | `MaquinaDeEstados` |
| 2 | Suspensión auxiliar autorizable | `test_ventana_declarada_permite_suspension` |
| 3 | Denegación de core | `test_no_se_puede_suspender_core` |
| 4 | Recursos sin etiqueta | `test_etiqueta_incoherente_se_deniega` |
| 5 | Allowlist incorrecta | `test_auxiliar_fuera_de_allowlist_se_deniega` |
| 6 | Impacto indirecto | `test_impacto_bloqueante_rechaza_suspension` |
| 7 | Idempotencia | `test_suspender_dos_veces_es_idempotente` |
| 8 | Reintentos | `test_reintento_recupera_fallo_transitorio` |
| 9 | Fallos parciales | `test_fallo_parcial_reporta_rollback` |
| 10 | Reconciliación | `test_convergente_sin_drift` |
| 11 | Drift sintético | `test_core_ausente_es_drift` |
| 12 | Rollback simulado | `test_rollback_preserva_datos_y_state` |
| 13 | Preservación de datos | `test_rollback_nunca_destruye_datos` |
| 14 | Preservación de state | `test_snapshot_con_destroy_datos_se_rechaza` |
| 15 | Créditos insuficientes | `test_umbrales_de_la_spec` |
| 16 | Bloqueo por costo proyectado | `test_forecast_excedido_bloquea_despliegues` |
| 17 | Dos targets simulados | `test_dos_nodos_simulados` |
| 18 | Ausencia de autorización | `test_encender_sin_aprobacion_se_deniega` |
| 19 | Manipulación de planes | `AlcanceDelPlan` + `test_destroy_aux_con_plan_sobre_core_se_deniega` |
| 20 | Concurrencia y duplicados | `test_concurrencia_estable` |

## 8. Dependencias externas y bloqueos

| Elemento | Estado |
|---|---|
| Restauración y verificación de backups | **`BLOCKED_I130`** (I-130 `NOT_AUTHORIZED`) |
| Ejecución real y 2 targets | `AWS_NOT_RUN`, `TWO_NODES_AWS_NOT_VERIFIED` |
| Créditos, costo real, cuenta, SCP | `NOT_VERIFIED` |
| Secretos de bootstrap | `RFC-004` `PROPUESTO` |
| TLS del borde | `H-01` abierto |
| Notificación de alarmas | `RFC-007` (`BLOCKED_RFC007_SNS_KMS`) |
| Roles `apply-aux`/`apply-core` | contrato registrado; sin crear (I-060/I-090) |

## 9. Invariantes verificadas por prueba

1. El core nunca aparece en `suspendibles` ni en un plan aceptado.
2. `OFF` no es estado válido del core 24×7.
3. Ningún flag habilita ejecución real.
4. No existe `destroy-all`, `destroy-data` ni `destroy-state`.
5. El dinero se calcula con `Decimal` y nunca se declara costo cero.
6. Sin datos financieros no se autoriza reanudar auxiliares.
