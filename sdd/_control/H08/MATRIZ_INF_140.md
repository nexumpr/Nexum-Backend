# Matriz de contratos INF-140-01..06 — I-140

| Campo | Valor |
|---|---|
| Spec | `I-140-operacion` |
| Versión | 0.1.0 |
| Estado | **IMPLEMENTADO_LOCAL — NO DESPLEGADO** |
| Ejecución AWS | `NOT_RUN` |

| Contrato | Implementación local | Pruebas | Estado |
|---|---|---|---|
| INF-140-01 Runbooks | `RUNBOOK_OPERATIVO.md` con 10 procedimientos y 8 campos | `validar_runbooks` | `IMPLEMENTADO_LOCAL` |
| INF-140-02 Prioridades | `MATRIZ_INCIDENTES.md` S1–S4 + validador de coherencia | `validar_prioridades` | `IMPLEMENTADO_LOCAL`, tiempos pendientes de ratificación |
| INF-140-03 Drills | `PLAN_PRUEBAS_OPERACION.md` con 5 drills `NOT_RUN` | `validar_drills` | `IMPLEMENTADO_LOCAL` |
| INF-140-04 Prueba y aceptación | inventario derivado del código + validadores SDD | `inventario_desde_codigo` | `IMPLEMENTADO_LOCAL`, AWS `NOT_VERIFIED` |
| INF-140-05 Acceso | sin root, sin SSH público, identidades temporales, SSM en ventana | `validar_acceso` | `IMPLEMENTADO_LOCAL` |
| INF-140-06 Inventario final | módulos y recursos declarados, `existe_en_aws = NOT_VERIFIED` | `inventario_desde_codigo` | `IMPLEMENTADO_LOCAL` |

## Invariantes

1. Ningún runbook se declara ejecutado por estar escrito.
2. Ningún drill se declara ejecutado en AWS desde un archivo local.
3. El inventario se deriva del código; un nombre no prueba existencia.
4. La operación segura no abre SSH público ni usa root.
5. El core 24×7 de I-120 y los datos de I-130 no se tocan en ningún procedimiento.

## Dependencias y bloqueos

| Elemento | Estado |
|---|---|
| I-020, I-030, I-050, I-090, I-110, I-120, I-130 | integradas |
| Notificación de alarmas | `RFC-007` → `BLOCKED_RFC007_SNS_KMS` |
| Agente CloudWatch | pendiente (I-020) |
| Rotación de secretos | `RFC-004` `PROPUESTO` |
| TLS del borde | `H-01` abierto |
| Drills reales | `NOT_RUN` (requieren Gate de despliegue) |
