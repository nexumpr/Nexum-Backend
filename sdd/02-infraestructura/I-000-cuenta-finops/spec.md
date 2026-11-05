# I-000-cuenta-finops — Cuenta AWS, plan Free y estudio de costo cero

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | GOV-07, GOV-15, A-000, A-110, R-001..060 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## 1. Objetivo y definición de terminado

Determinar antes del Gate I-GO la viabilidad real de TODOS los requisitos obligatorios usando la cuenta y región autorizadas, sin consumir créditos por sorpresa, ni confundir saldo de créditos con uso gratuito.

## Autoridad y restricciones

- `constitution.md` (GOV-01..18, R-001..060) y el Gate H04 aprobado explícitamente el **2026-11-05** tienen precedencia; H05 diseña infraestructura, no la ejecuta.
- Arquitectura ratificada: Go (Clean Architecture), AWS, DynamoDB, **Redis real**, **2 instancias simultáneas**, balanceador HTTPS y CI/CD/DevSecOps. Ninguno es demostración opcional.
- El PDF del docente v3 exige Backend cloud con costo cero; el catálogo V6.0.2 enumera 75 rutas propuestas E001..E075 y utiliza stack histórico GCP/MongoDB **descartado**.
- No crear código Go, archivos Terraform `.tf`, pipelines ni endpoints H06 durante H05; `terraform apply/destroy`, `git push`, cambios AWS y facturación están prohibidos hasta fases y aprobaciones autorizadas.
- **Regla de evidencia:** documentos pueden alcanzar DRAFT completo sin acceso cloud; Gate I-GO requiere demostrar **viabilidad operacional y económica**, no asumirla. Campos pendientes no se cambian a PASS ni se solventan con créditos.

## 2. Contrato de infraestructura propuesto

### INF-000-01 — Plan y contexto

Región de evaluación us-east-2 (Ohio); tomar tipo de cuenta (Free/Paid), fecha de alta, expiración y créditos desde consola o APIs de lectura con evidencia fechada. No cambiar el plan. La cuenta y región reales se seleccionan por entorno, nunca se codifican como secretos.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-000-02 — Matriz por recurso

Registrar EC2 x2 simultáneos, volúmenes EBS, ALB/LCU, TLS/ACM, dirección IPv4 pública, transferencia, tablas DynamoDB (lectura/escritura/storage/indexes/transacciones), Redis, CloudWatch, logs, S3/state/backup, Route53/DNS, endpoints/NAT, Secrets, SNS y egress. Cada fila: SKU, modalidad, región, cuota, tarifa actual, crédito aplicable, restricción SCP, horas y suma.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-000-03 — Dos escenarios

A) dos instancias encendidas simultáneamente 24×7 durante 45 días; B) 2 encendidas simultáneamente únicamente durante ventanas de prueba y apagadas fuera. Ambos preservan el cumplimiento de dos instancias durante el despliegue. No presentar escenario B como 24×7.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-000-04 — Costeo

USD estimado = Σ (horas×precio-hora + unidades×precio-unidad + almacenamiento GB-mes×tarifa + transferencias), antes de créditos; cobro neto = max(0, USD antes de créditos − créditos aplicables), considerando tarifas variables e impuestos. Cero neto por créditos NO es costo base cero.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-000-05 — Bloqueo de decisión

Si cualquier servicio necesario no está permitido en plan Free/SCP, o el costo no cumple el criterio literal del profesor, marcar `BLOCKED_FINOPS`; documentar ADR y pedir aprobación del responsable y, si cambia un requisito docente, aclaración formal del profesor. No pasar a Paid automáticamente ni sustituir por un mock.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-000-06 — Controles

Presupuesto zero-spend, avisos Free Tier, umbrales preventivos y reporte diario; alarmas avisan, NO garantizan impedir un cargo ni destruyen recursos. Ausencia de reporte de costos no equivale a USD 0.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-000-07 — Investigación con fecha

Fuente oficial vigente: Free plan máximo seis meses o agotamiento de créditos; ALB cobra por hora/LCU; IPv4 pública tiene cargo por hora; Redis administrado debe cotizarse. Precios de otras regiones NO son evidencia de Ohio.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-000-08 — Separación de responsabilidades

La aprobación I-GO exige diseño demostrablemente viable, pero no requiere ni autoriza aprovisionar servicios reales durante H05. La comprobación completa de uso/cargos será en releases H08 tras autorización.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

## 3. Dependencias y secuencia

I-010/I-020/I-030/I-040/I-050/I-110/I-120; aprobación de las 12 A en Gate H04

## 4. Criterios de aceptación de diseño (Gate H05)

- Tabla de precios/limitaciones con sello de fecha y servicio, sin «gratis» por suposición.
- Costeo a 45 días de escenarios A/B con rangos de consumo y crédito separado.
- Detección explícita de toda línea que tenga costo >0 y de incompatibilidades SCP/plan.
- ADR de factibilidad firmado por humano antes de I-GO; ninguna tarifa desconocida marcada 0.
- Plan de alarmas, shutdown y costo residual de datos y servicios aunque el runtime esté apagado.

## 5. Riesgos que impiden una aprobación sin evidencia

- **Riesgo:** Servicios limitados por Free/SCP. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.
- **Riesgo:** Tarifas que cambian por región/fecha y consumo. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.
- **Riesgo:** Créditos agotados o vencidos antes de evaluación. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.
- **Riesgo:** ALB, IP, Redis y observabilidad con facturación incremental. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.

## 6. Entregables y referencias

Los siguientes nombres identifican evidencias/artefactos **futuros** de I-GO y de implementación; no son archivos creados en H05: `MATRIZ_COSTOS_SERVICIOS_H05.md`, `MATRIZ_VIABILIDAD_AWS.md`, `ADR-COSTO-CERO-PENDIENTE.md`.

## 7. Criterio de bloqueo y aprobación

**BLOCKED** si falta una obligación técnica, contradice alguna spec A aprobada, no puede comprobarse la viabilidad Free/SCP o no hay aceptación humana del riesgo. **DRAFT** significa que el diseño está documentado; nunca equivale a infraestructura aplicada.

## 8. Cierre tecnico-financiero H05 (propuesta v2; aun no aprobada)

- ADR propuesto H05-FINOPS-20261105: comparar A 24/7 **$103.41** vs B 8h/día **$84.11**, ambos 45 días, antes de créditos, IVA y compra de dominio. El **límite prudencial $90/$30 NO se cumple en A**. B sí cumple bajo supuestos, pero no ofrece URL 24/7. La cifra definitiva debe recalcularse con precios Ohio del perfil real; no se autoriza marcar I-GO PASS.
- Para Excel/inspección humana: anexo `FINOPS_45_DIAS_DESGLOSE_V2.csv` en USD, alcance por recurso y fuentes; no aplicar descuento por Free ni fingir cero bruto.
- Bloqueos explícitos: Redis OSS exacto AWS API, elegibilidad SKU y cuota, control dominio HTTPS, crédito aplicable, posible IVA en México sobre servicios elegibles (no asumir ni 0 ni un cobro sin evidencia), variación de egress/LCU/CPU; cargar en evidencia privada sanitizada.
- Estado fuente: `INFORME_FINOPS_Y_TOPOLOGIA_H05_V2.md`; Gate separado `GATE_H05_REVISION_PENDIENTE.md`, NO aprobado.


> **PRECEDENCIA H05:** esta enmienda V3 es la propuesta vigente frente a cualquier alternativa incompatible de secciones V2 anteriores. No se considera aprobada hasta firma humana Gate H05.
## 9. Enmienda v3 NO APROBADA — regla irrenunciable de cero desembolso personal

Decisión del responsable (2026-11-05): **ningún peso de dinero personal**, nunca se debe actualizar voluntaria/automáticamente a AWS Paid ni suscribir servicios, comprar dominio o usar fuentes de cobro externas. La cuenta debe permanecer `FREE/ACTIVE`, con revisión de estado y saldo antes de cualquier aprovisionamiento futuro. La documentación oficial AWS Free confirma ausencia de cargos durante el plan y cierre de la cuenta al agotarse créditos o caducar, **lo cual causaría interrupción total de servicio y riesgo sobre datos**. También advierte acciones excepcionales que convierten cuenta a Paid (por ejemplo AWS Organizations); prohibidas. La facturación estimada BRUTA sigue existiendo y consume créditos. Cero desembolso no significa costo cero de recursos.

La API, DynamoDB, dos instancias Go, ALB, CloudFront y Redis OSS administrado deben permanecer **operativos 24×7 durante la ventana de 45 días**; no seleccionar la opción anterior de 8 h/día para el entorno requerido. El coste optimizado C se documenta en `FINOPS_45_DIAS_MODELO_V3.json` y `ADR_H05_CERO_PESOS_45_DIAS_V3.md`. C = **USD 84.058** bruto antes de créditos bajo sus supuestos; remanente teórico USD 35.942; se mantiene la reserva-meta de USD 30. No es garantía de consumo real ni de 45 días sin agotar saldo; activar vigilancia y umbrales preventivos antes del despliegue H08.

Topología C: CloudFront con dominio default TLS `*.cloudfront.net` y origen VPC hacia ALB privado, 2 EC2 privadas, S3+DynamoDB por gateway endpoints (no NAT), Redis ElastiCache OSS privado. Esto elimina 4 IPv4 públicas y Route 53 del presupuesto v2, no sustituye Redis por simulación. CloudFront VPC Origins es compatible con Ohio y según AWS no tiene cargo adicional por la funcionalidad; **aún debe confirmarse disponibilidad/permisos para el perfil Free particular**. `I-030` plantea TLS de cliente en CloudFront con tramo HTTP privado ALB: pendiente de aceptación de seguridad; si se exige TLS extremo a extremo, se necesita nombre/certificado aceptable y **recotizar**, sin dinero personal.

La variante A anterior de USD 103.41 (ALB público) se conserva solo como comparador histórico; la variante B (8h) NO satisface disponibilidad. Las partidas AWS $0 se basan únicamente en ausencia de recurso o beneficios explícitos, nunca en crédito gratis inferido. Se prohíbe cualquier mecanismo automático que desactive el core al detectar presupuesto; escalar al responsable y asegurar respaldo, sin crear pagos.
