# H05 — Estimación integral v2 para 45 días (5-nov-2026)

**Estado: `REVIEW/ACCOUNT_EVIDENCE_REQUIRED`; no es cotización de AWS emitida en la cuenta, no constituye Gate aprobado.** Horizonte 45 × 24 = 1,080 h; región `us-east-2`; moneda USD; AWS Free activo con 120 USD y expiración documentada 2027-05-04 UTC (evidencia aportada previamente). Los precios son antes de créditos y ANTES DE IMPUESTOS.

## Decisión técnica propuesta sin modificar H04

Dos EC2 `t4g.micro` con binario Go `linux/arm64` en AZ distintas, ASG desired=min=max=2 mientras está ON; ALB internet-facing en dos subredes públicas con ACM TLS 443, target group puerto privado Go; ambas instancias EC2 en subredes públicas con IPv4 (cuesta, pero evita NAT Gateway y endpoints de interfaz). **Inbound de EC2 solo desde SG del ALB en puerto de aplicación**, sin SSH, SG restrictivos y acceso administrativo vía SSM usando salida TCP/443; no exponer Redis. Instancias EC2: IMDSv2, IAM roles, `t4g` en modo de créditos standard para controlar cargos, grupos egress/telemetría revisados. DynamoDB y S3 por VPC Gateway endpoints sin coste adicional; Redis OSS administrado en ElastiCache `cache.t4g.micro`, **1 nodo**, subred privada, TLS y AUTH, SG restringido al SG backend. Mismo Redis compartido por los dos backend; DynamoDB almacena datos financieros, Redis es cache/sesiones efímeras con reconstrucción; un Redis de un nodo **no es HA** y exige política de degradación segura en A-080. Dos zonas para backend/ALB; cache single-AZ es punto único de falla aceptable solo con degradación controlada y ratificación humana.

No usar Valkey en sustitución de Redis OSS, ni Redis local de prueba, ni NAT administrado sin RFC. Una tercera EC2 para Redis sí sería Redis real, pero **tres `t4g.micro` suman 6 vCPU** y la cuota EC2 estándar por defecto es 5; sin evidencia de cuota elevada, esa alternativa está BLOQUEADA. EBS dos volúmenes gp3 10GB; no contenedores ni Docker. Uso de dominio propio YA registrado y validable por DNS; un ACM público estándar sin dominio controlado no basta para URL HTTPS confiable.

## Escenarios calculados (con decimal exacto; ver CSV y JSON)

| Escenario | Disponibilidad HTTP esperada | Bruto 45 días, USD | Resto de créditos teórico, USD | Objetivo ≤90 USD / reserva ≥30 USD |
|---|---|---:|---:|---|
| A — dos Go + ALB + Redis OSS siempre ON 24/7 | 24h/día, solo tras comprobar 2 targets healthy | **103.41** | **16.59** | **NO CUMPLE** objetivo prudencial; dentro 120 antes de variación/impuestos |
| B — dos Go ON 8h/día; ALB+Redis persistentes 24h/día | **8h/día**, fuera de horario el ALB no sirve API | **84.11** | **35.89** | Cumple objetivo bajo estas hipótesis; **NO 24/7** |

Para B son 360 h por backend e IPv4 de backend, 1,080 h de ALB/2 IP/Redis/EBS/DNS; NO se apaga Redis administrado (ElastiCache no tiene stop operativo genérico): destruir/recrear exige un RFC, posible restauración y validación de DNS; no se presupuesta como ahorro automático. Ingreso de tráfico estimado 1 LCU promedio, 3 GB de logs, 2 GB DynamoDB, 2 millones RRU y 600,000 WRU (unidades facturadas, no operaciones netas), 10 GB egress, 10 GB inter-AZ, 2 secretos, 3 custom metrics, 2 alarms, 1 restauración y reserva de 2 USD CI/CD. Una tasa desconocida no se inventa como cero; partidas con tasa de referencia o hipótesis se etiquetan. Se eligió provisión conservadora y no se aplicaron posibles subsidios de DynamoDB ni IPv4 en este cálculo.

**Sensibilidad no incluida en base:** IVA del 16% si aplica a la facturación mexicana: +**16.55 USD** hipotéticos sobre A; la cifra conjunta sería **119.95 USD** y deja casi ningún margen frente a los 120 USD. AWS documenta impuesto aplicable a ciertos clientes en México y que los créditos promocionales no cubren impuestos, pero **no se ha verificado cómo se factura esta cuenta Free particular**, incluyendo base imponible y momento de aplicar créditos. No declarar impuesto cobrado ni cero sin una factura/detalle Billing o confirmación oficial; no usar el colchón de 16.59 USD como garantía. Si el IVA real se cobra al margen del crédito, hay riesgo de desembolso aunque AWS Free afirme que el plan evita cargos por el consumo permitido. Dominio no registrado y su renovación tampoco se absorben automáticamente con créditos; confirmar dominio previo y costos fuera de AWS.

## Comparativa de arquitectura y riesgos

- **A gestionada (recomendada si se exige 24/7):** 2 Go EC2 + 1 ElastiCache Redis OSS + ALB HTTPS + DynamoDB + S3/CloudWatch. 4 IPv4 mínimas (2 ALB, 2 EC2), sin NAT. ~103.41 USD preimpuestos y fuera del objetivo reserva 30. Riesgos: LCU extra, emisión TLS/DOMINIO, tax, variación de consumo, créditos restantes cuando se encienda, permisos/quota y CPU burst.
- **B ventanas**: mismos servicios reales, 2 Go simultáneos solo en ventana aprobada; ~84.11 USD preimpuestos. No es equivalente a API 24/7.
- **C Redis autogestionado**: nodo EC2 con Redis OSS real y SG cerrado podría reducir costo, pero se bloquea por 6 vCPU frente al default 5 hasta verificar cuota, mantenimiento/parcheo y rutas seguras; **no se escoge silenciosamente**.
- **NAT Gateway**: rechazado para la configuración elegida porque añade componente fijo caro (~USD 0.045/h en precios de referencia, más tráfico) sin necesidad; prohibido aprovisionarlo automáticamente. VPC endpoints tipo Interface: no seleccionados, no costo declarado porque NO existen; si hay una política que obliga a backend privado, recalcular con NAT/Interface y volver a Gate.

## Riesgo de 45 días y umbrales

Saldo inicial **documentado**, no observado en vivo aquí: 120 USD. Al implementar, exigir saldo real ≥ costos reestimados+reserva definida; no iniciar A con pretensión de reserva de 30. Para B sugerir presupuesto alerta a USD 45, 65, 75 y 85; para A a 50, 75, 90 y 100 (en uso acumulado, no cargos bancarios). Ejecutar comprobación diaria de facturación y detectar aumento de LCU/IP/GB, y tener rollback de runtime sin borrar DynamoDB/state. Alertas AWS no son corte duro instantáneo; AWS Free puede cerrar cuenta al agotarse el crédito y se podrían perder recursos si no se preservan copias exportables autorizadas.

## Barreras para declarar H05 APPROVED (no repetir H03/H04)

1. Redis OSS SKU directo AWS Price List (OnDemand/node-hour, `cache.t4g.micro`, Ohio, Redis **no Valkey/Memcached**, sin Extended Support); nueva utilidad de solo lectura lista.
2. `service-quotas get-service-quota` EC2 vCPU >=4; dos `t4g.micro` ofertables en 2 AZ, ALB autorizado, ElastiCache permitido en Free. Listas públicas demuestran inclusión a nivel servicio, **no autorizan cada operación IAM/SCP**; permisos de creación se verificarán en H08 tras autorización, no ahora.
3. Registrar si ya existe un dominio propio con DNS controlable, o tratar compra y validación HTTPS como bloqueo sin presupuesto.
4. Confirmar aplicación del crédito USD 120 a EC2/ALB/ElastiCache/Dynamo/IPv4 y la tributación/IVA de la cuenta Free con evidencia Billing y términos específicos; sin desembolso externo garantizado solo cuando AWS lo confirme.
5. Decidir explícitamente si se requiere 24/7: **A = $103.41 sin $30 de reserva**, **B = $84.11 con ventana**. Si son obligatorios 24/7 Y reserva USD 30, hay incompatibilidad bajo el modelo actual; elegir una tarifa/topología adicional verificada o aclaración formal del responsable.
6. Adjuntar evidencia de Pricing Calculator/Price List para partidas etiquetadas hipótesis (logs, DDB, S3, egress) y capturas de cuenta sanitizadas; este documento no las convierte en resultados confirmados.

**Resultado actual: `BLOCKED_EVIDENCE_FINOPS` por incertidumbre de elegibilidad SKU, IVA/créditos, dominio y condición de disponibilidad.** El presupuesto A está por debajo de 120 USD antes de impuestos y variaciones, **pero esto no equivale a garantía operativa 45 días**. Gate listo para valoración humana solo tras resolver los bloqueos.

## Fuentes (revisadas 2026-11-05)

- Free plan: https://docs.aws.amazon.com/awsaccountbilling/latest/aboutv2/free-tier-plans.html
- Servicios elegibles: https://docs.aws.amazon.com/accounts/latest/reference/supported-services-sign-up-new.html
- Restricciones SCP: https://docs.aws.amazon.com/accounts/latest/reference/scps-and-rcps-for-projects.html
- Cuota 5 vCPU: https://docs.aws.amazon.com/ec2/latest/instancetypes/ec2-instance-quotas.html
- Pricing ElastiCache oficial (concepto): https://aws.amazon.com/elasticache/pricing/
- Contraste Redis Ohio $0.016 (fuente secundaria declara Price List API): https://reserver.io/reserved-instances/regions/us-east-2
- VPC IPv4: https://aws.amazon.com/vpc/pricing/
- ALB: https://aws.amazon.com/elasticloadbalancing/pricing/
- Gateway endpoints gratis: https://docs.aws.amazon.com/vpc/latest/privatelink/gateway-endpoints.html
- DynamoDB precios: https://aws.amazon.com/dynamodb/pricing/
- DynamoDB backup: https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/DynamodbDisasterRecoveryStrategy.html
- ACM: https://aws.amazon.com/certificate-manager/faqs/
- Route53: https://aws.amazon.com/route53/pricing/
- AWS Mexico tax 16%: https://aws.amazon.com/tax-help/mexico/
- Promotional credits/taxes: https://aws.amazon.com/awscredits/
- Calculadora sin IVA: https://aws.amazon.com/aws-cost-management/aws-pricing-calculator/faqs/
- CloudWatch: https://aws.amazon.com/cloudwatch/pricing/
- Secret Manager: https://aws.amazon.com/secrets-manager/pricing/
