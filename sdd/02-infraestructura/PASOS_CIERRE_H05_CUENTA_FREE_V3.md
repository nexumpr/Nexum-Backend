# Pasos de cierre H05 — lectura dirigida, sin tocar AWS

**H05 NO ESTÁ APROBADO.** Ejecutar únicamente después de instalar este paquete. AWS CLI está configurado como `nexum-login` y zona `us-east-2`. Comandos de README son compatibles con Warp PowerShell 7; su salida es sanitizada. No hacer `terraform apply`, `aws ec2 run-instances`, `aws cloudfront create-*`, `aws billing upgrade`, `git push`, compra de dominio ni activar CloudFront tarifa plana.

1. `py -3.13 $z --aws-check` registra `Free/ACTIVE` y créditos, cuota de vCPU, oferta de dos AZ y respuesta de `cloudfront list-vpc-origins` sin detalles del nombre/número de cuenta. Opcionalmente re-ejecutar `--aws-check` del paquete V2 si se requiere más detalle de los SKU Redis y compartir el JSON redactado. `ListVpcOrigins` correcto **NO confirma** que `CreateVpcOrigin` esté permitido por SCP/Free.
2. En consola AWS Billing/Cost Management (sin cambios), confirmar Free y saldo actual visible, fecha de vencimiento posterior a la operación prevista; registrar fecha y monto en evidencia **no pública**. No compartir números de cuenta, tokens, ARN íntegros ni datos fiscales.
3. En consola CloudFront (NO pulsar Create), buscar sección `VPC origins` y estado de acceso de la cuenta Free. Si AWS pide `Upgrade` o plan Flat-Rate, DETENER: opción C bloqueada. La pantalla tampoco confirma permiso Create.
4. Revisar criterio docente: ¿exige HTTPS válido hasta ALB, o basta HTTPS externo cliente→CloudFront con origen privado HTTP? Si exige end-to-end, C se mantiene bloqueado.
5. En cuanto se disponga de pruebas, rellenar `GATE_H05_REVISION_PENDIENTE.md` **solo mediante solicitud de cambio autorizada**: no marcar checks automáticos ni aprobar H05 por sí solo. No abrir H06 antes de firma expresa del responsable.

## Riesgo de uso de créditos

Las alarmas/avisos no garantizan que la cuenta permanezca activa. Un consumo inesperado de créditos puede cerrarla antes del día 45, aunque no exista cobro bancario. Si el pronóstico no conserva el saldo necesario, pedir rediseño humano, nunca Paid. No apagar DynamoDB, API, ALB, Redis ni CloudFront durante la ventana de 45 días.
