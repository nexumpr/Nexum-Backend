# Matriz de retención — Nexum-Backend (I-130)

| Campo | Valor |
|---|---|
| Spec | `I-130-backup-recuperacion` |
| Versión | 0.1.0 |
| Estado | **IMPLEMENTADO_LOCAL — NO DESPLEGADO** |
| Evidencia | Declarativa; `AWS_NOT_RUN` |

## 1. Retención por activo

| Activo | Retención declarada | Límite técnico | Mecanismo de caducidad |
|---|---|---|---|
| DynamoDB (PITR) | 35 d | **35 d** (máximo de PITR) | AWS descarta automáticamente |
| DynamoDB (backup on-demand) | 30 d | Retención aprobada (sin el límite de 35 d del PITR) | **No caduca solo**: requiere borrado autorizado |
| Terraform state (S3) | 30 d | Versionado sin límite | Lifecycle de versiones no actuales (I-070) |
| Secretos | — | — | Rotación RFC-004 (no aprobada) |

El PITR nunca supera 35 días (límite del mecanismo). El backup on-demand usa la retención aprobada y
**no** se le aplica ese máximo: no caduca automáticamente y su borrado exige procedimiento autorizado.

## 2. Borrado de cuenta frente a retención

| Elemento | Comportamiento | Afirmación permitida |
|---|---|---|
| Datos activos | Borrado inmediato | Sí |
| Copias PITR | Caducan según su ventana | Sí, «retención documentada» |
| Backups on-demand | Persisten hasta borrado autorizado | Sí, «retención documentada» |
| Copias inmutables | **No** se declara borrado instantáneo | No se afirma |
| State | Conserva versiones según lifecycle | Sí |

El operador (`verificar_backup_restore.py borrado-cuenta`) rechaza con código 2 cualquier política
que declare `borrado_inmediato_backups: true` o `afirma_borrado_inmutable: true`.

## 3. Coste de la retención

| Partida | Precio | Uso declarado | USD/mes |
|---|---|---|---|
| PITR | 0.20 USD/GB-mes | 3 GB | 0.6000 |
| Backup on-demand | 0.10 USD/GB-mes | 3 GB | 0.3000 |
| S3 state | 0.023 USD/GB-mes | 3 GB | 0.0690 |
| **Total** | | | **0.9690** |

En 45 días: **1.4336 USD** (0.0009 residente + 0.0004 opcional por hora). Restore de verificación:
0.3000 USD por 2 GB, solo si se ejecuta.

`AWS_COST_REAL`, `AWS_CREDITS` y `AWS_ACCOUNT_STATUS` siguen `NOT_VERIFIED`.

## 4. Discrepancia documentada

La sección 8 de la spec I-130 presupuestó **2 GB** de PITR, backup y restauración; el inventario de
I-000/I-120 usa **3 GB**. Se mantiene 3 GB (cifra ya usada en el residual de I-120) y se registra la
diferencia para que el responsable la ratifique o la corrija en el Gate; no se altera la spec.

## 5. Lo que esta matriz no afirma

- No afirma que las copias existan.
- No afirma que la retención esté configurada en AWS.
- No afirma cumplimiento legal de plazos: no se inventa un plazo distinto del declarado.
