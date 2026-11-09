# Matriz RTO/RPO — I-130 (INF-130-01)

| Campo | Valor |
|---|---|
| Spec | `I-130-backup-recuperacion` |
| Versión | 0.1.0 |
| Estado | **IMPLEMENTADO_LOCAL — NO DESPLEGADO** |
| Ratificación | `RTO_RPO_PENDING_HUMAN_APPROVAL` |
| Medición AWS | `NOT_RUN` |

## 1. Las seis capas (no se mezclan)

| Capa | RTO | RPO | Fuente | Estado |
|---|---|---|---|---|
| Objetivo propuesto | 4 h | 24 h | spec INF-130-01 | `PROPUESTO` |
| Objetivo ratificado | — | — | decisión humana | **`PENDIENTE`** |
| Objetivo medido | — | — | restore real con duración registrada | `NO_MEDIDO` |
| Capacidad teórica | depende del tamaño y de crear la tabla | PITR continuo ≤ 35 d; 24 h si solo hay backup diario | documentación AWS | `TEORICA` |
| Simulación local | suma de fases declaradas | pérdida máxima declarada | `verificar_recuperacion.py` | `SIMULADO` (no acredita) |
| Resultado real AWS | — | — | Gate de despliegue | `NOT_RUN` |

**Regla:** una simulación local nunca cambia `objetivo_medido`. Solo un restore real medido puede
declarar cumplimiento.

## 2. Fases medidas (runbook INF-130-06)

`incidente → contención → pausa_escrituras → identificación_respaldo → verificación → revisión_finops
→ gate_humano → restore_aislado → validación_financiera → reconciliación → reanudación → monitoreo → cierre`

`medir-recuperacion` exige las **13 fases**, en orden, con duración no negativa; suma minutos y compara
con el RTO. Además compara `perdida_maxima_horas` con el RPO.

```powershell
py -3.13 herramientas/verificar_recuperacion.py medir-recuperacion --solicitud fases.json
```

Salidas: `CUMPLE_SIMULADO`, `INCUMPLE_SIMULADO` (fase faltante, orden incorrecto, fase desconocida,
duración > RTO o pérdida > RPO).

## 3. Cálculos deterministas cubiertos

| Cálculo | Prueba |
|---|---|
| Duración de recuperación (suma de fases) | `test_trece_fases_dentro_de_rto` |
| Pérdida máxima simulada vs RPO | `test_perdida_supera_rpo` |
| Ventana de respaldo (vigencia) | `VigenciaDeBackup` |
| Tiempo de validación (fase `validacion_financiera`) | `_fases()` |
| Condición de incumplimiento | `test_duracion_supera_rto`, `test_fase_faltante` |
| Recuperación parcial | `test_plan_state_rechazado`, `estado-backup CADUCADO` |

## 4. Lo que falta para declarar cumplimiento

1. Ratificación humana de RTO/RPO (`RTO_RPO_PENDING_HUMAN_APPROVAL`).
2. Restore real en AWS con tabla aislada y duración registrada.
3. Comparación con la tabla activa y reconciliación financiera real.
4. Gate de despliegue independiente.

Sin esos cuatro, este documento no afirma RTO/RPO cumplidos.
