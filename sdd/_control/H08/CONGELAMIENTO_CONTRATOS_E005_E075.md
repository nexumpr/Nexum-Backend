# Congelamiento contractual E005–E075

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/CONGELAMIENTO_CONTRATOS_E005_E075.md` |
| Versión | 3 (revisión 2026-11-16) |
| Baseline contractual | `sdd/03-endpoints/openapi/nexum-api-v1.{yaml,json}` (H06, 75 operaciones) |
| Registro por endpoint | `sdd/_control/H08/CLASIFICACION_E005_E075.json` |
| Decisiones | `sdd/_control/H08/DECISIONES_CONTRATO_E005_E075.json` |
| Norma | `RFC-016-CONTRATOS_E005_E075.md` v3.0.0 (**PROPUESTA**) |
| **`CONTRATOS_E005_E075_FROZEN`** | **`NOT_VERIFIED`** |

> Este documento registra el estado real. No congela nada por existir.

## 1. Reconciliación

```
71 operaciones · 812 diferencias · 0 sin clasificar · 0 pendientes de decisión
ESENCIAL=9 · COMPATIBILIDAD=199 · OPCIONAL=597 · OBSOLETA=7
```

| Clase | N | Endpoints | Bloquea congelar |
|---|---|---|---|
| ESENCIAL | 9 | E010, E011, E040, E055, E059, E065, E066 | **Sí** |
| COMPATIBILIDAD | 199 | 71 (aditivos A8) + E006/E017/E031/E037/E042/E052/E055/E058/E063/E075 | **Sí** |
| OBSOLETA | 7 | E005–E011 (`SESSIONHASH`, 28 refs) | **Sí** |
| OPCIONAL | 597 | varios | No (derivación declarada) |
| PENDIENTE_DE_DECISION | **0** | — | — |

**`DECISION_READY = VERIFIED` en los 71**: toda diferencia tiene decisión adoptada y motivada.
**`CONTRACT_FROZEN = NOT_VERIFIED`**: faltan las enmiendas aprobadas y aplicadas.

## 2. Estado individual de los 71 contratos

`m` = diferencias materiales pendientes (ESENCIAL + COMPATIBILIDAD + OBSOLETA).
Todas las filas están `CONTRACT_FROZEN=NOT_VERIFIED` porque las enmiendas A1–A7 no están aprobadas ni aplicadas.

| Endpoint | m | Bloqueo | Endpoint | m | Bloqueo |
|---|---|---|---|---|---|
| E005 | 1 | A1 + AWS_READY(SES) | E041 | 0 | RFC-016 |
| E006 | 2 | A1 + A6 (época) | E042 | 1 | HTTP |
| E007 | 1 | A1 | E043 | 0 | RFC-016 |
| E008 | 1 | A1 | E044 | 0 | RFC-016 |
| E009 | 1 | A1 | E045 | 0 | RFC-016 |
| E010 | 1 | A1 | E046 | 0 | RFC-016 |
| E011 | 1 | A1 | E047 | 0 | RFC-016 |
| E012 | 0 | RFC-016 | E048 | 0 | RFC-016 |
| E013 | 0 | RFC-016 | E049 | 0 | RFC-016 |
| E014 | 0 | RFC-016 | E050 | 0 | RFC-016 |
| E015 | 0 | RFC-016 | E051 | 0 | RFC-016 |
| E016 | 0 | RFC-016 | E052 | 2 | A5 |
| E017 | 1 | HTTP | E053 | 0 | RFC-016 |
| E018 | 0 | RFC-016 | E054 | 0 | RFC-016 |
| E019 | 0 | RFC-016 | E055 | 2 | A2 |
| E020 | 0 | RFC-016 | E056 | 0 | RFC-016 |
| E021 | 0 | RFC-016 | E057 | 0 | RFC-016 |
| E022 | 0 | RFC-016 | E058 | 12 | A3 |
| E023 | 0 | RFC-016 | E059 | 0 | RFC-016 |
| E024 | 0 | RFC-016 | E060 | 0 | RFC-016 |
| E025 | 0 | RFC-016 | E061 | 0 | RFC-016 |
| E026 | 0 | RFC-016 | E062 | 0 | RFC-016 |
| E027 | 0 | RFC-016 | E063 | 1 | HTTP |
| E028 | 0 | RFC-016 | E064 | 0 | RFC-016 |
| E029 | 0 | RFC-016 | E065 | 0 | RFC-016 |
| E030 | 0 | RFC-016 | E066 | 0 | RFC-016 |
| E031 | 1 | HTTP | E067 | 0 | RFC-016 |
| E032 | 0 | RFC-016 | E068 | 0 | RFC-016 |
| E033 | 0 | RFC-016 | E069 | 0 | RFC-016 |
| E034 | 0 | RFC-016 | E070 | 0 | RFC-016 |
| E035 | 0 | RFC-016 | E071 | 0 | RFC-016 |
| E036 | 0 | RFC-016 | E072 | 0 | RFC-016 |
| E037 | 1 | HTTP | E073 | 0 | RFC-016 |
| E038 | 0 | RFC-016 | E074 | 0 | RFC-016 |
| E039 | 1 | A7 (debe devolver `openingSaved`) | E075 | 1 | `transcript`/`text` |
| E040 | 1 | **A7 ESENCIAL** | | | |

**Resumen:** 71/71 `DECISION_READY=VERIFIED` · **0 congelados** · enmiendas A1–A5, A7 y A8 pendientes de aprobación y aplicación.

## 3. Qué falta exactamente

| # | Falta | Instrumento |
|---|---|---|
| 1 | Aprobar clasificación y decisiones | `APRUEBO_RFC_016_CONTRATOS_E005_E075` |
| 2 | A1 persistencia (7 specs) | `APRUEBO_ENMIENDA_A1_PERSISTENCIA` |
| 3 | A2 `hasEnoughData` (E055) | `APRUEBO_ENMIENDA_A2_E055` |
| 4 | A3 contrato E058 (12 campos) | `APRUEBO_ENMIENDA_A3_E058` |
| 5 | A5 `enoughData`/`minimumSamples` (E052) | `APRUEBO_ENMIENDA_A5_E052` |
| 6 | A7 `openingSaved` (E040/E039) | `APRUEBO_ENMIENDA_A7_E040` |
| 7 | **A8: 38 incorporaciones con schema** (199 candidatos revisados) | `APRUEBO_ENMIENDA_A8_ADITIVOS` |
| 8 | A4 registrar las 5 decisiones HTTP | incluido en (1) |
| 9 | A6 época de revocación (código compartido) | `APRUEBO_RFC_A6_EPOCA_SESION` |
| 10 | A9 secreto de sellado · A10 IAM `QUEUE#*`/`RESET#*` | Gates propios (no bloquean congelar) |
| 11 | CI documental verde tras aplicar | automático |

## 4. Dependencias externas (no bloquean congelar)

| Dependencia | Estado | Bloquea |
|---|---|---|
| SES: remitente real identificado | **PENDIENTE** (no se inventa) | despliegue E005 |
| SES: identidad verificada y salida de sandbox | PENDIENTE | despliegue E005 |
| SES: `ses:SendEmail` en rol runtime | NO EXISTE | despliegue E005 |
| SES: conectividad (interface endpoint) | NO DEMOSTRADA · coste ≈21,60 USD/45 d | despliegue E005 + FinOps |
| SES: preflight de lecturas | **BLOQUEADO por permisos** | diagnóstico SES |
| ML: artefacto entrenado e inferencia | NO IMPLEMENTADO | despliegue E055 |
| A6: migración de época | NO IMPLEMENTADO | implementación E006 |
| Voz: reconocimiento en Android | fuera del backend | E075 cliente |

## 5. Evidencia de pruebas

| Comprobación | Resultado |
|---|---|
| `validar_h06.py` | `H06_CONTRACT_PASS` (75 operaciones) |
| `verificar_preparacion_e005.py` | `PREPARACION_CONTRATO_PASS` (71 ops, 12 decisiones pendientes) |
| `clasificar_contratos_e005.py` | `CLASIFICACION_OK` (812, 0 sin clasificar) |
| Pruebas Python | 256 (1 fallo local esperado por `opencode.json`) |
| Go | `gofmt`/`build`/`vet`/`test` PASS |
| CI | 13/13 PASS |

## 6. Reglas de cambio futuro

Todo cambio material exige justificación, análisis de compatibilidad, enmienda mínima, Gate y
regresión. `DECISIONES_CONTRATO_E005_E075.json` fija el valor esperado y el CI falla si el contrato
no coincide.

## 7. Firma

**Sin firma.** Pendiente de RFC-016 y de A1–A5/A7. El documento pasará a
`CONTRATOS_E005_E075_FROZEN=VERIFIED` solo tras aplicarlas, validar los 71 contratos y verde en CI.
