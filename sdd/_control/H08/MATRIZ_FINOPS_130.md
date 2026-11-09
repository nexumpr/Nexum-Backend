# Matriz FinOps 2 GB vs 3 GB — conciliación I-130

| Campo | Valor |
|---|---|
| Spec | `I-130-backup-recuperacion` |
| Versión | 0.2.0 |
| Estado | **IMPLEMENTADO_LOCAL — NO DESPLEGADO** |
| Conciliación | `herramientas/conciliar_finops_130.py` |
| Presupuesto vigente | I-000: bruto **102.7830**, con margen 10 % **113.0613**, créditos declarados **120.0000** |
| Costo observado | `CONSUMO_REAL = NOT_VERIFIED` |

## 1. Conclusión de la conciliación

El modelo **H05 V3 ya presupuesta** las cuatro líneas relevantes (PITR, backup, restore y S3 del
state) con precios idénticos a los del catálogo:

| Concepto | Modelo H05 V3 | I-130 (3 GB) | Incremento |
|---|---|---|---|
| DynamoDB PITR | 3 GB-mes × 0.20 | 3 GB-mes × 0.20 | **0.0000** |
| DynamoDB backup | 3 GB-mes × 0.10 | 3 GB-mes × 0.10 | **0.0000** |
| S3 Terraform-state | 3 GB-mes × 0.023 | 3 GB-mes × 0.023 | **0.0000** |
| DynamoDB restore | 2 GB × 0.15 (eventual) | no habilitado | **0.0000** |
| **Total** | | | **0.0000 USD** |

El modelo V3 (84.0580 USD) es la base **histórica**; el presupuesto vigente es el de I-000
(102.7830 bruto / 113.0613 con margen). La conciliación usa el vigente y el modelo solo como referencia
por SKU.

**No se añade 1.4336 USD al presupuesto**: esa cifra es la *itemización* de líneas que el modelo ya
contiene. Sumarla otra vez sería doble conteo. `1.4336 USD/45 d` es el importe de las líneas de I-130
consideradas aisladas, no un incremento.

## 2. Qué cambia entre 2 GB y 3 GB

| SKU | 2 GB (45 d) | 3 GB (45 d) | Diferencia |
|---|---|---|---|
| `DDB_PITR_GB_MES` | 0.5918 | 0.8877 | −0.2959 |
| `DDB_BACKUP_GB_MES` | 0.2959 | 0.4438 | −0.1479 |
| `S3_STANDARD_GB_MES` | 0.0681 | 0.1021 | −0.0340 |
| **Total** | **0.9558** | **1.4336** | **−0.4779** |

La diferencia **no es un ahorro nuevo**: es la distancia entre dos escenarios, y el modelo ya usa 3 GB.
Adoptar 2 GB reduciría el modelo en 0.4779 USD/45 d; no se cambia el compromiso sin RFC.

## 3. Separaciones obligatorias

| Concepto | Valor | Estado |
|---|---|---|
| Backup habilitado | 0.4438 USD/45 d | `HABILITADO` (PITR) / `OPCIONAL_NO_HABILITADO` (on-demand) |
| Restore futuro | 0.3000 USD por 2 GB | `EVENTUAL`, no recurrente |
| Recurrente I-130 | 1.4336 USD/45 d | ya presupuestado |
| Incremento real | **0.0000 USD** | — |

## 4. Estado de la evidencia

| Estado | Valor |
|---|---|
| `ESTIMADO` | calculado con `Decimal` desde el catálogo |
| `PRESUPUESTADO` | sí, modelo H05 V3 |
| `VERIFICADO_EN_CUENTA` | **`NOT_VERIFIED`** |
| `CONSUMO_REAL` | **`NOT_VERIFIED`** |

## 5. Decisión

`VIABILIDAD_LOCAL_SIN_EVIDENCIA_DE_CUENTA`. **No** se declara `NO_BLOCKED_FINOPS` como PASS.

| Concepto | Valor |
|---|---|
| Presupuesto vigente con margen | 113.0613 USD |
| Incremento I-130 | 0.0000 USD |
| Créditos declarados | 120.0000 USD |
| **Margen teórico** | **6.9387 USD** |
| ¿Es dinero disponible en AWS? | **No** (`margen_teorico_es_dinero_disponible = false`) |
| Créditos verificados | `null` / `NOT_VERIFIED` |
| Consumo real | `NOT_VERIFIED` |

El margen es aritmética sobre cifras declaradas, no saldo comprobado. Cualquier decisión de despliegue
que dependa de créditos queda bloqueada hasta verificación autorizada.

```powershell
py -3.13 herramientas/conciliar_finops_130.py
```

Código 1 solo si el margen local resultara negativo; 0 en otro caso, con la decisión explícita arriba.

## 6. Lo que no se hace

- No se consulta AWS ni Cost Explorer.
- No se modifica el catálogo ni el modelo H05.
- No se cambian compromisos económicos del SDD.
- No se presenta el crédito declarado como saldo disponible comprobado.
