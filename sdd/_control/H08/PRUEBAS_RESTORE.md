# Pruebas de restauración — Nexum-Backend (I-130)

| Campo | Valor |
|---|---|
| Spec | `sdd/02-infraestructura/I-130-backup-recuperacion` |
| Versión | 0.1.0 |
| Estado | **IMPLEMENTADO_LOCAL — NO DESPLEGADO** |
| Verificación AWS | `RESTORE_AWS_NO_VERIFICADO` |
| RTO/RPO | `OBJETIVO_PROPUESTO_NO_MEDIDO` |

> Este documento describe las pruebas **locales** y el procedimiento pendiente en AWS. Ninguna fila
> se declara PASS contra la nube.

## 1. Qué se prueba en local (ejecutado)

| # | Prueba | Evidencia |
|---|---|---|
| 1 | Inventario completo (mecanismo, retención, estado, protección) | `verificar-inventario` → `INVENTARIO_VALIDADO` |
| 2 | Objetivos RTO/RPO declarados y no medidos | `estado` → `OBJETIVO_PROPUESTO_NO_MEDIDO` |
| 3 | Plan de restauración a tabla nueva válida | `plan-restauracion` → `PLAN_LOCAL_VALIDADO` |
| 4 | Restauración sobre la tabla activa rechazada | `DENEGADO` (código 2) |
| 5 | Destino fuera del patrón aislado rechazado | `DENEGADO` |
| 6 | `sobreescribir` rechazado | `DENEGADO` |
| 7 | Destino que es activo del inventario rechazado | `DENEGADO` |
| 8 | Invariante `saved = opening + Σ allocations` | `INVARIANTES_OK` |
| 9 | Invariante rota detectada | `INVARIANTES_ROTAS` (código 1) |
| 10 | Asignación negativa rechazada | `DENEGADO` |
| 11 | Coste de restauración por GB (0.3000 USD por 2 GB) | `plan-restauracion` |
| 12 | Borrado de cuenta sin afirmaciones falsas | `BORRADO_DIFERIDO_DOCUMENTADO` |
| 13 | Borrado inmediato de copias / inmutable rechazado | `DENEGADO` |
| 14 | Comandos destructivos inexistentes | `destroy-data`, `destroy-state`, `destroy-all`, `restore-in-place` → código 2 |
| 15 | El verificador no importa SDK de AWS ni lanza procesos | prueba estática de la fuente |

## 2. Procedimiento de restauración (pendiente de AWS)

```powershell
# 1. Plan local (sin AWS)
py -3.13 herramientas/verificar_backup_restore.py plan-restauracion --solicitud solicitud.json
```

`SOLICITUD` de ejemplo (sin datos reales):

```json
{
  "tabla_origen": "NEXUM_CORE_h08i",
  "tabla_destino": "NEXUM_CORE_RESTORE_h08i",
  "punto_restauracion": "2026-11-09T00:00:00Z",
  "gb_restauracion": "2"
}
```

Secuencia en AWS, **solo tras Gate de despliegue**:

1. Verificar que el PITR cubre el punto solicitado (solo lectura).
2. Crear tabla **nueva** con el nombre autorizado y la etiqueta `nexum:purpose=restore-verification`.
3. Comparar recuentos y checksums contra la tabla activa.
4. Validar invariantes de saldos y aportaciones con datos sintéticos.
5. Registrar **duración real** (base del RTO) y desfase del último punto recuperable (RPO).
6. Eliminar la tabla de verificación solo con Gate y autorización humana.

## 3. Qué falta para declarar RTO/RPO medidos

| Requisito | Estado |
|---|---|
| Ejecución real del restore | `NOT_RUN` (cuesta 0.15 USD/GB y exige AWS) |
| Duración registrada | `NOT_RUN` |
| Comparación contra tabla activa | `NOT_RUN` |
| Aprobación humana del RTO/RPO final | pendiente |

## 4. Límites

- Las pruebas locales usan fixtures y datos sintéticos; no acreditan un restore real.
- `BLOCKED_I130` de I-120 solo se cierra con restore verificado, no con una simulación.
- La restauración de aportaciones divididas se comprueba por invariante aritmética, no por
  comparación con datos productivos.
