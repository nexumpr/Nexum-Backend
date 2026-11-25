# Propuesta de egreso FCM — comparativa y recomendación condicionada

**Estado:** `PROPUESTA — NO AUTORIZADA — SIN RECURSOS CREADOS`.
**Fecha:** 2026-11-24. **Alcance:** backend Go en subredes privadas que necesita
`oauth2.googleapis.com` y `fcm.googleapis.com` (FCM HTTP v1) para el outbox push
y el scheduler de recordatorios.

## 1. Hechos verificados y límites

- La VPC no tiene NAT ni IPv4 pública en las EC2. El IGW existe y está adjunto,
  pero **no** hay ruta pública (I-010).
- El security group del backend **no** tiene egreso a Internet en 443; sólo S3,
  DynamoDB, Redis e interface endpoints. Añadir una ruta NAT sin tocar el SG no
  habilita FCM.
- Créditos verificados por lectura `freetier get-account-plan-state` el
  2026-11-24: **130.491975 USD**, plan `FREE`/`ACTIVE`, expiración 2027-05-04.
  No es un presupuesto asignado; agotarlos puede cerrar la cuenta.
- Precios de la tabla: lista pública de `us-east-2`, **no revalidados con Price
  List API en esta sesión**. `AWS_BILLING_NOT_VERIFIED`.
- Horizonte: 8 días hasta la evaluación del 2026-12-02; 45 días de convención
  FinOps; 161 días hasta la expiración del plan Free.

## 2. Comparativa

### Opción A — NAT Gateway gestionado (1 AZ o 2 AZ)

| Concepto | Coste |
|---|---|
| Horas NAT Gateway | 0.045 USD/h por AZ |
| IPv4 pública del NAT | 0.005 USD/h por IP (si AWS la factura separada; verificar factura) |
| Tráfico procesado | 0.045 USD/GB |
| Salida a Internet | 0.09 USD/GB después de 100 GB/mes (FCM es << 100 GB) |

| Horizonte | 1 AZ fijo | 2 AZ fijo | 1 GB tráfico | % de 130.492 (1 AZ) |
|---|---:|---:|---:|---:|
| 8 días (192 h) | 9.60 USD | 19.20 USD | 0.05 USD | 7.4 % |
| 45 días (1 080 h) | 54.00 USD | 108.00 USD | 0.05 USD | 41.4 % |
| 161 días (3 864 h) | 193.20 USD | 386.40 USD | 0.05 USD | 148 % (excede) |

- **Seguridad:** gestionado, sin SSH ni EC2 pública; el NAT no filtra por
  dominio, así que el SG debe abrir 443 a Internet o a un prefijo amplio.
- **Disponibilidad:** 1 AZ es punto único; 2 AZ duplica coste. Hoy ambas EC2
  están en `us-east-2a`, por lo que 1 AZ no añade un fallo nuevo efectivo.
- **Complejidad:** baja. **Terraform:** subred pública, route table, IGW route,
  EIP, NAT, ruta privada y regla SG opcional (`modules/egreso_fcm`).
- **Riesgos:** coste horario continuo; olvidar destruirlo; egreso amplio;
  dependencia de que el NAT tenga ruta pública correcta.

### Opción B — NAT instance (t4g.nano/t4g.micro)

| Concepto | Coste |
|---|---|
| t4g.nano Linux | 0.0042 USD/h |
| t4g.micro Linux | 0.0084 USD/h |
| IPv4 pública | 0.005 USD/h |
| EBS gp3 8 GB | 0.08 USD/GB-mes ≈ 0.64 USD/mes |
| Salida a Internet | 0.09 USD/GB después de 100 GB/mes |

| Horizonte | 1 t4g.nano | 2 t4g.nano (HA) | % de 130.492 (1) |
|---|---:|---:|---:|
| 8 días | ≈ 1.94 USD | ≈ 3.87 USD | 1.5 % |
| 45 días | ≈ 10.90 USD | ≈ 21.79 USD | 8.4 % |
| 161 días | ≈ 39.00 USD | ≈ 78.00 USD | 29.9 % |

- **Seguridad:** hay una EC2 con IPv4 pública; exige parcheo, iptables,
  deshabilitar SSH público (SSM en ventana), y el mismo egreso amplio.
- **Disponibilidad:** 1 instancia es SPOF; 2 exigen una route table por AZ.
- **Complejidad:** media-alta (AMI, user data, hardening, parches, métricas).
- **Terraform:** subred pública, EIP, EC2, SG, IAM/SSM, route table.
- **Riesgos:** deriva de configuración, IP pública, superficie de ataque y
  responsabilidad operativa.

### Opción C — Proxy de egreso controlado (Squid/Envoy sobre t4g.nano)

| Concepto | Coste |
|---|---|
| t4g.nano + IPv4 + EBS 8 GB | ≈ 1.94 / 10.90 / 39.00 USD (8/45/161 d) |
| Logs CloudWatch (~100 MB/mes) | ≈ 0.05 USD/mes |
| Salida a Internet | 0.09 USD/GB después de 100 GB/mes |

- **Seguridad:** allowlist de hosts (`oauth2.googleapis.com:443`,
  `fcm.googleapis.com:443`) y CONNECT; no inspecciona TLS. Mejor que NAT para
  SSRF, pero añade una EC2 pública, parcheo y configuración de proxy.
- **Disponibilidad:** SPOF salvo dos proxies; hoy el backend ya está en una AZ.
- **Complejidad:** alta. **Terraform:** la misma base que NAT instance más
  configuración, certificados internos y logs.
- **Riesgos:** proxy mal configurado, bypass por IP directa, coste de operación.

### Opción D — Serverless (API Gateway/Lambda o EventBridge API Destinations)

| Concepto | Coste |
|---|---|
| Interface endpoint (`execute-api`/`lambda`/`events`) | 0.01 USD/h por AZ |
| 2 AZ, 45 días | 21.60 USD |
| API Gateway HTTP API | ≈ 1.00 USD/millón de peticiones |
| Lambda | ≈ 0.20 USD/millón + duración; carga FCM pequeña |
| Total orientativo 45 días | ≈ 23–26 USD (sin medir) |

- **Seguridad:** sin EC2 pública; allowlist en código; nuevas credenciales FCM
  en Secrets Manager y nuevo rol de ejecución.
- **Disponibilidad:** gestionada multi-AZ.
- **Complejidad:** alta: código nuevo, API privada o EventBridge, IAM, secretos,
  observabilidad y otro pipeline. EventBridge API Destinations no cubre
  necesariamente el OAuth2 de cuenta de servicio de Google.
- **Terraform:** endpoints, API/EventBridge, Lambda, IAM y secretos.
- **Riesgos:** nueva superficie y deuda; el backend seguiría necesitando el
  endpoint de AWS para invocar el componente.

### Opción E — Descartadas

- **AWS Network Firewall:** 0.395 USD/h + tráfico; ≈ 426 USD/45 días, 327 % de
  los créditos. No viable.
- **VPC endpoint para FCM:** Google no ofrece PrivateLink; no existe.
- **IPv6/egress-only IGW:** no resuelve FCM por IPv4 y no hay IPv6 en la VPC.
- **Abrir IPv4 pública a las EC2:** viola la topología y expone el backend.

## 3. Recomendación

**Opción A con 1 NAT Gateway, condicionada a que el horizonte real termine el
2026-12-02** (8 días, ≈ 9.60 USD, 7.4 % de los créditos). Es la única que no
añade EC2 pública ni SSH, tiene complejidad baja y se destruye con un cambio
Terraform revisado.

Condiciones del Gate FinOps:

1. `APRUEBO_GATE_FINOPS_FCM_NAT_1AZ` con horizonte explícito y fecha de
   destrucción. Si el horizonte supera el 2026-12-02, **no** se aprueba NAT:
   se elige proxy controlado (Opción C) o se difiere el push.
2. `APRUEBO_EGRESO_FCM_SEGURIDAD_443`: aceptar el egreso 443 sin allowlist de
   dominio o sustituirlo por proxy. Sin esta frase, `habilitar_regla_egreso_backend`
   permanece en `false` y FCM no funciona.
3. Plan Terraform custodiado y revisado por SHA-256; sin `apply` automático.
4. Destrucción programada del NAT/EIP al cerrar la ventana; el módulo debe
   volver a `habilitar = false`.
5. No se crea nada mientras no existan las tres frases y el plan autorizado.

Si el proyecto debe seguir hasta 2027-05-04, el NAT 1 AZ consume 148 % de los
créditos: **no es viable**. En ese escenario la recomendación es proxy
controlado (≈ 39 USD/161 días) o diferir FCM hasta que exista presupuesto.

## 4. Cambios Terraform preparados

- Módulo `infra/terraform/modules/egreso_fcm` con `habilitar = false` por
  defecto: no crea NAT, EIP, subred, ruta ni regla.
- No está instanciado en `envs/h08i`; no cambia el plan actual.
- Pruebas estructurales: `tests/infra/terraform/test_egreso_fcm_contrato.py`.
- El módulo no crea EC2, no abre SSH y no asigna IPv4 pública al lanzar.
