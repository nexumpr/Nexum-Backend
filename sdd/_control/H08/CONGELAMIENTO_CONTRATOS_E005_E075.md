# Congelamiento contractual E005–E075

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/CONGELAMIENTO_CONTRATOS_E005_E075.md` |
| Fecha | 2026-11-16 |
| Baseline contractual | `sdd/03-endpoints/openapi/nexum-api-v1.{yaml,json}` (H06, 75 operaciones) |
| SHA de referencia del repositorio | `a26ae8f27e93e3d8550dbd76d6e2c662e08ab271` (PR #65) |
| Registro por endpoint | `sdd/_control/H08/CLASIFICACION_E005_E075.json` |
| Decisiones | `sdd/_control/H08/DECISIONES_CONTRATO_E005_E075.json` |
| RFC aplicable | `RFC-016-CONTRATOS_E005_E075.md` (**PROPUESTA**) |
| **`CONTRATOS_E005_E075_FROZEN`** | **`NOT_VERIFIED`** |

> Este documento **no congela nada por existir**. Registra el estado real y qué falta.
> Ningún contrato se declara congelado sin aprobación humana y enmiendas aplicadas.

---

## 1. Resultado de la reconciliación

```
71 operaciones · 812 diferencias · 0 sin clasificar
ESENCIAL=1 · COMPATIBILIDAD=20 · OPCIONAL=784 · OBSOLETA=7
```

- **0 diferencias sin clasificar** (requisito de cierre cumplido).
- **1 ESENCIAL**: `hasEnoughData` en E055.
- **20 COMPATIBILIDAD**: 5 HTTP + 1 E006 + 2 E055 + 12 E058 + ... (detalle en el JSON).
- **7 OBSOLETA**: referencias `SESSIONHASH` en E005–E011.
- **784 OPCIONAL POSPUESTAS**: el contrato vigente cubre la función; cada una queda registrada
  con motivo y puede reabrirse por Gate.

## 2. Estado individual de los 71 contratos

`m` = diferencias materiales pendientes (ESENCIAL + COMPATIBILIDAD + OBSOLETA).
Todas las filas están en `CONTRACT_FROZEN=NOT_VERIFIED` porque la aprobación de RFC-016 está
pendiente; el motivo específico se indica con `m`.

| Endpoint | m | Estado | Bloqueo |
|---|---|---|---|
| E005 | 1 | NOT_VERIFIED | A1 persistencia + SES (AWS_READY) |
| E006 | 2 | NOT_VERIFIED | A1 + `token`/`resetToken` |
| E007 | 1 | NOT_VERIFIED | A1 |
| E008 | 1 | NOT_VERIFIED | A1 |
| E009 | 1 | NOT_VERIFIED | A1 |
| E010 | 1 | NOT_VERIFIED | A1 |
| E011 | 1 | NOT_VERIFIED | A1 |
| E012 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E013 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E014 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E015 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E016 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E017 | 1 | NOT_VERIFIED | HTTP `201` vs `200` |
| E018 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E019 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E020 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E021 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E022 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E023 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E024 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E025 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E026 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E027 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E028 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E029 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E030 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E031 | 1 | NOT_VERIFIED | HTTP `204` vs `200` |
| E032 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E033 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E034 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E035 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E036 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E037 | 1 | NOT_VERIFIED | HTTP `200` vs `201` |
| E038 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E039 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E040 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E041 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E042 | 1 | NOT_VERIFIED | HTTP `204` vs `200` |
| E043 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E044 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E045 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E046 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E047 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E048 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E049 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E050 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E051 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E052 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E053 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E054 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E055 | 2 | NOT_VERIFIED | A2 `hasEnoughData` + contenedor `items[]` |
| E056 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E057 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E058 | 12 | NOT_VERIFIED | A3 preferencias granulares |
| E059 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E060 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E061 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E062 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E063 | 1 | NOT_VERIFIED | HTTP `202` vs `200` |
| E064 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E065 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E066 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E067 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E068 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E069 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E070 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E071 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E072 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E073 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E074 | 0 | NOT_VERIFIED | RFC-016 pendiente |
| E075 | 1 | NOT_VERIFIED | `transcript` vs `text` |

**Resumen:** 71/71 `NOT_VERIFIED` · 0 congelados · 15 con diferencias materiales · 56 sin
diferencias materiales (solo opcionales pospuestas).

## 3. Qué falta exactamente para congelar

| # | Falta | Instrumento |
|---|---|---|
| 1 | Aprobar la clasificación y las decisiones | `APRUEBO_RFC_016_CONTRATOS_E005_E075` |
| 2 | Aplicar A1 (28 referencias `SESSIONHASH` → modelo real, 7 specs) | `APRUEBO_ENMIENDA_A1_PERSISTENCIA` |
| 3 | Aplicar A2 (`hasEnoughData` en E055, OpenAPI YAML+JSON) | `APRUEBO_ENMIENDA_A2_E055` |
| 4 | Aplicar A3 (preferencias granulares E058) | `APRUEBO_ENMIENDA_A3_E058` |
| 5 | Cerrar las 5 decisiones HTTP como «conservar» en el registro | incluido en (1) |
| 6 | CI documental verde tras aplicar | automático |

Cuando 1–6 estén hechos, los 71 pasan a `CONTRACT_FROZEN=VERIFIED` salvo E005, que seguirá
`CONTRACT_FROZEN=VERIFIED` + `AWS_READY=NOT_READY` por SES.

## 4. Dependencias externas (no bloquean congelar)

| Dependencia | Estado | Bloquea |
|---|---|---|
| SES: identidad verificada | PENDIENTE | despliegue de E005 |
| SES: salida de sandbox | PENDIENTE | despliegue de E005 |
| SES: `ses:SendEmail` | NO EXISTE | despliegue de E005 |
| SES: conectividad desde subred privada | NO DEMOSTRADA | despliegue de E005 |
| SES: coste | NO VERIFICADO | despliegue de E005 |
| ML: artefacto + entrenamiento | NO IMPLEMENTADO | despliegue de E055 |
| Voz: reconocimiento en Android | FUERA del backend | E075 en cliente |

## 5. Decisiones registradas (SES, ML, voz)

Ver `RFC-016` §4–§7, `ML_E055_CONTRATO_Y_DISENO.md` y `CONTRATO_INTEGRACION_ANDROID.md`.
Resumen: SES es el proveedor decidido; K-Means agrupa patrones y la recomendación se redacta de
forma determinista; el reconocimiento de voz ocurre en Android.

## 6. Reglas de cambio futuro

Cualquier cambio material posterior exige: justificación, análisis de compatibilidad
(`comparar_contrato_api.py`), enmienda mínima, Gate específico y prueba de regresión. Los cambios
silenciosos se detectan porque `DECISIONES_CONTRATO_E005_E075.json` fija el valor esperado del
contrato y el CI falla si no coincide.

Baseline reproducible: los SHA-256 de `nexum-api-v1.yaml` y `nexum-api-v1.json` quedan en
`BASELINE_SHA256.json`; cualquier modificación contractual cambia su hash y exige manifiesto nuevo.

## 7. Evidencia de pruebas

| Comprobación | Resultado |
|---|---|
| `validar_h06.py` | `H06_CONTRACT_PASS` (75 operaciones) |
| `verificar_preparacion_e005.py` | `PREPARACION_CONTRATO_PASS` (71 ops, 5 discrepancias, 12 pendientes) |
| `clasificar_contratos_e005.py` | `CLASIFICACION_OK` (812 diferencias, 0 sin clasificar) |
| Pruebas del clasificador | 7 OK |
| Pruebas de contratos | 28 OK |

## 8. Firma

**Sin firma.** Pendiente de `APRUEBO_RFC_016_CONTRATOS_E005_E075` y de las enmiendas A1–A3.
Este documento quedará `CONTRATOS_E005_E075_FROZEN=VERIFIED` solo tras aplicarlas y verificar CI.
