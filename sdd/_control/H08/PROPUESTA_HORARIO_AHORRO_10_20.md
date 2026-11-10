# Propuesta de ventana de operación 10:00–20:00 (America/Mexico_City)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/PROPUESTA_HORARIO_AHORRO_10_20.md` |
| Fecha | 2026-11-10 |
| Estado | **PROPUESTA — NO APROBADA, NO IMPLEMENTADA** |
| Alcance | Ahorro de créditos en partidas facturadas por hora |
| Autoridad | Requiere RFC propio + Gate humano. **No** cambia nada por sí sola |

> Esta propuesta **no** modifica el compromiso 24×7 registrado en I-000, no altera Terraform,
> no cambia `ventana_mantenimiento_activa` y no apaga nada. Su implementación exigiría un RFC
> (cambio de compromiso de disponibilidad) y un Gate de operación independiente.

## 1. Objetivo

Reducir el consumo de créditos apagando el **cómputo** fuera del horario de uso, manteniendo
encendido y persistente todo lo que sostiene datos, estado y artefactos.

## 2. Ventana propuesta

| Parámetro | Valor |
|---|---|
| Zona horaria | `America/Mexico_City` (UTC−6) |
| Encendido | 10:00 |
| Apagado | 20:00 |
| Horas/día | 10 |
| Reducción de horas facturables | 14 h/día = **58.3 %** de las partidas por hora |

## 3. Qué se apagaría (solo con Gate de operación)

| Componente | Acción | Efecto |
|---|---|---|
| ASG del backend (2 × `t4g.micro`, `cpu_credits = standard`) | `desired = 0` / `min = 0` por ventana | Detiene la facturación por instancia-hora |
| Tráfico de borde | CloudFront se mantiene; el origen devolverá error mientras el core esté apagado | Requiere aceptar indisponibilidad |

## 4. Qué NO se apagaría nunca

| Componente | Motivo |
|---|---|
| Bucket de state S3 + versionado | Custodia del state; `prevent_destroy` |
| DynamoDB `NEXUM_CORE_<ENV>` | Datos; facturación on-demand, no por hora; apagarla no ahorra y arriesga integridad |
| Secretos y configuración | Necesarios para reconstruir el runtime |
| Artefactos de despliegue (S3) | El bootstrap del nodo los exige al arrancar |
| Alarmas, presupuesto y logs | Detección de incidentes y evidencia |

## 5. Ahorro (método, no cifra inventada)

El ahorro es exactamente la fracción de las partidas **por hora** del core que deja de consumirse:
`(24 − 10) / 24 = 58.3 %`. La cifra en USD debe recalcularse con el catálogo `VERIFIED_AWS` de
I-000 y el modelo de `infra/finops/`, **no** estimarse aquí. Partidas ya residentes (state,
DynamoDB, secretos) **no** se reducen con este horario.

## 6. Riesgos y condiciones

| Riesgo | Consecuencia | Mitigación exigida antes de aprobar |
|---|---|---|
| Android consume E001/E002 fuera de la ventana | 503 / error de red para el cliente | Aceptación humana explícita; horario comunicado; reintentos del cliente |
| Health checks y alarmas durante el apagado | Alarmas de targets < 2 y readiness disparando en cada ciclo | Silenciar por ventana o ajustar umbrales con justificación |
| `instance refresh` o despliegue durante la ventana | Despliegue sin capacidad | Prohibir despliegues fuera de ventana |
| Cambio del compromiso 24×7 de I-000 | Contradice el escenario A aprobado | **RFC obligatorio** con decisión humana |
| Pruebas de Android incompletas | Apagar antes de terminar bloquea la validación | No aplicar hasta que las pruebas de Android terminen |

## 7. Condición de activación

Ninguna. Este documento es informativo y no habilita acciones. La activación requiere:
RFC aprobado → Gate de operación → ejecución del operador autorizado → evidencia.
