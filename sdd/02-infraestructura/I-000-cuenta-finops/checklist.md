# I-000-cuenta-finops — Cuenta AWS, plan Free y estudio de costo cero

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | GOV-07, GOV-15, A-000, A-110, R-001..060 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## Checklist de revisión (cada punto requiere evidencia, no basta la presencia de este archivo)

### Integridad y autoridad

- [ ] H04 tiene aprobación humana y el contenido coincide con las reglas superiores.
- [ ] Se analizó el PDF docente v3 y el catálogo V6.0.2 sin asumir GCP/MongoDB.
- [ ] Se mantiene el alcance de Backend, no se exige validación Mobile.
- [ ] No se crearon recursos ni artefactos de código/EP antes de H08/H06.

### Decisiones técnicas

- [ ] `INF-000-01` **Plan y contexto:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-000-02` **Matriz por recurso:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-000-03` **Dos escenarios:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-000-04` **Costeo:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-000-05` **Bloqueo de decisión:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-000-06` **Controles:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-000-07` **Investigación con fecha:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-000-08` **Separación de responsabilidades:** diseño justificado, referencias, supuestos y evidencia trazable.

### Pruebas y cobertura de diseño

- [ ] Tabla de precios/limitaciones con sello de fecha y servicio, sin «gratis» por suposición.
- [ ] Costeo a 45 días de escenarios A/B con rangos de consumo y crédito separado.
- [ ] Detección explícita de toda línea que tenga costo >0 y de incompatibilidades SCP/plan.
- [ ] ADR de factibilidad firmado por humano antes de I-GO; ninguna tarifa desconocida marcada 0.
- [ ] Plan de alarmas, shutdown y costo residual de datos y servicios aunque el runtime esté apagado.

### Riesgos, economía y aprobaciones

- [ ] Resuelto o escalado formalmente: Servicios limitados por Free/SCP.
- [ ] Resuelto o escalado formalmente: Tarifas que cambian por región/fecha y consumo.
- [ ] Resuelto o escalado formalmente: Créditos agotados o vencidos antes de evaluación.
- [ ] Resuelto o escalado formalmente: ALB, IP, Redis y observabilidad con facturación incremental.
- [ ] Costos potenciales (incluido apagado) identificados por servicio en I-000; no se afirma $0 por usar créditos.
- [ ] SCP y disponibilidad de servicios relevantes documentados (no simulados).
- [ ] Gate I-GO tiene resolución expresa del responsable, o esta spec continúa DRAFT/BLOCKED.

### Ejecución futura H08 (NO ejecutada en este lote)

- [ ] Pruebas AWS/CI/HTTP y sus evidencias ejecutadas realmente tras aprobación y release; no cambiar a PASS antes.

## Registro de verificación H08-I — 2026-11-06

- [x] Herramienta reproducible de comprobación AWS Free: `infra/finops/consultas_readonly.py` (16 consultas, sólo lectura).
- [x] Modelo de precios con fuente y fecha por SKU: `infra/finops/precios_us-east-2.json`.
- [x] Cálculo determinista a 45 días con importes exactos: `infra/finops/modelo_costos.py` (84.0580 USD, residual 6.8440 USD).
- [x] Escenarios de disponibilidad A/B/C con distinción 24×7: `infra/finops/escenarios.py`.
- [x] Validación de dos instancias simultáneas y red privada sin NAT (diseño): `infra/finops/requisitos.py`.
- [x] Cotización de EC2, EBS, ALB, CloudFront, DynamoDB, Redis OSS, S3 y CloudWatch en el catálogo (estado `DOC_ONLY`/`HIPOTESIS`/`SUPUESTO`).
- [x] Separación de costo bruto, créditos verificados, impuestos y desembolso personal: `resultados/costos_45d.json` → `financiero`.
- [x] Matriz de viabilidad financiera: `infra/finops/MATRIZ_VIABILIDAD_AWS_I000.md`.
- [x] ADR de costo cero: `infra/finops/ADR_COSTO_CERO_I000.md` (estado `BLOCKED_FINOPS`).
- [x] Evidencias sanitizadas y reproducibles: `infra/finops/evidencia/`, `infra/finops/EVIDENCIA_I000.md`.
- [x] Pruebas unitarias del cálculo financiero: 63 pruebas OK (`tests/infra/finops`).
- [ ] Plan Free, vigencia y créditos verificados en AWS — **NOT_VERIFIED**: sesión expirada.
- [ ] Precios revalidados contra Price List API — **NOT_VERIFIED**.
- [ ] SKU Redis OSS, VPC Origins, cuota de vCPU y `t4g.micro` confirmados — **NOT_VERIFIED**.
- [ ] Costo observado (0.00 USD) comprobado — **NOT_RUN**: Cost Explorer cuesta 0.01 USD por solicitud.
- [ ] Costo residual aceptado formalmente por el responsable — **PENDIENTE**.
- [ ] IVA y TLS extremo a extremo decididos — **PENDIENTE**.
- [ ] Gate I-GO con resolución expresa — **PENDIENTE**.

**Estado de la spec:** `BLOCKED_FINOPS`. No se marca ningún punto no ejecutado como PASS.

