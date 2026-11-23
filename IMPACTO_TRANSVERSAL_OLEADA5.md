# Impacto transversal — Oleada 5

**Estado:** `BLOCKED_TRANSVERSAL`.
**Rama:** `liz-oleada5` (base exacta Oleada 4 `22d4aa9`).
**Fuente normativa:** `Nexum_Guia_Liz_Oleada5_FINAL_E021_E064_E075.md` (sección H). El PDF V6.0.2 queda excluido por decisión humana expresa.

Este documento inventaría los impactos que cruzan otros E-ID anteriores. No autoriza cambios y no debe interpretarse como Gate firmado. Ninguna de las acciones descritas se ejecutó sin autorización puntual.

## 1. E014 — purga durable

| Aspecto | Detalle |
|---|---|
| Origen | E064–E067 crean `REMINDER#<id>` bajo `USER#<sub>` y el satélite `IDEM#REMINDER#<hash>` para idempotencia. |
| Impacto | La purga durable de E014 debe eliminar ambos registros y cualquier índice satélite sin `Scan`, manteniendo el aislamiento por titular y la trazabilidad de borrado. |
| Archivos probablemente afectados | `internal/application/account/*`, `internal/domain/deletion/*`, adaptadores DynamoDB de purga, y pruebas asociadas. |
| Riesgo | Una purga incompleta deja recordatorios huérfanos o claves de idempotencia reutilizables; un `Scan` global viola H06. |
| Cambio exacto propuesto | (PENDIENTE DE GATE) Extender el plan de purga con `REMINDER#` e `IDEM#REMINDER#` mediante `Query` acotada por `USER#<sub>`, respetando ttl/soft-delete existente. |
| Pruebas propuestas | Purga de usuario A sin afectar B; verificación de 0 items `REMINDER#`/`IDEM#REMINDER#`; asegurar que no aparece `Scan` en el plan. |
| AWS | No ejecutado. Requiere Gate antes de tocar tablas, IAM o datos reales. |

## 2. E020 — exportación aprobada

| Aspecto | Detalle |
|---|---|
| Origen | E021 agrega perfil y resumen financiero; E064–E067 incorporan recordatorios con títulos, fechas y zona horaria del usuario. |
| Impacto | Si E020 exporta «todos los datos del usuario», el catálogo de exportación debe declarar explícitamente los recordatorios y sus satélites, con el mismo filtro de propiedad y sin exponer hashes de idempotencia. |
| Archivos probablemente afectados | `internal/application/export/*`, esquema/documentación de exportación y pruebas de privacidad. |
| Riesgo | Exportar el registro de idempotencia o metadatos internos; o no exportar recordatorios y producir una copia incompleta. |
| Cambio exacto propuesto | (PENDIENTE DE GATE) Incluir `Reminder` en el manifiesto de exportación, excluir `IDEM#REMINDER#` y validar que el sujeto autenticado coincide con la partición consultada. |
| Pruebas propuestas | Exportación usuario A vs B; ausencia de claves `IDEM#`; validación de esquema; sin logs con títulos ni correos. |
| AWS | No ejecutado. Requiere Gate antes de modificar contratos de exportación o ejecutar lecturas reales. |

## 3. E072/E073 — contrato de health (bloqueo separado)

La respuesta observable actual de `/api/v1/health/live` y `/api/v1/health/ready` es `{"status":"UP"}` / `{"status":"UP","dependencies":{...}}`, mientras el OpenAPI exige `Health` con `status: ok|degraded`, `service` y `timestamp`. Cambiar el cuerpo, los códigos o el criterio de disponibilidad afectaría ALB/CloudFront, workflows CI/CD, filtros de CloudWatch, runbooks y evidencias históricas. Por tanto se mantiene `CONTRACT_BLOCKED` y no se modifica `HealthRegistry` ni `router.RegisterHealth`.

## 4. Solicitud de Gate

Formato requerido por la guía:

```text
SOLICITO_GATE_TRANSVERSAL_O5:
  origen: E021/E064-E067
  destino: E014, E020 (y E072/E073 para el contrato de health)
  archivos: <listar tras aprobación de alcance>
  cambios_exactos: <diff propuesto>
  riesgos: purga incompleta, exportación incompleta o fuga de metadatos, ruptura de probes
  pruebas: unitarias + integración local + HTTP real + validadores H06/SDD/IaC
  aws: NO ejecutado; se solicita autorización separada
```

Hasta que el propietario firme ese Gate, este documento se mantiene como `BLOCKED_TRANSVERSAL` y no se declara cierre 75/75.
