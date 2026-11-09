# Matriz FinOps 2 GB vs 3 GB — I-130

| Campo | Valor |
|---|---|
| Spec | `I-130-backup-recuperacion` |
| Versión | 0.1.0 |
| Estado | **IMPLEMENTADO_LOCAL — NO DESPLEGADO** |
| Costo observado | `CONSUMO_REAL = NOT_VERIFIED` |

## 1. Origen de la discrepancia

| Fuente | Cantidad | Concepto exacto |
|---|---|---|
| I-130 §8 | 2 GB | Escenario de referencia del cierre H05 (PITR, backup y restore) |
| I-000 / inventario I-120 | 3 GB | Cantidad ya usada en el presupuesto de 45 días y en el residual |

No son la misma medición: **2 GB** es un escenario ilustrativo de la spec y **3 GB** es la cifra que
arrastra el presupuesto aprobado de I-000. Ninguna de las dos es un consumo observado.

**Decisión local:** se mantienen **3 GB** para no contradecir I-000, y se calcula también 2 GB para que
el responsable elija. Cambiar el compromiso económico exige RFC; no se modifica el catálogo ni la spec.

## 2. Costes por escenario (precios `VERIFIED_AWS` de I-000)

| Partida | Precio | 2 GB | 3 GB |
|---|---|---|---|
| PITR (GB-mes) | 0.20 | 0.4000 | 0.6000 |
| Backup on-demand (GB-mes) | 0.10 | 0.2000 | 0.3000 |
| S3 state (GB-mes) | 0.023 | 0.0460 | 0.0690 |
| **Total mensual** | | **0.6460** | **0.9690** |
| **Total 45 días** (÷730 × 1080) | | **0.9556** | **1.4336** |
| Restore de verificación (una vez) | 0.15/GB | 0.3000 | 0.4500 |

## 3. Separación de estados

| Estado | Significado | Valor |
|---|---|---|
| `ESTIMADO` | cálculo determinista con precios del catálogo | 0.9556 / 1.4336 USD |
| `PRESUPUESTADO` | línea incluida en el presupuesto de 45 días de I-000 | sí (PITR + backup) |
| `VERIFICADO_EN_CUENTA` | precio unitario confirmado por Price List | sí para los SKU usados |
| `CONSUMO_REAL` | facturación observada | **`NOT_VERIFIED`** |

## 4. Impacto en el presupuesto de 45 días

| Concepto | Valor |
|---|---|
| Bruto verificado I-000 (escenario C) | 84.0430 USD |
| Conectividad I-000 | 23.7400 USD |
| Total verificado con margen 10 % | 113.0613 USD |
| Créditos verificados | 120.0000 USD |
| Incremento de I-130 (3 GB) | 1.4336 USD |
| **Total con I-130 (sin reserva de restore)** | **114.4949 USD** ≤ 120.0000 |
| Con un restore de verificación (0.4500) | 114.9449 USD |
| Margen restante | ≈ 5.06 USD |

`BLOCKED_FINOPS` **no** se declara: cabe en el presupuesto verificado. Se advierte que el margen es
estrecho y que cualquier ventana adicional de restore lo consume.

## 5. Costes residuales y durante OFF

- PITR y backup **persisten** aunque el runtime auxiliar esté cerrado: I-120 no apaga el core.
- Recursos temporales de recuperación (tabla de verificación) se facturan por almacenamiento mientras
  existan y deben eliminarse con Gate.
- `AWS_COST_REAL`, `AWS_CREDITS` y `AWS_ACCOUNT_STATUS`: `NOT_VERIFIED`. Los créditos **no** son garantía
  de gasto cero.

## 6. Reproducibilidad

```powershell
py -3.13 herramientas/verificar_backup_restore.py estado          # 3 GB (inventario)
py -3.13 herramientas/verificar_recuperacion.py matriz-rto-rpo    # objetivos
```

La variante 2 GB se obtiene cambiando `cantidad` en el inventario de respaldo **en una copia**, nunca
sobrescribiendo el catálogo ni la spec.
