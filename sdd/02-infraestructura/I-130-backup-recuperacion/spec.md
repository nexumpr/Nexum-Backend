# I-130-backup-recuperacion — Respaldo, restauración, RTO/RPO y borrado

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-070, I-040, I-070, I-120 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## 1. Objetivo y definición de terminado

Garantizar la capacidad de recuperar datos financieros y configuración sin impedir solicitudes legítimas de eliminación de cuenta; incorporar costos y pruebas de recuperación reales.

## Autoridad y restricciones

- `constitution.md` (GOV-01..18, R-001..060) y el Gate H04 aprobado explícitamente el **2026-11-05** tienen precedencia; H05 diseña infraestructura, no la ejecuta.
- Arquitectura ratificada: Go (Clean Architecture), AWS, DynamoDB, **Redis real**, **2 instancias simultáneas**, balanceador HTTPS y CI/CD/DevSecOps. Ninguno es demostración opcional.
- El PDF del docente v3 exige Backend cloud con costo cero; el catálogo V6.0.2 enumera 75 rutas propuestas E001..E075 y utiliza stack histórico GCP/MongoDB **descartado**.
- No crear código Go, archivos Terraform `.tf`, pipelines ni endpoints H06 durante H05; `terraform apply/destroy`, `git push`, cambios AWS y facturación están prohibidos hasta fases y aprobaciones autorizadas.
- **Regla de evidencia:** documentos pueden alcanzar DRAFT completo sin acceso cloud; Gate I-GO requiere demostrar **viabilidad operacional y económica**, no asumirla. Campos pendientes no se cambian a PASS ni se solventan con créditos.

## 2. Contrato de infraestructura propuesto

### INF-130-01 — RTO/RPO

Objetivos propuestos para revisión: RPO 24h si backup diario y RTO 4h en laboratorio, ajustados tras experiencia/costeo. No afirmar objetivos logrados sin restore medido.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-130-02 — Datos

Dynamo on-demand backup o PITR si habilitado/financiable; costos por GB y restauración, separación env, tags y retención. No renunciar a restaurabilidad por ahorro sin ADR humano.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-130-03 — State

Respaldo y versionado del backend Terraform aprobado; IAM policies y parámetros de recuperación fuera Git, secretos con proceso de rotación y acceso auditado.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-130-04 — Restore

Recuperar en tabla NUEVA aislada y comparar recuentos/checksums/relaciones sin sobreescribir datos activos; validar movimientos y saldos con invariantes, probar sesion tras recuperación.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-130-05 — Privacidad

Política de retención/borrado de cuenta: backups pueden conservar datos hasta caducar bajo aviso/obligaciones aplicables; inhabilitar acceso y documentar eliminación diferida, sin afirmar borrado instantáneo de copias inmutables.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-130-06 — Runbook

Secuencia incidente→pausa de escrituras→verificación backup→restauración→reconciliación→reanudación, evidence con duración y costo.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

## 3. Dependencias y secuencia

I-000, I-040, I-060, I-070, I-120

## 4. Criterios de aceptación de diseño (Gate H05)

- Inventario de activos con política de backup y costo.
- Restore probado con datos sintéticos y RTO/RPO medidos en H08.
- Backups cifrados y acceso mínimo.
- Borrado cuenta respeta retención documentada.
- Terraform state recuperable sin secretos expuestos.

## 5. Riesgos que impiden una aprobación sin evidencia

- **Riesgo:** PITR/backup adicional de pago. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.
- **Riesgo:** Pérdida de datos al eliminar tabla. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.
- **Riesgo:** Restore incorrecto de aportaciones divididas. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.

## 6. Entregables y referencias

Los siguientes nombres identifican evidencias/artefactos **futuros** de I-GO y de implementación; no son archivos creados en H05: `POLITICA_BACKUPS.md`, `PRUEBAS_RESTORE.md`, `MATRIZ_RETENCION.md`.

## 7. Criterio de bloqueo y aprobación

**BLOCKED** si falta una obligación técnica, contradice alguna spec A aprobada, no puede comprobarse la viabilidad Free/SCP o no hay aceptación humana del riesgo. **DRAFT** significa que el diseño está documentado; nunca equivale a infraestructura aplicada.

## 8. Cierre tecnico-financiero H05 (propuesta v2; aun no aprobada)

- Presupuesto 45d incluye PITR Dynamo 2 GB-mes×1.5 a $0.20/GB-mes, backup adicional 2GB×1.5 a $0.10/GB-mes y 2GB de restauración futura ×$0.15/GB, montos referenciales; retención PITR sujeta a límites de AWS, snapshot/copia validada con datos sintéticos en H08. No inferir que disponer de PITR garantiza borrado instantáneo de respaldo. Dynamo state y copias no se destruyen en OFF.
