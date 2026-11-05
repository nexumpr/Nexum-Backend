# Gate H05 — Documento de decisión humana pendiente (v3)

**Fecha 2026-11-05 — Estado: BLOCKED_EVIDENCE_AND_APPROVAL — NO APROBADO.** H03/H04 aprobados; H06 aún no iniciado. No se autoriza ningún recurso AWS, upgrade Paid, Terraform, GitHub write ni H08.

## Reglas ya determinadas

- $0 MXN personales bajo cualquier circunstancia. AWS `FREE/ACTIVE` **obligatorio**; nunca ejecutar Upgrade, compras, suscripción AWS Organizations o CloudFront flat rate, ni crear dominio de pago.
- **API, DynamoDB, las dos EC2 Go simultáneas, balanceador ALB, Redis OSS y CloudFront 24×7** durante 45 días. No se elige modo 8h/día.
- Costos **brutos en créditos**, no cobro bancario; la cuenta AWS Free se cierra por fin de plan o créditos agotados. No garantía automática de uptime por estimación.

## Alternativas evaluadas

- A heredada: ALB público/EC2 públicas, Redis administrado; **103.408 USD** por 45 días, saldo estimado 16.592; reserva meta 30 FALLA.
- B heredada: 8h/día de EC2; **descartada** por disponibilidad.
- **C propuesta (aun pendiente)**: CloudFront HTTPS default domain + VPC origin ALB privado + dos EC2 privadas y Redis ElastiCache + DynamoDB + S3 gateway endpoints; costo estimado **84.058 USD/45d**, saldo **35.942**. AWS confirma VPC origins en Ohio y sin recargo por función; el acceso en cuenta Free y gastos de tráfico no fueron verificados en cuenta. CloudFront->ALB se plantea HTTP por red privada: **NO TLS de aplicación extremo a extremo**. Requiere aceptación expresa del responsable.

## Checklist y evidencia para firma (ninguna marcada como ya realizada)

- [ ] **Cuenta/Saldos**: `FREE/ACTIVE`, créditos actuales >= presupuesto 45d + reserva 30, expiración posterior a 45d desde inicio; no auto-upgrade.
- [ ] **EC2**: cuota vCPU >=4; oferta ARM t4g.micro en >=2 AZ y soporte 2 instancias, permisos de crear roles/SG/ASG.
- [ ] **ElastiCache**: motor Redis OSS exacto, clase/hora tarifa SKU oficial en Ohio, permisos Free/SCP compatibles. No aceptar precio Valkey.
- [ ] **CloudFront**: acceso a distribuir y a `VpcOrigin` en cuenta Free, no sólo que CloudFront sea servicio permitido; documento de origen privado y control de acceso.
- [ ] **TLS**: aprobación de HTTPS cliente→CloudFront y tramo HTTP privado a ALB **o** ruta de end-to-end HTTPS sin compra de dominio. Si profesor exige TLS extremo a extremo, bloqueo explícito hasta resolver.
- [ ] **Sin NAT**: entrega de binarios Go, rollback, parcheado SO y logs desde EC2 privadas viable mediante endpoints Gateway y AMIs, sin cargos extra no modelados.
- [ ] **FinOps**: crédito remanente, drift presupuestal, alarmas diarias y manual de incidentes; posibilidad de apagar sólo **no core** con aprobación humana. Modelo C comparado con presupuesto real de consola.
- [ ] **Consentimiento del autorizador**: selección consciente del escenario C, excepciones de TLS y riesgo de corte AWS Free explícitas.

## Acta — SIN APROBACIÓN

- **Decisión**: `PENDIENTE`.
- **Arquitectura seleccionada**: `PENDIENTE`.
- **Condición HTTPS entre CloudFront y ALB**: `PENDIENTE`.
- **Evidencia de cuenta**: `PENDIENTE`.
- **Firma humana/fecha**: `PENDIENTE`.

**Si y solo si el usuario aprueba inequívocamente este Gate**, habilitar diseño H06 completo con todos los endpoints trazables (no ejecución ni infraestructura H08). Hasta entonces se corrige **solo** H05. No se requiere validación/aprobación Mobile.


## Evidencia recibida el 2026-11-05 (sin inferir permisos no comprobados)

- Instalaciones H05-v2/v3 confirmadas por usuario: `VERIFY_PASS` con 13 y 11 archivos respectivamente; los hashes de H03/H04 no se reauditaron.
- Comprobación AWS leída del terminal del usuario: Free `READ_OK YES`; cuota EC2 estándar **16 vCPU**; ofrecimiento `t4g.micro` `READ_OK`; listado VPC origins `READ_OK`; captura de consola muestra `VPC origins (0)` y botón Create. Esto **no** verifica Create ni compatibilidad de toda la infraestructura.
- Precio Redis en consulta anterior devuelve **0.013, 0.016, 0.026 USD/h** sin identificación inequívoca del SKU. **No certificado**: se necesita lectura Price List filtrada por región, motor y nodo, o cotización AWS Pricing Calculator antes del primer `apply`.
- Crédito disponible de $120 indicado por el usuario: **no demostrado como saldo actual API**; el presupuesto de $84.058 se mantiene **estimación**, no garantía. Es necesario confirmar vencimiento >=45 días desde despliegue y saldo en facturación.
- Restricción del responsable: operación cotidiana exclusivamente por **IaC implementada luego por OpenCode**. No se permiten instrucciones de creación/apagado manual en consola ni `terraform apply` en H05.

### Gate propuesto: decisión humana requerida

`H05 = REVIEW_READY_CONDITIONAL / NOT_APPROVED` para comenzar diseño H06 **sólo después** de autorización expresa. Separar **aprobación documental H05** de **autorización de despliegue H08+**. La aceptación documental puede registrar riesgos observables de CloudFront y del SKU sin fingir pruebas de creación. El primer `apply` queda impedido hasta que verificaciones automáticas y presupuesto resulten PASS.

**No autorizadas:** actualización a Paid, creación de recursos, cierre de cuenta, modificación de IAM, borrado de DynamoDB o publicación GitHub.
