# Gate H08-I / I-130 — Respaldo y recuperación (LOCAL)

**FRASE HUMANA REGISTRADA:** `APRUEBO_GATE_I130_LOCAL` (2026-11-09, responsable del proyecto).

**ESTADO:** `AUTHORIZED_I130_LOCAL` — implementación **exclusivamente local**. No se autoriza
`terraform plan/apply/destroy`, ninguna mutación AWS, ningún despliegue ni DAST.

## 1. Alcance autorizado

Implementar y probar en local, sobre la rama `feat/h08i-i130-backup`, la spec
`sdd/02-infraestructura/I-130-backup-recuperacion`:

- módulo `infra/terraform/modules/respaldo` con guardas que fallan cerrado (sin recursos AWS);
- inventario declarativo `infra/terraform/respaldo/inventario_backup.json`;
- verificador `herramientas/verificar_backup_restore.py` (estado, plan de restauración, invariantes
  sintéticas y borrado de cuenta);
- documentos `POLITICA_BACKUPS.md`, `PRUEBAS_RESTORE.md`, `MATRIZ_RETENCION.md` y `MATRIZ_INF_130.md`;
- pruebas de contrato, adversas y de coste; commit, push del rama, PR y seguimiento de CI.
  **Merge requiere autorización humana separada.**

## 2. NO autorizado

- `terraform plan`, `apply`, `destroy`, `import`, `state` y backend remoto.
- Cualquier llamada AWS, incluida la de solo lectura en esta corrida.
- Crear backups, tablas de restauración o snapshots reales.
- Cost Explorer y facturación.
- Cambiar IAM, SCP, OIDC, presupuesto o plan Free.
- Despliegue, DAST y declaración de `RESTORE_VERIFIED` o `RTO_RPO_MEDIDO`.
- Modificar `spec.md`/`plan.md` de I-130 ni ninguna otra spec.
- I-140/I-150 y H08-E: siguen `NOT_AUTHORIZED`.

## 3. Decisiones de diseño aplicadas

1. **PITR obligatorio**: sin PITR no se declara restauración a un punto en el tiempo (INF-130-02).
2. **Restauración solo en tabla nueva aislada**: patrón `^NEXUM_CORE_RESTORE_[a-z0-9]{4,20}$`; se
   rechaza la tabla activa, cualquier activo del inventario y `sobreescribir`.
3. **State recuperable**: versionado, cifrado y `prevent_destroy` exigidos a la vez (INF-130-03).
4. **Retención realista**: ≤ 35 días (límite de PITR); no se promete más de lo que AWS conserva.
5. **Sin capacidades destructivas**: `destroy-data`, `destroy-state`, `destroy-all` y
   `restore-in-place` no existen como comandos; en Terraform son variables con validación permanente
   en `false`.
6. **Nombre del modulo `respaldo`**: `.gitignore` excluye `**/backup/`; para no debilitar ese
   control se renombro el directorio a `infra/terraform/modules/respaldo` en lugar de anadir una
   excepcion al `.gitignore`.
7. **Backup on-demand fuera de IaC**: el proveedor AWS no expone `aws_dynamodb_backup`; se documenta
   como procedimiento autorizado y el módulo solo declara guardas.
7. **RTO/RPO no medidos**: se declaran propuestos; `MEDIDO` exige restore real.
8. **Dinero exacto**: `Decimal`, mes de 730 h, residente y opcional separados, restore por GB.

## 4. Entregables verificables

| Artefacto | Contenido |
|---|---|
| `infra/terraform/modules/backup/` | `terraform_data` con 6 preconditions y validaciones permanentes |
| `infra/terraform/envs/h08i/backup.tf` | cableado con PITR de I-040 y state de I-070 |
| `infra/terraform/respaldo/inventario_backup.json` | objetivos, política, 4 activos y reglas de restore |
| `herramientas/verificar_backup_restore.py` | 5 comandos, códigos 0/1/2, sin AWS |
| `tests/infra/backup/` | 39 pruebas adversas |
| `tests/infra/terraform/test_backup_contrato.py` | 20 pruebas de contrato estático |
| `POLITICA_BACKUPS.md`, `PRUEBAS_RESTORE.md`, `MATRIZ_RETENCION.md`, `MATRIZ_INF_130.md` | diseño, runbook, retención y matriz |

## 5. Verificación exigida y evidencia

| Comprobación | Resultado |
|---|---|
| `terraform fmt -check -recursive infra/terraform` | PASS |
| `terraform init -backend=false` + `validate` (bootstrap + envs) | PASS |
| Suite `tests/infra/backup` | 39 OK |
| Suite `tests/infra/terraform` (incluye contrato I-130) | PASS |
| Trivy HIGH/CRITICAL sobre `infra/terraform` | PASS (sin excepciones nuevas) |
| Validador SDD integral + SHA-256 | PASS |
| Restore real y medición de RTO/RPO | **`NOT_RUN`** |
| Coste observado | **`NOT_VERIFIED`** |

## 6. Bloqueos y dependencias externas

- **RFC-004** (`PROPUESTO`): los secretos no se declaran respaldados ni rotados.
- **H-01**: TLSv1 por certificado por defecto de CloudFront; sin cambios.
- **RFC-006** y **RFC-007**: excepción WAF temporal y notificación de alarmas bloqueada.
- **`AWS_SCP_NOT_VERIFIED`**, cuenta y créditos `NOT_VERIFIED`.
- **`BLOCKED_I130` de I-120** sigue abierto: solo se cierra con un restore verificado, no con este
  Gate local.
- **Discrepancia 2 GB (spec §8) vs 3 GB (inventario)**: registrada, pendiente de decisión humana.

## 7. Criterio de cierre local

I-130 podrá declararse `IMPLEMENTADO_LOCAL — NO DESPLEGADO` cuando: las guardas existan y fallen
cerrado, la restauración no pueda alcanzar la tabla activa, los objetivos RTO/RPO se declaren no
medidos, los costes sean exactos y separados, Terraform valide, Trivy pase, las suites locales pasen y
el SDD/hashes permanezcan íntegros.

**No se declara** `AWS_DEPLOYED`, `RESTORE_VERIFIED`, `RTO_RPO_MEDIDO` ni `FINOPS_GARANTIZADO`.

## 8. Registro

| Fecha | Hecho |
|---|---|
| 2026-11-09 | `APRUEBO_GATE_I130_LOCAL` concedido; rama `feat/h08i-i130-backup` creada desde `main` `31d2512` (incluye el cierre documental de I-120) |
| 2026-11-09 | Implementación local completada; verificación, PR y CI por el agente; merge pendiente de autorización humana ligada a PR + HEAD |
