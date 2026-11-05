# H05 V4 — Contrato de implementación IaC, mínimo no negociable

**Estado:** PROPUESTA PARA GATE, no implementación. No se han creado recursos ni presupuestos AWS definitivos.

## Entregables H08+ exigidos a OpenCode

1. Terraform con módulos separados `network`, `edge-cloudfront-vpc-origin`, `alb`, `go-compute-2az`, `redis-oss`, `dynamodb`, `state`, `security`, `observability-finops`, `auxiliary`; configuración reproducible en `us-east-2`.
2. Plano de dependencias y exportación de API `*.cloudfront.net`, HTTPS público, CORS/JWT/health. Confirmar protocolo al origen, validación de dominio TLS y límites de CloudFront VPC origins; si la exigencia académica es HTTPS extremo a extremo y no se puede cumplir sin pagar, bloquear la implementación.
3. `plan` sin efectos, `apply` con aprobación, `status` lectura, `suspend-aux`, `resume-aux`, `reconcile`, `rollback` y `destroy-aux` con allowlist y revisión de Terraform plan; estrategia de secretos y state remoto; todos los comandos a través de pipeline/CLI, nunca consola manual cotidiana.
4. Bootstrap seguro de AMI/configuración de Go sin NAT; documentar y presupuestar explícitamente rutas de entrega de artefactos, SSM o alternativa, CloudWatch, repositorios de paquetes, egress e interfaces endpoints si se requieren. No asumir que la sola existencia de S3/Dynamo Gateway endpoints habilita SSM o descarga de paquetes.
5. AWS CLI / SDK `read-only` preflight de plan Free, saldo/vencimiento, cuotas, Redis SKU exacto, permiso CloudFront VPC origins, presupuesto con impuestos y tráfico, y `terraform plan`; fallar cerrado si falta evidencia. La UI con Create visible no prueba `CreateVpcOrigin`.
6. Seguridad: no upgrade Paid; sin AWS Organizations ni productos Marketplace; mínimo privilegio; no secrets en Git; las dos EC2 de Go simultáneas; conservar base y state; política de snapshots antes de destruir recursos auxiliares.
7. Evidencia futura: doble target healthy, HTTPS desde cliente, contrato API, DynamoDB/Redis reales, alertas, teardown de auxiliares, restauración idempotente, plan de 45 días y cero cobro personal.

## Clasificación y ciclo

**Core (always-on 45 días):** dos EC2, ALB, CloudFront, Redis OSS, DynamoDB, conectividad, permisos mínimos, estado de IaC. **Auxiliary (on-demand):** entornos efímeros, pruebas de carga/DAST, colectores no críticos, dashboards opcionales, herramientas temporales. Nunca se elimina telemetría imprescindible para detectar gasto/seguridad.

El esquema de etiquetas `nexum:criticality=core|auxiliary` y `nexum:environment` es obligatorio; ausencia de etiqueta => DENY. `suspend-aux` nunca utiliza `terraform destroy` sobre el root de core.

## Costos y restricciones

La cifra v3 de **84.058 USD/45 días** es escenario, no facturación asegurada. El saldo USD 120 reportado no implica que hoy queden USD 120. Calcular saldo real, fecha de expiración, SKU exacto Redis, CloudFront datos/solicitudes, LCU, snapshots, EBS, logs, transferencias e impuestos aplicables; usar safety margin y gates humanos. Cuenta Free puede cerrarse al agotarse créditos; cero pesos personales se impone por encima de disponibilidad.

## Gate

Esta propuesta no es autorización de desplegar. Aprobación expresa de H05 permite **escribir H06**. Escribir H06 no implica autorización de Terraform, GitHub push ni uso de presupuesto.
