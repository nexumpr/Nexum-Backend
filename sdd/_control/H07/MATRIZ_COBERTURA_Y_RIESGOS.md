# Cobertura documental real y calidad de cierre

| Frente | Cobertura | Veredicto documental | Pendiente de ejecución |
|---|---|---|---|
| Gobernanza | H03, 8 agentes y 9 skills, constitución | PASS según entrega previa | agentes aún restringidos; ampliar solo con gate |
| Arquitectura | A-000..A-110: 12 × 4 | APPROVED, sin cambios H07 | código Go, pruebas unitarias/integración |
| Infraestructura | I-000..I-150: 16 × 4 | H05 APPROVED_DOCUMENTAL_CONDICIONADO | IaC completa, CI/CD, 2 nodos/ALB/Redis/DynamoDB/CloudFront |
| Endpoints | E001..E075: 75 × 4 + OpenAPI | SHA/validación contractual PASS; Gate humano pendiente | HTTP real, persistencia, seguridad runtime, proveedor IA |
| Economía | estimación 45 días y guardas AWS Free | MODELO, no factura certificada | saldo/vencimiento/SKU Redis/TLS/egreso/IVA/permisos |
| Publicación | H07, catálogo, hashes, workflow CI | preparado; Gate pendiente | GitHub actor `nexumpr`, repo nuevo, PR, revisión humana |
| Fuentes | matriz H04 + inventario H06 | trazabilidad interna PASS | cotejar ZIP Android vigente y PDF original |

**La estructura base contiene 103 especificaciones** (12 A + 16 I + 75 E) y **412 archivos** por cada conjunto `spec/plan/tasks/checklist`, más anexos, agentes y gobierno. Ninguna validación documental demuestra completitud de implementación ni garantiza presupuesto 45 días.

Prioridad de riesgos: R1 (BLOCKER antes de AWS): costo/pago cero y elegibilidad, SCP/IAM, salida a servicios desde VPC privada, CloudFront VPC Origins/HTTPS, Redis precio exacto. R2 (BLOCKER antes de release funcional): proveedor ML/LLM y costo, email/push, modelo DynamoDB bajo concurrencia, control idempotencia por dos nodos, pruebas reales. R3 (antes de afirmar cobertura móvil/academia): fuente original y matriz de diferencias.
