# Política de respaldos — Nexum-Backend (I-130)

| Campo | Valor |
|---|---|
| Spec | `sdd/02-infraestructura/I-130-backup-recuperacion` |
| Versión | 0.1.0 |
| Estado | **IMPLEMENTADO_LOCAL — NO DESPLEGADO** |
| Gate | `APRUEBO_GATE_I130_LOCAL` (2026-11-09) |
| Verificación AWS | `RESTORE_AWS_NO_VERIFICADO` · `AWS_NOT_RUN` |

> Documento de diseño implementado y probado en local. No acredita que exista un backup creado, una
> restauración ejecutada ni un RTO/RPO cumplido.

## 1. Activos cubiertos

| Activo | Mecanismo | Retención | Cifrado | Estado |
|---|---|---|---|---|
| `dynamodb-core` (tabla `NEXUM_CORE_<ENT>` de I-040) | PITR | 35 d | SSE administrada por AWS | `DECLARADO_NO_CREADO` |
| `dynamodb-backup-on-demand` | backup on-demand (procedimiento fuera de Terraform) | 30 d | SSE administrada por AWS | `OPCIONAL_NO_CREADO` |
| `terraform-state` (bucket de I-070) | versionado + SSE-S3 + `prevent_destroy` | 30 d | SSE-S3 | `DECLARADO_NO_CREADO` |
| `secretos-bootstrap` | rotación (RFC-004) | — | KMS administrada | `BLOQUEADO_RFC004` |

El proveedor AWS **no expone** un recurso declarativo `aws_dynamodb_backup`; por eso el backup
on-demand es un procedimiento autorizado fuera de Terraform y el módulo solo declara guardas. No se
crea nada en este Gate.

## 2. Objetivos RTO/RPO

| Objetivo | Valor propuesto | Estado |
|---|---|---|
| RTO | 4 h | `OBJETIVO_PROPUESTO_NO_MEDIDO` |
| RPO | 24 h (backup diario) | `OBJETIVO_PROPUESTO_NO_MEDIDO` |

Un objetivo **no medido no es un objetivo cumplido**. Para declararlo `MEDIDO` hace falta un restore
real en AWS con duración registrada, y eso exige el Gate de despliegue.

## 3. Reglas de restauración

1. Solo sobre **tabla nueva** cuyo nombre cumpla `^NEXUM_CORE_RESTORE_[a-z0-9]{4,20}$`.
2. Prohibido restaurar sobre la tabla activa o sobre cualquier activo del inventario.
3. Prohibido `sobreescribir`.
4. Etiqueta obligatoria `nexum:purpose=restore-verification`.
5. Verificación por recuentos, checksums e invariantes de saldos con datos sintéticos.
6. La tabla de verificación se elimina solo con Gate y autorización humana.

## 4. Protecciones que fallan cerrado

| Guarda | Dónde | Efecto |
|---|---|---|
| PITR obligatorio | `modules/respaldo` (variable + precondition) | sin PITR no se declara restauración |
| State con versionado + cifrado + `prevent_destroy` | `modules/respaldo` | las tres condiciones simultáneas |
| Patrón de restauración que no alcanza la tabla activa | `modules/respaldo` + `verificar_backup_restore.py` | `PLAN_RECHAZADO` |
| Retención ≤ 35 d | `modules/respaldo` | no se promete más de lo que PITR conserva |
| `destroy_datos` / `borrado_copias` | variables con validación permanente en `false` | capacidad inexistente |
| Destruir datos / restaurar en producción | `verificar_backup_restore.py` | código 2 con mensaje específico |

## 5. Costes (calculados, no medidos)

| Concepto | Valor |
|---|---|
| PITR (`DDB_PITR_GB_MES`, 3 GB) | 0.60 USD/mes |
| Backup on-demand (`DDB_BACKUP_GB_MES`, 3 GB) | 0.30 USD/mes |
| State (`S3_STANDARD_GB_MES`, 3 GB) | 0.069 USD/mes |
| **Residente** | **0.0009 USD/h · 0.9720 USD/45 d** |
| **Opcional (backup)** | 0.0004 USD/h · 0.4320 USD/45 d |
| **Total** | **0.0013 USD/h · 1.4336 USD/45 d** (itemización; el modelo H05 ya contiene estas líneas, incremento 0.0000) |
| Restore de verificación (2 GB) | 0.3000 USD por ejecución |

`DDB_RESTORE_GB` (0.15) y `DDB_BACKUP_GB_MES` (0.10) son `VERIFIED_AWS` del catálogo de I-000.
`AWS_COST_REAL = NOT_VERIFIED`: no se declara gasto cero y los créditos no se asumen disponibles.

## 6. Borrado de cuenta (GOV-07)

- Los **datos activos** se borran.
- Las **copias** siguen su retención documentada hasta caducar.
- **No** se afirma borrado instantáneo de copias inmutables ni borrado inmediato de backups: el
  operador rechaza ambas afirmaciones con código 2.

## 7. Lo que este documento no afirma

- No afirma que exista un backup creado.
- No afirma que PITR esté activo en AWS (se declara desde I-040 y no se verificó).
- No afirma RTO/RPO cumplidos.
- No sustituye `PRUEBAS_RESTORE.md` ni el Gate de despliegue.
