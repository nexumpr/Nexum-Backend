# Matriz de incidentes — I-140

| Campo | Valor |
|---|---|
| Spec | `I-140-operacion` |
| Versión | 0.1.0 |
| Estado | **IMPLEMENTADO_LOCAL — NO DESPLEGADO** |
| Ratificación de tiempos | `PENDIENTE_DE_RATIFICACION_HUMANA` |
| Ejecución | `NOT_RUN` |

> Los tiempos son una **propuesta operativa**, no un SLA ratificado. El escalamiento es humano y nunca
> autoriza por sí mismo cambios destructivos ni ajustes de facturación.

## 1. Severidades

| Nivel | Descripción | Notificación | Escalamiento | Rollback |
|---|---|---|---|---|
| **S1** | Pérdida o filtración de datos, o servicio caído por completo | 15 min | Responsable + Seguridad | LT/versión previa; nunca destruir datos |
| **S2** | Degradación parcial (1 nodo, Redis sin caché, latencia alta) | 60 min | Responsable de guardia | LT previa o degradación controlada |
| **S3** | Aviso sin impacto funcional (alarma informativa, warning) | 240 min | Cola del equipo | No aplica |
| **S4** | Mejora o deuda técnica detectada | siguiente día hábil | Backlog | No aplica |

`S4` es registro de backlog y **no** dispara procedimiento de incidente.

## 2. Reglas de clasificación

1. Si hay datos personales o financieros comprometidos, es **S1** aunque el servicio responda.
2. Si el servicio responde con menos de 2 targets sanos, es **S2** como mínimo (GOV-02).
3. Una alarma de presupuesto **no** es S1: se trata como S2 de FinOps y nunca apaga el core.
4. Reclasificar un incidente exige constancia del motivo y del actor.

## 3. Flujo

`detección → clasificación → notificación → contención → diagnóstico → mitigación → rollback si aplica
→ verificación → cierre con evidencia → acciones preventivas`

Cada paso registra hora, actor, evidencia y estado. Sin evidencia no hay cierre.

## 4. Límites

- No se declara ningún incidente real: `NOT_RUN`.
- La notificación automática de alarmas sigue bloqueada (`RFC-007`).
- Esta matriz no sustituye un SLA ni una obligación contractual.
