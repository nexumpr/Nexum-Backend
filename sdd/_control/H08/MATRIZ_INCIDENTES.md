# Matriz de incidentes — I-140

| Campo | Valor |
|---|---|
| Spec | `I-140-operacion` |
| Versión | 0.2.0 |
| Estado | **IMPLEMENTADO_LOCAL — NO DESPLEGADO** |
| Ratificación de tiempos | `PENDIENTE_DECISION_HUMANA` |
| Ejecución | `NOT_RUN` |

> No se inventan SLA, guardias ni notificaciones operativas. Los tiempos son propuesta y permanecen
> `PENDIENTE_DECISION_HUMANA`; `RFC-007` mantiene la notificación automática bloqueada.

## 1. Severidades

| Nivel | Criterio objetivo | Tiempo de respuesta | Escalamiento |
|---|---|---|---|
| **S1** | Pérdida de servicio total, compromiso de integridad de datos o filtración | `PENDIENTE_DECISION_HUMANA` | Obligatorio: responsable + Seguridad |
| **S2** | Degradación parcial significativa (1 nodo, Redis sin caché, error 5xx sostenido) | `PENDIENTE_DECISION_HUMANA` | Obligatorio: responsable de guardia |
| **S3** | Advertencia o anomalía sin impacto funcional | `PENDIENTE_DECISION_HUMANA` | Cola del equipo |

## 2. Ciclo obligatorio

| Fase | Contenido exigido |
|---|---|
| Detección | Fuente (alarma, reporte, health check) y hora |
| Registro auditable | Identificador, actor y hora de apertura |
| Priorización | Severidad asignada con criterio objetivo |
| Notificación | Canal humano y destinatario |
| Escalamiento | Responsable de la decisión según severidad |
| Contención | Acción que limita el daño sin destruir evidencia |
| Resolución | Cambio aplicado y su autorización |
| Cierre | Evidencia y criterio cumplido |
| Reapertura | Condición que reabre y quién decide |
| Evidencia | Artefactos admisibles sin datos sensibles |

## 3. Reglas de clasificación

1. Datos personales o financieros comprometidos ⇒ **S1**, aunque el servicio responda.
2. Menos de 2 targets sanos ⇒ **S2** como mínimo (GOV-02).
3. Alarma de presupuesto ⇒ **S2 de FinOps**; nunca apaga el core ni cambia el plan.
4. Reclasificar exige motivo y actor registrados.
5. La matriz no admite datos sensibles ni evidencia AWS simulada como real; el validador lo rechaza.

## 4. Límites

- Ningún incidente real se declara: `NOT_RUN`.
- No hay guardia 24×7 contratada ni notificación automática (`RFC-007`).
- Esta matriz no sustituye un SLA ni una obligación contractual.
