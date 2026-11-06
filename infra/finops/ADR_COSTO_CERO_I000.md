# ADR — Costo cero, IGW/VPC Origins y TLS para I-000

- **ID:** `ADR-COSTO-CERO-I000`
- **Fecha:** 2026-11-06 (revisión 3) · **Fase:** H08-I · **Spec:** I-000-cuenta-finops
- **Estado:** `PROPUESTO — BLOCKED_FINOPS` (no aprobado; no autoriza aprovisionamiento)
- **Aprobador requerido:** responsable humano del Backend

## 1. Contexto

Backend en AWS con **costo cero** y **cero pesos de dinero personal**, sin pasar a Paid y sin
comprar dominio. Topología C: CloudFront (`*.cloudfront.net`) → VPC Origin → ALB privado → dos EC2
privadas en AZ distintas, Redis OSS, DynamoDB on-demand, gateway endpoints S3/DynamoDB, 45 días 24×7.

## 2. Evidencia AWS verificada (2026-11-06, perfil `nexum`, sólo lectura)

| Hecho | Valor | Estado |
|---|---|---|
| Plan | `FREE` / `ACTIVE`, expira 2027-05-04 | `VERIFIED_AWS` |
| Créditos restantes | 120.0 USD (`accountPlanRemainingCredits`) | `VERIFIED_AWS` |
| Cuota vCPU On-Demand | 16 (dos `t4g.micro` = **4 vCPU**, no 2) | `VERIFIED_AWS` |
| `t4g.micro` | us-east-2a / 2b / 2c | `VERIFIED_AWS` |
| Redis OSS | `cache.t4g.micro` redis 7.1, 0.016 USD/nodo-hora | `VERIFIED_AWS` |
| Precios | **22 SKU** por Price List API | `VERIFIED_AWS` |
| Partidas sin cargo | certificado CloudFront, VPC Origins, gateway endpoints | `DOC_OFICIAL` |
| Costo bruto C | 84.0430 USD (verificado 79.0430) | calculado/verificado |
| Cobertura con margen 10 % | requerido 86.9473 ≤ 120 → **TOTAL** | calculado |
| Costo observado facturado | Desconocido; Cost Explorer no ejecutado | `NOT_RUN` |
| IVA | `UNKNOWN` | `NOT_VERIFIED` |
| Elegibilidad VPC Origins | No acreditada por `list-vpc-origins` | `NOT_VERIFIED` |

## 3. Internet Gateway y VPC Origins (corrección de diseño)

La documentación oficial de CloudFront exige, para crear un VPC origin:

1. **Internet Gateway adjunto** a la VPC. No se usa para enrutar; sólo denota que la VPC puede
   recibir tráfico de internet. **No implica rutas públicas, NAT ni IPv4 públicas.**
2. Subred **privada** con al menos **una IPv4 libre** (sin costo) para la ENI que crea CloudFront.
3. Origen (ALB) con **security group** que permita inbound desde la prefix list
   `com.amazonaws.global.cloudfront.origin-facing` o desde el SG administrado
   `CloudFront-VPCOrigins-Service-SG`.
4. Región soportada: **us-east-2 figura en la lista oficial**.
5. Restricciones: no gRPC, no triggers de Lambda@Edge de origin.

**Distinciones que no deben confundirse:** Internet Gateway (requerido, gratis, sin rutas públicas)
≠ NAT Gateway (0 en el diseño, con costo) ≠ rutas públicas (0) ≠ IPv4 públicas de EC2 (0) ≠ acceso
privado de CloudFront al ALB (el túnel de VPC Origins).

El diseño se corrigió: `red.internet_gateway = true`, `red.rutas_publicas = false`, NAT 0, IPv4
públicas 0. La elegibilidad de la cuenta sigue `NOT_VERIFIED` porque `list-vpc-origins` sólo enumera
orígenes existentes (0) y **crear uno es una operación mutante no autorizada**.

## 4. TLS extremo a extremo: opciones

| Criterio | **A. HTTPS cliente→CloudFront + HTTP privado hasta ALB** | **B. HTTPS cliente→CloudFront + HTTPS hasta ALB** |
|---|---|---|
| Compatibilidad con VPC Origins | Sí (tráfico en túnel privado, sin internet) | Sí, con listener HTTPS en el ALB |
| Certificado necesario | Por defecto `*.cloudfront.net` (gratis) | ACM en us-east-2 para el ALB **+ nombre de dominio propio** |
| Dominio propio | No | **Sí** (ACM no valida sin dominio controlado) |
| Costo | 0 USD | ACM público 0 USD, pero dominio/Route 53 > 0 USD |
| Cero gasto personal | Compatible | **Incompatible** salvo dominio ya poseído y pagado |
| Riesgo de seguridad | Tramo interno sin cifrar (aislado en VPC, sin tránsito público) | Cifrado de extremo a extremo |
| Cumplimiento estricto del SDD | Requiere aceptación formal de excepción | Cumple sin excepción |

**Recomendación:** opción **A** con **aceptación formal documentada** del responsable, porque B exige
un dominio de pago y rompe la regla de cero gasto personal. Si el responsable exige B, debe aportar
un dominio ya financiado (fuera del presupuesto del proyecto) y se recotiza. En ningún caso se
aprueba la excepción por decisión del agente: queda `TLS_NO_RESUELTO` hasta la firma humana.

## 5. Costo observado e impuestos

- **Cost Explorer** cuesta 0.01 USD por solicitud: no se ejecutó. Alternativa gratuita: consola de
  facturación → **Billing and Cost Management → Bills** (mes en curso) y **Free Tier** (uso por
  servicio). Campos a revisar: `Total net cost` del periodo y uso por servicio. Evidencia admisible:
  captura o exportación sanitizada (sin números de cuenta ni datos personales) o autorización
  expresa de una solicitud de Cost Explorer.
- **IVA:** no se determina desde las APIs consultadas. Distinguir (a) normativa fiscal oficial,
  (b) datos fiscales de la cuenta, (c) impuesto realmente facturado y (d) suposición. No se imputa
  0 %; se requiere determinación responsable o factura real.

## 6. Decisión

1. **`BLOCKED_FINOPS`** con 5 motivos: IVA no determinado, elegibilidad de VPC Origins no confirmada,
   costo observado no verificado, riesgo de cobro personal asociado y TLS sin resolver.
2. **No se aprueba aprovisionamiento.** Sin recursos, presupuestos, alarmas ni Terraform.
3. **Cobertura de créditos `TOTAL`** con el bruto verificado y margen 10 % (86.9473 ≤ 120), pero eso
   **no** convierte la decisión en `VIABLE`: los cinco bloqueos siguen abiertos.
4. Se rechaza B (8 h/día) como entorno requerido y A queda como comparador de mayor costo.
5. El riesgo de cobro personal se mantiene `True` porque el IVA no está determinado.

## 7. Riesgos residuales

1. Créditos: 120 USD verificados no garantizan cubrir 45 días si el tráfico real supera el modelo.
2. Variables: LCU, egreso, solicitudes CloudFront y transferencia inter-AZ dependen del uso real.
3. Apagado no elimina el costo: persisten **6.8790 USD** (datos, backups, logs, secretos, alarmas).
4. Creación de VPC Origins y ALB requiere autorización mutante posterior.
5. TLS: si se exige B, aparece costo de dominio.
6. Cost Explorer: sin autorización, "cero gasto observado" sigue sin demostrar.

## 8. Consecuencias

I-000 no cierra e I-010 no inicia hasta que el responsable: (1) determine IVA, (2) acepte la opción
TLS A o aporte dominio para B, (3) autorice la comprobación de elegibilidad de VPC Origins en fase de
despliegue, (4) aporte evidencia de facturación o autorice Cost Explorer, y (5) firme el cierre.
