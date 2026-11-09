# Runbook de restauración — I-130 (INF-130-06)

| Campo | Valor |
|---|---|
| Spec | `I-130-backup-recuperacion` |
| Versión | 0.1.0 |
| Estado | **IMPLEMENTADO_LOCAL — NO DESPLEGADO** |
| Ejecución AWS | `RESTORE_AWS_NOT_RUN` |
| Gate de ejecución | pendiente (Gate de despliegue) |

> Cada fase declara responsable, precondiciones, evidencia, comprobaciones, criterio de éxito, riesgos,
> rollback, tiempo, coste y estado. Ninguna fase se marca ejecutada en AWS en este Gate.

| # | Fase | Responsable | Precondiciones | Evidencia | Criterio de éxito | Riesgos | Rollback | Tiempo (est.) | Coste (est.) | Estado |
|---|---|---|---|---|---|---|---|---|---|---|
| 1 | Incidente | Responsable técnico | Detección de anomalía | Ticket + métricas | Incidente registrado | Diagnóstico erróneo | — | 15 min | 0 | `NOT_RUN` |
| 2 | Contención | Responsable técnico | Fase 1 | Log de acciones | Daño acotado | Contener de más | Reabrir servicio | 20 min | 0 | `NOT_RUN` |
| 3 | Pausa controlada de escrituras | Responsable + Gate | Fase 2; no pertenece a I-130 | Bandera de pausa + aviso | Escrituras detenidas sin corrupción | Bloqueo prolongado | Reanudar | 10 min | 0 | `NOT_RUN` (depende de otra spec) |
| 4 | Identificación del respaldo | Responsable técnico | PITR/backup disponible | `estado-backup` = `DISPONIBLE` | Punto recuperable confirmado | Backup caducado | Elegir otro punto | 15 min | 0 | `NOT_RUN` |
| 5 | Verificación de integridad | Responsable técnico | Fase 4 | Hash/fecha/procedencia | Respaldo íntegro | Respaldo corrupto | Descartar y elegir otro | 20 min | 0 | `NOT_RUN` |
| 6 | Revisión FinOps | finops-guardian | Fase 5 | Coste de restore (0.15 USD/GB) | Coste aprobado | Exceso de presupuesto | Cancelar | 10 min | 0.3000 USD / 2 GB | `NOT_RUN` |
| 7 | Gate humano | Responsable | Fases 4–6 | Autorización con PR/SHA | Autorización registrada | Ausencia de Gate | Detener | 5 min | 0 | `NOT_RUN` |
| 8 | Restore aislado | Responsable técnico | Fase 7 | Tabla nueva `NEXUM_CORE_RESTORE_*` | Tabla creada y etiquetada | Restaurar sobre producción | Eliminar tabla de verificación | 45 min | 0.3000 USD / 2 GB | `NOT_RUN` |
| 9 | Validación financiera | qa-evidence-verifier | Fase 8 | `verificar_integridad_financiera.py` = `INTEGRIDAD_OK` | Invariantes OK | Saldos inconsistentes | Descartar tabla | 30 min | 0 | `SIMULADO` |
| 10 | Reconciliación | Responsable + FinOps | Fase 9 | Conciliación de movimientos y aportaciones | Sin duplicados ni saldos rotos | Duplicados por idempotencia | Reejecutar reconciliación | 30 min | 0 | `NOT_RUN` |
| 11 | Reanudación | Responsable | Fase 10 | Health checks + 2 targets | `ON_2_HEALTHY` | Reanudar con datos incompletos | Volver a pausa | 10 min | 0 | `NOT_RUN` |
| 12 | Monitoreo | devsecops-security | Fase 11 | Alarmas y logs | Sin 5xx anómalos | Falsa sensación de normalidad | Escalar | 30 min | 0 | `NOT_RUN` |
| 13 | Cierre | Responsable | Fases 11–12 | Informe con duración real | RTO/RPO medidos o desviación documentada | Cerrar sin evidencia | Reabrir | 15 min | 0 | `NOT_RUN` |

## Reglas del runbook

1. **Nunca** se restaura sobre la tabla activa ni se deshabilita `deletion protection` para restaurar.
2. La tabla de verificación se elimina solo con Gate y autorización humana.
3. La pausa de escrituras **no** forma parte de I-130: si el mecanismo vive en otra spec, se registra la
   dependencia y su Gate; no se implementa aquí.
4. El coste del restore se aprueba antes de ejecutarlo (`DDB_RESTORE_GB` = 0.15 USD/GB).
5. Una restauración antigua **no** reactiva cuentas eliminadas ni sesiones revocadas (ver
   `POLITICA_PRIVACIDAD_BACKUPS.md`).
6. Cualquier fase puede detenerse sin completar el restore: el criterio es no publicar datos dudosos.

## Simulación local

```powershell
py -3.13 herramientas/verificar_recuperacion.py medir-recuperacion --solicitud fases.json
```

Devuelve `CUMPLE_SIMULADO` o `INCUMPLE_SIMULADO` y **no** modifica el objetivo medido.
