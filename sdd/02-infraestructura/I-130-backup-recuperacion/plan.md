# I-130-backup-recuperacion — Respaldo, restauración, RTO/RPO y borrado

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-070, I-040, I-070, I-120 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## 1. Estrategia de diseño documental de H05

Garantizar la capacidad de recuperar datos financieros y configuración sin impedir solicitudes legítimas de eliminación de cuenta; incorporar costos y pruebas de recuperación reales.

## Autoridad y restricciones

- `constitution.md` (GOV-01..18, R-001..060) y el Gate H04 aprobado explícitamente el **2026-11-05** tienen precedencia; H05 diseña infraestructura, no la ejecuta.
- Arquitectura ratificada: Go (Clean Architecture), AWS, DynamoDB, **Redis real**, **2 instancias simultáneas**, balanceador HTTPS y CI/CD/DevSecOps. Ninguno es demostración opcional.
- El PDF del docente v3 exige Backend cloud con costo cero; el catálogo V6.0.2 enumera 75 rutas propuestas E001..E075 y utiliza stack histórico GCP/MongoDB **descartado**.
- No crear código Go, archivos Terraform `.tf`, pipelines ni endpoints H06 durante H05; `terraform apply/destroy`, `git push`, cambios AWS y facturación están prohibidos hasta fases y aprobaciones autorizadas.
- **Regla de evidencia:** documentos pueden alcanzar DRAFT completo sin acceso cloud; Gate I-GO requiere demostrar **viabilidad operacional y económica**, no asumirla. Campos pendientes no se cambian a PASS ni se solventan con créditos.

## 2. Plan secuencial y verificaciones

### P-130-01 — RTO/RPO

**Entrada:** H04 aprobado, requisitos `A-070, I-040, I-070, I-120`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Objetivos propuestos para revisión: RPO 24h si backup diario y RTO 4h en laboratorio, ajustados tras experiencia/costeo. No afirmar objetivos logrados sin restore medido.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-130-02 — Datos

**Entrada:** H04 aprobado, requisitos `A-070, I-040, I-070, I-120`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Dynamo on-demand backup o PITR si habilitado/financiable; costos por GB y restauración, separación env, tags y retención. No renunciar a restaurabilidad por ahorro sin ADR humano.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-130-03 — State

**Entrada:** H04 aprobado, requisitos `A-070, I-040, I-070, I-120`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Respaldo y versionado del backend Terraform aprobado; IAM policies y parámetros de recuperación fuera Git, secretos con proceso de rotación y acceso auditado.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-130-04 — Restore

**Entrada:** H04 aprobado, requisitos `A-070, I-040, I-070, I-120`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Recuperar en tabla NUEVA aislada y comparar recuentos/checksums/relaciones sin sobreescribir datos activos; validar movimientos y saldos con invariantes, probar sesion tras recuperación.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-130-05 — Privacidad

**Entrada:** H04 aprobado, requisitos `A-070, I-040, I-070, I-120`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Política de retención/borrado de cuenta: backups pueden conservar datos hasta caducar bajo aviso/obligaciones aplicables; inhabilitar acceso y documentar eliminación diferida, sin afirmar borrado instantáneo de copias inmutables.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-130-06 — Runbook

**Entrada:** H04 aprobado, requisitos `A-070, I-040, I-070, I-120`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Secuencia incidente→pausa de escrituras→verificación backup→restauración→reconciliación→reanudación, evidence con duración y costo.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

## 3. Orden de ejecución futura H08 (no ejecutar ahora)

1. Auditar plan AWS/credenciales/precios y límites SCP sin cambios.
2. Preparar revisión de cambios, artefactos IaC y análisis económico en PR autorizado.
3. Ejecutar validaciones (fmt/validate/plan/pruebas), someter al gate humano.
4. Aprovisionar o modificar únicamente después de autorización de release y protección de datos.
5. Ejecutar pruebas reales, registrar métricas, costos y rollback, y no declarar DONE antes de las evidencias.

## 4. Dependencias cruzadas

I-000, I-040, I-060, I-070, I-120

## 5. Riesgos y tratamiento

1. `I-130-backup-recuperacion-R01`: PITR/backup adicional de pago. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.
2. `I-130-backup-recuperacion-R02`: Pérdida de datos al eliminar tabla. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.
3. `I-130-backup-recuperacion-R03`: Restore incorrecto de aportaciones divididas. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.

## 6. Puerta de entrada a H06

Gate H05/I-GO NO se logra con instalación de Markdown: requiere pruebas de viabilidad económica/cuenta, trazabilidad física Dynamo/Redis, controles IAM y aceptación expresa del responsable. Los contratos detallados E001..E075 se formalizan exclusivamente en H06.
