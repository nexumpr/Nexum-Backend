# H05 — Matriz de costos y bloqueos, v3 (2026-11-05)

**Estado: DRAFT / BLOCKED_EVIDENCE_AND_GATE — NO APROBADO.** H03/H04 aprobados no cambian. Este documento reemplaza la matriz v2; se conserva `FINOPS_45_DIAS_MODELO_V2.json` como evidencia de escenario anterior, **no como arquitectura elegida**.

## Política del usuario (no negociable)

- **CERO MXN de fondos personales**: mantener AWS Free, nunca AWS Paid, no comprar dominio, no suscribir plan CloudFront flat-rate (AWS Free Tier y plan comercial CloudFront Free son productos distintos). AWS Free garantiza sin cargos durante su vigencia pero cierra la cuenta al agotarse créditos o expirar; por tanto NO garantiza 45 días de uptime. Una alerta es aviso, no corte de gasto. Fuente AWS [Free plans](https://docs.aws.amazon.com/awsaccountbilling/latest/aboutv2/free-tier-plans.html).
- API/2 EC2/ALB/CloudFront/Redis/DynamoDB se mantienen siempre en línea; no aceptar la propuesta anterior 8h/día. Los extras de testing efímeros sólo se eliminan/apagan tras aprobación y no afectan el core.

## Comparación 45 días / 1080 horas, USD brutos antes de créditos

| Recurso | A: versión 2 pública 24×7 | C: privada con CloudFront 24×7 | Cambio |
|---|---:|---:|---|
| Dos EC2 `t4g.micro` Go | 18.14 | 18.14 | sin cambio |
| ALB horas + LCU supuesto 1 LCU | 32.94 | 32.94 | sigue encendido |
| Redis OSS ElastiCache `cache.t4g.micro` | 17.28 | 17.28 | precio secundario, SKU por corroborar |
| Cuatro IPv4 públicas | 21.60 | **0.00** | en C ALB y EC2 son privados; VPC Origin |
| Route 53 zona hospedada | 0.75 | **0.00** | hostname `*.cloudfront.net` |
| EBS 20 GB combinados | 2.40 | 2.40 | sin cambio |
| Demás partidas V2 (Dynamo, S3, observabilidad, etc.) | 10.30 | 10.30 | conservadas como supuestos |
| Reserva de sensibilidad CloudFront | 0.00 | **3.00** | no es tarifa conocida, previsión variable |
| **TOTAL (sumas sin redondeo intermedio)** | **103.408** | **84.058** | **-19.350** |
| **Saldo teórico crédito USD 120** | **16.592** | **35.942** | margen-meta 30 en C |

*La reserva CloudFront 3 USD no es cargo oficial ni límite garantizado. La línea anterior de 10.30 incluye el costo de Internet egress estimado 0.90 para sensibilidad, aunque puede existir free allowance: no se elimina para evitar optimismo. Ver CSV y JSON V3 para todos los sumandos. No hay presupuesto infinito: ALB LCU, logs y tráficos reales pueden superar los supuestos.*

## Bloqueos del Gate vigentes

1. **AWS cuenta real**: confirmar `FREE/ACTIVE`, saldo actual, AZ+EC2 cuota>=4, precios de EC2/ALB/Redis mediante salida redactada del verificador (o consola). Redis `cache.t4g.micro` motor `Redis OSS` no Valkey; si se selecciona C sigue faltando SKU oficial.
2. **Permisos específicos** CloudFront distribuciones / VPC origins y crear ALB interno / EC2 privada, sin llamar operaciones mutantes en H05; List autorizado NO demuestra Create permitido. En consola verificar visibilidad/limitación Free/SCP, y en H08 probar con autorización.
3. **Aprobación seguridad TLS**: CloudFront HTTPS público; HTTP privado hasta ALB, no extremo a extremo. Si docente requiere ALB:443, C no cumple y habría que resolver dominio válido sin desembolso o re-diseñar.
4. **Arranque/gestión sin NAT/SSH**: especificar prueba H08 con S3 gateway artifact, imágenes ARM64 y logs, sin interface endpoints no presupuestados; si invalida operación, recalcular antes de apply.
5. **Créditos reales / tráfico**: confirmar 45 días desde primera creación y circuito FinOps, forecast diario. Alarmas no garantizan impedir cierre.

**No ejecutar** terraform apply, iniciar ni detener instancias, `CreateVpcOrigin`, comprar DNS o crear recursos; ninguna infraestructura ha sido desplegada en H05. Fuente: [CloudFront VPC Origin](https://docs.aws.amazon.com/AmazonCloudFront/latest/DeveloperGuide/private-content-vpc-origins.html), [sin costo adicional por VPC origins](https://aws.amazon.com/about-aws/whats-new/2024/11/amazon-cloudfront-vpc-origins/), [endpoints gateway](https://docs.aws.amazon.com/vpc/latest/privatelink/gateway-endpoints.html).
