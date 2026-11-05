# ADR propuesto H05-FINOPS-V3 — Cero pesos personales y API disponible 45 días

**Estado:** PROPOSED / NOT APPROVED. Fecha 2026-11-05. Fuente de cambio: declaración expresa del responsable el 2026-11-05.

## Restricciones y principios

1. Siempre AWS `FREE`, no pago personal y no cambio a `PAID`. Si un servicio/plan está bloqueado por cuenta, cambiar arquitectura **con Gate nuevo**, nunca forzar compra/paid.
2. API pública de cliente y DynamoDB siempre disponibles; garantizar 2 servidores Go simultáneos y Redis real compartido al operar. Los servicios base que hacen funcionar API también están siempre encendidos (ALB, CloudFront, Redis). `sleep` sólo para entornos de pruebas y herramientas adicionales.
3. Conservar H03/H04 sin modificaciones. H05 prepara despliegue futuro; las cuotas, permisos específicos y emisiones de certificados no están probados mediante creación real.

## Selección propuesta: C / privadas + CloudFront

```text
Internet Android HTTPS (certificado CloudFront *.cloudfront.net)
    |
CloudFront distribution (24x7; APIs sin cache; hostname AWS por defecto)
    |
CloudFront VPC Origin (red privada; sin IP pública, sin cargo adicional propio)
    |  HTTP privado en AWS (sin TLS a nivel aplicación; exige aceptación del riesgo)
ALB interno (2 zonas; real, LCU/h permanente)
    |                       |
EC2 ARM64 Go A          EC2 ARM64 Go B (ambas privadas y encendidas 24x7)
    |                       |
    +---- Redis OSS ElastiCache privado (1 nodo, 24x7) ----+
    +---- DynamoDB on demand mediante Gateway Endpoint ----+
    +---- S3 privado binarios/state mediante Gateway Endpoint
```

- No NAT, ni Internet Gateway necesario para instancias privadas, ni Elastic IP, ni puertos públicos Redis/Go.
- No gRPC (CloudFront VPC origin no lo soporta); Nexum usa REST/JSON.
- Redis ElastiCache node-based `cache.t4g.micro` privado; sin HA del nodo Redis (SPOF explícito, recuperación no inmediata). No suponer Stop: sólo delete lo detiene y rompe requisito mientras API viva.
- CloudFront no balancea los Go: ALB sí. Bloquear caching JWT y finanzas por TTL=0 y políticas correctas; no exponer tokens en logs.
- Sin acceso de mantenimiento de OS por SSH/NAT: artefactos Go firmados/versionados S3 privados y reemplazo con AMI controlada; verificar en H08 antes de publicarlo; no inventar endpoint SSM gratis.
- Riesgo material: entre CloudFront y ALB hay HTTP sobre conexión VPC privada, no TLS aplicación extremo a extremo. Si la rúbrica exige TLS en ALB o cada salto, **este ADR NO es aprobable** sin rediseño; prohibido declararlo cumplido por red privada.

## Costos y crédito

Tabla verificable `FINOPS_45_DIAS_DESGLOSE_V3.csv` / `FINOPS_45_DIAS_MODELO_V3.json`: USD 84.058 estimados para 45d con 120 declarados; margen USD 35.942, meta >=30. Origen de cambios V2: eliminar IPv4 ALB 10.80 + IPv4 EC2 10.80 + zona DNS 0.75; agregar sensibilidad CloudFront 3.00. Mantener ALB, Redis, EC2, Dynamo, backups y extras como anteriormente modelados. Precio de Redis 0.016 USD/h es **secundario, sin comprobación oficial del SKU**. No fingir que AWS carga la reserva CloudFront.

AWS Free según documentación: ausencia de cargos personales *mientras se mantiene Free*; cuando créditos/plan terminan **cierre de cuenta y pérdida de acceso al servicio y datos**. No interpretar que Free ofrece 45d asegurados. Desproteger fondos personales NO es alternativa.

## Acciones manuales / evidencia y aprobaciones

- Ejecutar instalador de este ZIP y AWS `--aws-check` **solo lectura**. No redactar plan como PASS sin evidencia.
- Abrir consola AWS, comprobar cuenta Free/120 restante/fecha, límites y visibilidad de CloudFront VPC origins. `List` no prueba `Create` y no se ejecutará Create hasta H08.
- Revisar requisito del profesor sobre TLS end-to-end: elegir aceptar TLS en borde públicamente o resolver un dominio autorizado sin pago, recotizando.
- Gate H05 firmado por humano tras integrar evidencia y confirmar que el riesgo económico es aceptable. Si no, `BLOCKED`, no pasar a H06.

## Fuentes oficiales (consulta 2026-11-05)

- AWS Free account plan: https://docs.aws.amazon.com/awsaccountbilling/latest/aboutv2/free-tier-plans.html
- CloudFront VPC origins: https://docs.aws.amazon.com/AmazonCloudFront/latest/DeveloperGuide/private-content-vpc-origins.html
- CloudFront VPC origins sin cargo adicional: https://aws.amazon.com/about-aws/whats-new/2024/11/amazon-cloudfront-vpc-origins/
- CloudFront HTTPS default certificate: https://docs.aws.amazon.com/AmazonCloudFront/latest/DeveloperGuide/DownloadDistValuesGeneral.html
- S3+DynamoDB VPC Gateway: https://docs.aws.amazon.com/vpc/latest/privatelink/gateway-endpoints.html
- ELB/EC2/DynamoDB/ElastiCache en Free list: https://docs.aws.amazon.com/accounts/latest/reference/supported-services-sign-up-new.html
