# I-000-cuenta-finops — Cuenta AWS, plan Free y estudio de costo cero

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | GOV-07, GOV-15, A-000, A-110, R-001..060 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## Gestión de tareas H05 (solo especificación y aceptación de diseño)

Estado inicial de todos los trabajos: `TODO`, salvo evidencia local de integridad H04 documentada por el responsable. No marcar como completado por generar archivos.

### T-000-01 — Cerrar plan y contexto

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y GOV-07, GOV-15, A-000, A-110, R-001..060.
- **Trabajo:** Región de evaluación us-east-2 (Ohio); tomar tipo de cuenta (Free/Paid), fecha de alta, expiración y créditos desde consola o APIs de lectura con evidencia fechada. No cambiar el plan. La cuenta y región reales se seleccionan por entorno, nunca se codifican como secretos.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-000-02 — Cerrar matriz por recurso

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y GOV-07, GOV-15, A-000, A-110, R-001..060.
- **Trabajo:** Registrar EC2 x2 simultáneos, volúmenes EBS, ALB/LCU, TLS/ACM, dirección IPv4 pública, transferencia, tablas DynamoDB (lectura/escritura/storage/indexes/transacciones), Redis, CloudWatch, logs, S3/state/backup, Route53/DNS, endpoints/NAT, Secrets, SNS y egress. Cada fila: SKU, modalidad, región, cuota, tarifa actual, crédito aplicable, restricción SCP, horas y suma.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-000-03 — Cerrar dos escenarios

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y GOV-07, GOV-15, A-000, A-110, R-001..060.
- **Trabajo:** A) dos instancias encendidas simultáneamente 24×7 durante 45 días; B) 2 encendidas simultáneamente únicamente durante ventanas de prueba y apagadas fuera. Ambos preservan el cumplimiento de dos instancias durante el despliegue. No presentar escenario B como 24×7.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-000-04 — Cerrar costeo

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y GOV-07, GOV-15, A-000, A-110, R-001..060.
- **Trabajo:** USD estimado = Σ (horas×precio-hora + unidades×precio-unidad + almacenamiento GB-mes×tarifa + transferencias), antes de créditos; cobro neto = max(0, USD antes de créditos − créditos aplicables), considerando tarifas variables e impuestos. Cero neto por créditos NO es costo base cero.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-000-05 — Cerrar bloqueo de decisión

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y GOV-07, GOV-15, A-000, A-110, R-001..060.
- **Trabajo:** Si cualquier servicio necesario no está permitido en plan Free/SCP, o el costo no cumple el criterio literal del profesor, marcar `BLOCKED_FINOPS`; documentar ADR y pedir aprobación del responsable y, si cambia un requisito docente, aclaración formal del profesor. No pasar a Paid automáticamente ni sustituir por un mock.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-000-06 — Cerrar controles

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y GOV-07, GOV-15, A-000, A-110, R-001..060.
- **Trabajo:** Presupuesto zero-spend, avisos Free Tier, umbrales preventivos y reporte diario; alarmas avisan, NO garantizan impedir un cargo ni destruyen recursos. Ausencia de reporte de costos no equivale a USD 0.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-000-07 — Cerrar investigación con fecha

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y GOV-07, GOV-15, A-000, A-110, R-001..060.
- **Trabajo:** Fuente oficial vigente: Free plan máximo seis meses o agotamiento de créditos; ALB cobra por hora/LCU; IPv4 pública tiene cargo por hora; Redis administrado debe cotizarse. Precios de otras regiones NO son evidencia de Ohio.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-000-08 — Cerrar separación de responsabilidades

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y GOV-07, GOV-15, A-000, A-110, R-001..060.
- **Trabajo:** La aprobación I-GO exige diseño demostrablemente viable, pero no requiere ni autoriza aprovisionar servicios reales durante H05. La comprobación completa de uso/cargos será en releases H08 tras autorización.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-000-90 — Revisión de riesgos y dependencias

- **Estado:** TODO.
- **Trabajo:** resolver o escalar: Servicios limitados por Free/SCP, Tarifas que cambian por región/fecha y consumo, Créditos agotados o vencidos antes de evaluación, ALB, IP, Redis y observabilidad con facturación incremental.
- **Criterio DoD:** cada riesgo clasificado RESOLVED/BLOCKED/ACCEPTED con firma/fecha; no usar ACCEPTED para incumplir requisito docente sin aclaración.

### T-000-99 — Ratificación de la especificación

- **Estado:** REVIEW pendiente.
- **Entrada:** `spec.md`, `plan.md`, `tasks.md`, `checklist.md` actualizados.
- **Salida:** decisión explícita del responsable y vínculo a evidencia en informe I-GO.
- **Bloqueo:** no crear Terraform/endpoints ni ejecutar AWS.

## Registro de implementación H08-I — 2026-11-06 (rama `feat/h08i-i000-finops`)

Estado global: `IMPLEMENTED_LOCAL` + `BLOCKED_FINOPS`. Ningún recurso AWS fue creado. La sesión AWS
del perfil `nexum` estaba expirada, por lo que toda evidencia de cuenta, precios y costo observado
permanece `NOT_VERIFIED`/`NOT_RUN`.

| Tarea | Estado real | Evidencia |
|---|---|---|
| T-000-01 Plan y contexto | `IMPLEMENTED_LOCAL` / evidencia `NOT_VERIFIED` | `infra/finops/consultas_readonly.py`; `infra/finops/evidencia/cuenta_2026-11-06.json` |
| T-000-02 Matriz por recurso | `PASS` documental (precios sin revalidar) | `infra/finops/MATRIZ_COSTOS_SERVICIOS_I000.md`; `precios_us-east-2.json` |
| T-000-03 Dos escenarios | `PASS` (modelo A/B/C) | `infra/finops/escenarios.py`; 63 pruebas OK |
| T-000-04 Costeo | `PASS` cálculo (bruto 84.0580; residual 6.8440) | `infra/finops/resultados/costos_45d.json` |
| T-000-05 Bloqueo de decisión | `PASS` regla aplicada → `BLOCKED_FINOPS` | `infra/finops/resultados/viabilidad_i000.json` |
| T-000-06 Controles | `NOT_RUN` (prohibido crear presupuestos/alarmas en I-000) | catálogo `presupuestos` en `consultas_readonly.py` |
| T-000-07 Investigación con fecha | `PASS` parcial (fuentes fechadas; sin Price List API) | `precios_us-east-2.json` campo `fecha_fuente` |
| T-000-08 Separación de responsabilidades | `PASS` | `MATRIZ_VIABILIDAD_AWS_I000.md` secciones 1 y 4 |
| T-000-90 Riesgos y dependencias | `BLOCKED` (4 riesgos abiertos) | `infra/finops/ADR_COSTO_CERO_I000.md` sección 5 |
| T-000-99 Ratificación | `PENDIENTE` de decisión humana | este registro + PR de I-000 |

**Pruebas ejecutadas:** `py -3.13 -m unittest discover -s tests/infra/finops` → 63 pruebas OK;
`consultas_readonly.py --validar` → 16 consultas, ninguna mutante; `requisitos.py` → diseño válido,
runtime `NOT_RUN`; `viabilidad.py` → `BLOCKED_FINOPS`.

**No ejecutado:** Terraform init/plan/apply/destroy, creación de recursos, presupuestos, alarmas,
Cost Explorer (0.01 USD por solicitud), cualquier verificación HTTP pública.

### Corrección H08-I — revisión 2 (2026-11-06, sesión AWS válida)

| Tarea | Estado real | Evidencia |
|---|---|---|
| T-000-01 Plan y contexto | `PASS` (AWS real) | `evidencia/cuenta_2026-11-06.json`: FREE/ACTIVE, expira 2027-05-04, 120.0 USD |
| T-000-02 Matriz por recurso | `PASS` parcial | 14 SKU `VERIFIED_AWS`; 11 partidas requeridas pendientes |
| T-000-03 Escenarios | `PASS` | A 91.8080 · B 28.9820 (no 24×7) · C 84.0080 |
| T-000-04 Costeo | `PASS` cálculo; bruto verificado `UNKNOWN` | `resultados/costos_45d.json` |
| T-000-05 Bloqueo de decisión | `PASS` regla aplicada → `BLOCKED_FINOPS` (7 motivos) | `resultados/viabilidad_i000.json` |
| T-000-06 Controles | `NOT_RUN` (crear presupuestos/alarmas prohibido en I-000) | catálogo `presupuestos` |
| T-000-07 Investigación con fecha | `PASS` parcial | `usagetype` y `fecha_fuente` por SKU verificado |
| T-000-08 Separación de responsabilidades | `PASS` | `MATRIZ_VIABILIDAD_AWS_I000.md` §1 |
| T-000-90 Riesgos y dependencias | `BLOCKED` (7 riesgos abiertos) | `ADR_COSTO_CERO_I000.md` §5 |
| T-000-99 Ratificación | `PENDIENTE` de decisión humana | PR #15 |

**Defectos corregidos:** `servicequotas` → `service-quotas`; `$env:TEMP` → `tempfile.gettempdir()`;
`--region` duplicado; `--filters` repetido; `ElasticLoadBalancing` → `AWSELB`; ElastiCache con
tarifa de soporte extendido; DynamoDB con tramo gratuito; lectura de evidencia desde el directorio
equivocado.

**Pruebas:** `py -3.13 -m unittest discover -s tests/infra/finops` → 140 pruebas OK.

**No ejecutado:** Cost Explorer (0.01 USD por solicitud), Terraform, creación de recursos,
presupuestos, alarmas, despliegue y verificación HTTP.

### Revisión 3 — resolución de precios y blindaje (2026-11-06)

| Tarea | Estado real | Evidencia |
|---|---|---|
| T-000-02 Matriz por recurso | `PASS` | 22 SKU `VERIFIED_AWS` + 3 partidas `DOC_OFICIAL` con fuente |
| T-000-04 Costeo | `PASS` | bruto C 84.0430, verificado 79.0430, residual 6.8790 |
| T-000-05 Bloqueo de decisión | `PASS` regla aplicada → `BLOCKED_FINOPS` (5 motivos) | `resultados/viabilidad_i000.json` |
| T-000-90 Riesgos y dependencias | `PASS` documentado; riesgos abiertos en ADR §7 | `ADR_COSTO_CERO_I000.md` |

- Cuota EC2 corregida: 2 instancias × 2 vCPU = **4 vCPU** (cuota 16).
- Internet Gateway agregado al diseño por requisito oficial de VPC Origins (`rutas_publicas: false`).
- Evidencia de despliegue endurecida: huella SHA-256 + IDs reales + documento de control con frase.
- Pruebas FinOps: **149 OK**.

