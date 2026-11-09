# Matriz de contratos INF-130-01..06 — I-130

| Campo | Valor |
|---|---|
| Spec | `sdd/02-infraestructura/I-130-backup-recuperacion` |
| Versión | 0.1.0 |
| Estado | **IMPLEMENTADO_LOCAL — NO DESPLEGADO** |
| Gate | `APRUEBO_GATE_I130_LOCAL` (2026-11-09) |
| Verificación AWS | `RESTORE_AWS_NO_VERIFICADO` · `AWS_NOT_RUN` |

## 1. Resumen

| Contrato | Implementación | Pruebas | Estado |
|---|---|---|---|
| INF-130-01 RTO/RPO | `objetivos` del inventario + `validar_objetivos` | `ObjetivosRTOyRPO` | `IMPLEMENTADO_LOCAL`, `OBJETIVO_PROPUESTO_NO_MEDIDO` |
| INF-130-02 Datos | activos PITR/on-demand, retención ≤ 35 d | `Costes`, `InventarioDeActivos` | `IMPLEMENTADO_LOCAL` |
| INF-130-03 State | versionado + cifrado + `prevent_destroy` como preconditions | `ModuloDeRespaldo` (contrato) | `IMPLEMENTADO_LOCAL` |
| INF-130-04 Restore | solo tabla nueva aislada + invariantes sintéticas | `PlanDeRestauracion`, `InvariantesSinteticas` | `IMPLEMENTADO_LOCAL`, `RESTORE_AWS_NO_VERIFICADO` |
| INF-130-05 Privacidad | borrado diferido documentado, sin afirmaciones falsas | `BorradoDeCuenta`, `MATRIZ_RETENCION.md` | `IMPLEMENTADO_LOCAL` |
| INF-130-06 Runbook | `PRUEBAS_RESTORE.md` §2 | `DocumentosObligatorios` | `IMPLEMENTADO_LOCAL` |

## 2. INF-130-01 — RTO/RPO

RTO 4 h y RPO 24 h propuestos. `validar_objetivos` exige valores positivos y marca
`OBJETIVO_PROPUESTO_NO_MEDIDO` mientras `medido=false`. El estado `MEDIDO` solo se alcanza con restore
real y duración registrada, que este Gate no ejecuta.

## 3. INF-130-02 — Datos

Cuatro activos declarados: tabla DynamoDB (PITR), backup on-demand opcional, bucket de state y
secretos (`BLOQUEADO_RFC004`). La retención se valida contra el límite real de PITR (35 d). El coste se
calcula con `Decimal` y separa residente de opcional; `secretsmanager` no se computa.

## 4. INF-130-03 — State

`modules/respaldo` exige simultáneamente `state_versionado`, `state_cifrado` y
`state_proteccion_borrado`. Los tres son `true` en el entorno y cualquier `false` detiene el plan. I-070
sigue siendo la autoridad del bucket; aquí solo se verifica la capacidad de recuperación declarada.

## 5. INF-130-04 — Restore

`plan_restauracion` exige `tabla_origen`, `tabla_destino` y `punto_restauracion`; el destino debe
cumplir `^NEXUM_CORE_RESTORE_[a-z0-9]{4,20}$`, no coincidir con el origen, no ser un activo del
inventario y no pedir `sobreescribir`. Terraform rechaza un patrón que alcance la tabla activa.
`verificar_restore_sintetico` comprueba `saved = opening + Σ allocations` con `Decimal` y detecta
asignaciones negativas. Resultado: `PLAN_LOCAL_VALIDADO` + `RESTORE_AWS_NO_VERIFICADO`.

## 6. INF-130-05 — Privacidad

`evaluar_borrado_cuenta` devuelve `BORRADO_DIFERIDO_DOCUMENTADO` y rechaza (código 2) afirmar borrado
inmediato de copias o borrado de copias inmutables. `MATRIZ_RETENCION.md` documenta retención y
caducidad por activo sin inventar plazos legales.

## 7. INF-130-06 — Runbook

`PRUEBAS_RESTORE.md` §2 fija la secuencia: verificar PITR → crear tabla nueva etiquetada → comparar
recuentos/checksums → validar invariantes → registrar duración real → eliminar la tabla de
verificación con Gate.

## 8. Guardas y capacidades inexistentes

| Capacidad | Estado |
|---|---|
| `destroy-data`, `destroy-state`, `destroy-all` | **no existen** (código 2) |
| `restore-in-place`, `restaurar-sobre-produccion` | **no existen** (código 2) |
| `borrado_copias` / `permitir_destroy_datos` en Terraform | validación permanente en `false` |
| Backup on-demand declarativo en Terraform | **no existe** en el proveedor: procedimiento fuera de IaC |

## 9. Dependencias y bloqueos

| Elemento | Estado |
|---|---|
| I-040 (tabla, PITR) | integrado |
| I-070 (state versionado) | integrado |
| I-120 (`impacto_core`) | integrado; su `BLOCKED_I130` **no** se cierra con simulación |
| RFC-004 (secretos) | `PROPUESTO` |
| H-01 (TLS), RFC-006, RFC-007 | abiertos |
| Restore real y RTO/RPO medidos | `NOT_RUN` (requiere Gate de despliegue) |
| Discrepancia 2 GB (spec §8) vs 3 GB (inventario) | registrada, pendiente de ratificación humana |
