# I-000-cuenta-finops — Cuenta AWS, plan Free y estudio de costo cero

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | GOV-07, GOV-15, A-000, A-110, R-001..060 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## 1. Estrategia de diseño documental de H05

Determinar antes del Gate I-GO la viabilidad real de TODOS los requisitos obligatorios usando la cuenta y región autorizadas, sin consumir créditos por sorpresa, ni confundir saldo de créditos con uso gratuito.

## Autoridad y restricciones

- `constitution.md` (GOV-01..18, R-001..060) y el Gate H04 aprobado explícitamente el **2026-11-05** tienen precedencia; H05 diseña infraestructura, no la ejecuta.
- Arquitectura ratificada: Go (Clean Architecture), AWS, DynamoDB, **Redis real**, **2 instancias simultáneas**, balanceador HTTPS y CI/CD/DevSecOps. Ninguno es demostración opcional.
- El PDF del docente v3 exige Backend cloud con costo cero; el catálogo V6.0.2 enumera 75 rutas propuestas E001..E075 y utiliza stack histórico GCP/MongoDB **descartado**.
- No crear código Go, archivos Terraform `.tf`, pipelines ni endpoints H06 durante H05; `terraform apply/destroy`, `git push`, cambios AWS y facturación están prohibidos hasta fases y aprobaciones autorizadas.
- **Regla de evidencia:** documentos pueden alcanzar DRAFT completo sin acceso cloud; Gate I-GO requiere demostrar **viabilidad operacional y económica**, no asumirla. Campos pendientes no se cambian a PASS ni se solventan con créditos.

## 2. Plan secuencial y verificaciones

### P-000-01 — Plan y contexto

**Entrada:** H04 aprobado, requisitos `GOV-07, GOV-15, A-000, A-110, R-001..060`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Región de evaluación us-east-2 (Ohio); tomar tipo de cuenta (Free/Paid), fecha de alta, expiración y créditos desde consola o APIs de lectura con evidencia fechada. No cambiar el plan. La cuenta y región reales se seleccionan por entorno, nunca se codifican como secretos.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-000-02 — Matriz por recurso

**Entrada:** H04 aprobado, requisitos `GOV-07, GOV-15, A-000, A-110, R-001..060`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Registrar EC2 x2 simultáneos, volúmenes EBS, ALB/LCU, TLS/ACM, dirección IPv4 pública, transferencia, tablas DynamoDB (lectura/escritura/storage/indexes/transacciones), Redis, CloudWatch, logs, S3/state/backup, Route53/DNS, endpoints/NAT, Secrets, SNS y egress. Cada fila: SKU, modalidad, región, cuota, tarifa actual, crédito aplicable, restricción SCP, horas y suma.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-000-03 — Dos escenarios

**Entrada:** H04 aprobado, requisitos `GOV-07, GOV-15, A-000, A-110, R-001..060`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** A) dos instancias encendidas simultáneamente 24×7 durante 45 días; B) 2 encendidas simultáneamente únicamente durante ventanas de prueba y apagadas fuera. Ambos preservan el cumplimiento de dos instancias durante el despliegue. No presentar escenario B como 24×7.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-000-04 — Costeo

**Entrada:** H04 aprobado, requisitos `GOV-07, GOV-15, A-000, A-110, R-001..060`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** USD estimado = Σ (horas×precio-hora + unidades×precio-unidad + almacenamiento GB-mes×tarifa + transferencias), antes de créditos; cobro neto = max(0, USD antes de créditos − créditos aplicables), considerando tarifas variables e impuestos. Cero neto por créditos NO es costo base cero.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-000-05 — Bloqueo de decisión

**Entrada:** H04 aprobado, requisitos `GOV-07, GOV-15, A-000, A-110, R-001..060`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Si cualquier servicio necesario no está permitido en plan Free/SCP, o el costo no cumple el criterio literal del profesor, marcar `BLOCKED_FINOPS`; documentar ADR y pedir aprobación del responsable y, si cambia un requisito docente, aclaración formal del profesor. No pasar a Paid automáticamente ni sustituir por un mock.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-000-06 — Controles

**Entrada:** H04 aprobado, requisitos `GOV-07, GOV-15, A-000, A-110, R-001..060`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Presupuesto zero-spend, avisos Free Tier, umbrales preventivos y reporte diario; alarmas avisan, NO garantizan impedir un cargo ni destruyen recursos. Ausencia de reporte de costos no equivale a USD 0.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-000-07 — Investigación con fecha

**Entrada:** H04 aprobado, requisitos `GOV-07, GOV-15, A-000, A-110, R-001..060`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Fuente oficial vigente: Free plan máximo seis meses o agotamiento de créditos; ALB cobra por hora/LCU; IPv4 pública tiene cargo por hora; Redis administrado debe cotizarse. Precios de otras regiones NO son evidencia de Ohio.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-000-08 — Separación de responsabilidades

**Entrada:** H04 aprobado, requisitos `GOV-07, GOV-15, A-000, A-110, R-001..060`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** La aprobación I-GO exige diseño demostrablemente viable, pero no requiere ni autoriza aprovisionar servicios reales durante H05. La comprobación completa de uso/cargos será en releases H08 tras autorización.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

## 3. Orden de ejecución futura H08 (no ejecutar ahora)

1. Auditar plan AWS/credenciales/precios y límites SCP sin cambios.
2. Preparar revisión de cambios, artefactos IaC y análisis económico en PR autorizado.
3. Ejecutar validaciones (fmt/validate/plan/pruebas), someter al gate humano.
4. Aprovisionar o modificar únicamente después de autorización de release y protección de datos.
5. Ejecutar pruebas reales, registrar métricas, costos y rollback, y no declarar DONE antes de las evidencias.

## 4. Dependencias cruzadas

I-010/I-020/I-030/I-040/I-050/I-110/I-120; aprobación de las 12 A en Gate H04

## 5. Riesgos y tratamiento

1. `I-000-cuenta-finops-R01`: Servicios limitados por Free/SCP. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.
2. `I-000-cuenta-finops-R02`: Tarifas que cambian por región/fecha y consumo. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.
3. `I-000-cuenta-finops-R03`: Créditos agotados o vencidos antes de evaluación. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.
4. `I-000-cuenta-finops-R04`: ALB, IP, Redis y observabilidad con facturación incremental. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.

## 6. Puerta de entrada a H06

Gate H05/I-GO NO se logra con instalación de Markdown: requiere pruebas de viabilidad económica/cuenta, trazabilidad física Dynamo/Redis, controles IAM y aceptación expresa del responsable. Los contratos detallados E001..E075 se formalizan exclusivamente en H06.
