# Gate H08-E / E005 — Endpoint SES API v2 (PrivateLink)

**ESTADO:** `PREPARADO — PENDIENTE DE APROBACIÓN HUMANA` (2026-11-17).

No se ha creado ningún recurso. Este documento no es una firma: la autorización debe
pronunciarse con la frase exacta del apartado 6.

## 1. Motivo

Las dos EC2 del backend están en subredes privadas sin NAT. La API de Amazon SES v2
(`email.us-east-2.amazonaws.com`) no es alcanzable sin un interface endpoint. El endpoint
SMTP (`email-smtp`) **no** sirve para el SDK v2 y no se usa.

## 2. Plan autorizado

| Campo | Valor |
|---|---|
| Archivo | `C:\Nexum-Custodia\H08E-E005-20261117\ses-endpoint.tfplan` |
| **SHA-256** | **`2B91C6C85204A465C176042FBB4F392B5288B0810CCE2D6829C20BDB7E8E9974`** |
| Tamaño | 185 068 bytes |
| Fecha | `2026-11-17T03:16:09Z` |
| Terraform | 1.16.6 · provider `hashicorp/aws` 5.100.0 |
| Backend | `s3://nexum-h08i-tfstate-713797602090/envs/h08i/terraform.tfstate` |
| Comprobación JSON | `applyable=true`, `complete=true`, `errored=false` |
| Acciones | **1 create · 1 update · 0 delete · 0 replace · 0 import** |

## 3. Cambios exactos

1. **Crear** `module.red.aws_vpc_endpoint.ses[0]`:
   - `service_name = com.amazonaws.us-east-2.email` (SES API v2; **no** `email-smtp`).
   - `vpc_endpoint_type = Interface`.
   - `vpc_id = vpc-0d090f7cc195c701f`.
   - `subnet_ids = [subnet-073a7408c84371c6b (us-east-2b), subnet-07773fedf3b74e518 (us-east-2a)]`.
   - `security_group_ids = [sg-0f643e89e2daa488e]` (SG `endpoints` existente).
   - `private_dns_enabled = true`.
   - Etiquetas `Name=nexum-h08i-vpce-ses`, `Modo=permanente`, `Spec=E005-SES`.
2. **Actualizar** `module.redis.aws_elasticache_user.default_deshabilitado`:
   - `authentication_mode.type` `no-password` → `no-password-required`.
   - `access_string` permanece `off ~* -@all`; es la normalización conocida del proveedor
     documentada en `GATE_H08E_E005_A10.md`, sin cambio efectivo de autenticación.
   - El usuario de aplicación, su contraseña, el grupo y TLS no cambian.
3. Sin cambios en NAT, VPC, subredes, NACL, ALB, CloudFront, DynamoDB, IAM, LT, ASG,
   buckets ni secretos. Sin destrucciones ni reemplazos.

## 4. Conectividad y seguridad

- El SG `endpoints` ya tiene ingress 443 desde el SG del backend (`sg-05c4632e1bf06f891`).
- El SG del backend ya tiene egress 443 hacia el SG `endpoints`.
- La NACL privada ya permite 443 de salida y efímeros de retorno.
- `private_dns_enabled = true` permite que el SDK resuelva `email.us-east-2.amazonaws.com`
  hacia las ENI del endpoint sin cambiar código ni configuración de la aplicación.
- No se abre ningún puerto a Internet; el endpoint es interno a la VPC.

## 5. FinOps — gasto nuevo y excepción al margen

| Concepto | Valor |
|---|---|
| Precio interface endpoint | 0.01 USD/h por AZ (Price List, I-000) |
| AZs | 2 (us-east-2a, us-east-2b) |
| **Coste recurrente** | **0.02 USD/h** |
| Coste diario | 0.48 USD/día |
| 34 días (17-nov a 21-dic, ventana I-000) | **16.32 USD** |
| 45 días | **21.60 USD** |
| Transferencia | 0.01 USD/GB de procesamiento; despreciable para correos de recuperación |
| Margen FinOps vigente de I-000 | **6.9387 USD** |
| Exceso sobre el margen | **9.3813 USD (34 d)** / **14.6613 USD (45 d)** |
| Créditos AWS verificados en I-000 | 120.0000 USD (2026-11-06); saldo actual no reconsultado |
| Gasto personal | **0.0000 USD**; el gasto se imputa a créditos AWS |

La aprobación debe aceptar explícitamente este gasto nuevo con cargo a créditos AWS y la
**excepción al margen FinOps vigente**. No se autoriza ningún otro recurso facturable, NAT,
IPv4 pública ni cambio de plan Free.

## 6. Frase de aprobación requerida

```text
APRUEBO_SES_ENDPOINT_2AZ_SHA_2B91C6C85204A465C176042FBB4F392B5288B0810CCE2D6829C20BDB7E8E9974
```

Alcance: aplicar **solo** el plan `ses-endpoint.tfplan` con el SHA-256 anterior, aceptando
0.02 USD/h con cargo a créditos AWS y la excepción al margen FinOps. No autoriza merge,
despliegue, E006 ni otros recursos.

## 7. Ejecución

| Campo | Valor |
|---|---|
| Comando previsto | `terraform apply C:\Nexum-Custodia\H08E-E005-20261117\ses-endpoint.tfplan` |
| Perfil | `nexum-tf-bootstrap` (credenciales temporales) |
| Resultado | _Pendiente de la aprobación y ejecución_ |
