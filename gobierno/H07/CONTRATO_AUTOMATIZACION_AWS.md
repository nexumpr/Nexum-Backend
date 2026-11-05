# IaC 100 % obligatoria, con operación diaria automatizada

Este archivo **complementa sin reemplazar** H05 I-000..I-150 y `CONTRATO_IAC_OPERACION_H05_V4.md`. Es una lista de pruebas a implementar, no un Terraform ejecutable.

**Aplicaciones obligatorias:** bootstrap y estado Terraform backend reproducible (bootstrap explícito, sin secret/state local subido); `fmt/validate/security/cost/plan`; GitHub OIDC mínimo privilegio para AWS con sesión temporal, sin llaves estáticas; plan ligado a commit SHA; aprobación antes de apply; comprobación de Free, saldo, vigencia, cuota, Redis SKU, CloudFront Origin y TLS; dos instancias saludables y ALB; smoke tests reales y rollback explícito.

**Operaciones exigidas:** `plan`, `apply` (aprobado), `status`, `suspend-aux`, `resume-aux`, `reconcile`, `rollback`, `destroy-aux` (aprobado). `core` incluye 2 EC2, CloudFront, ALB, Redis, DynamoDB, red, secretos y state; `auxiliary` se gestiona por estado/stack independiente, sin afectar core. La tag `nexum:criticality` debe existir y ser verificada por código, ausencia = DENY. `destroy-aux` nunca acepta un plan que elimina core o datos.

**Disponibilidad y costo:** no programar apagado del core para ajustar costos; si presupuesto real pone en riesgo $0 personales, **bloquear nuevas mutaciones y escalar**, no afirmar que API siempre estará disponible. AWS Free puede cerrar recursos al agotar crédito o caducar. Alertas sin límite de gasto NO son garantía. Automatizar controles para prevenir cambios de pago y preservar respaldos dentro de elegibilidad/costo.

**Dependencias de red:** sin NAT por defecto NO equivale a conectividad total. Demostrar descarga de binarios/SSM/CloudWatch/ECR o S3 y DNS, TLS a endpoints, AWS VPC endpoints y precios; documentar opción de bootstrap offline firmado. Probar que CloudFront VPC Origin es elegible y tiene permiso real, no solo lectura UI; HTTPS público no equivale a TLS hasta origen privado.

**Criterios de evidencia H08:** plan aprobado, terraform apply exitoso por pipeline, costo auditado, prueba 2 nodos y failover, persistencia DynamoDB real, Redis real compartido, CloudFront HTTPS, emisión/revocación JWT, observabilidad, backup/restore, suspend/resume aux y drift reconciliation. Cada prueba con identificador de commit, región, timestamp y datos anonimizados. Resultado `NOT_RUN` hasta entonces.
