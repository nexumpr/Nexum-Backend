# Propuesta de egreso FCM/OAuth — NAT Gateway administrado en 1 AZ

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/PROPUESTA_EGRESO_FCM.md` |
| Versión | 2.0.0 (sustituye la comparativa 1.x sin decisión) |
| Fecha | 2026-11-24 |
| Estado | **`PROPUESTA_APROBADA_PARA_DISENO_E_IAC` — NO APLICADA, SIN RECURSOS CREADOS** |
| Topología elegida | **NAT Gateway administrado, una sola AZ, subred pública nueva y dedicada, EIP único, IGW existente** |
| Gate registrado | `APRUEBO_FCM_EGRESO_FINOPS` (alcance **solo** diseño e IaC; **no** autoriza `terraform apply`) |
| Alcance funcional | Salida HTTPS 443 desde las dos EC2 privadas del backend hacia `oauth2.googleapis.com` y `fcm.googleapis.com` (FCM HTTP v1) para el outbox de push y el scheduler de recordatorios |
| Fuera de alcance | Cambiar la topología del ALB, abrir SSH, asignar IPv4 pública a las EC2, crear EC2 nuevas, tocar DynamoDB/Redis/secretos, publicar releases |
| Evidencia AWS de coste | `AWS_BILLING_NOT_VERIFIED`; precios NAT `SUPUESTO`/`NOT_VERIFIED` |

> Este documento **no** acredita recursos AWS, ni una aprobación de gasto, ni un
> `apply`. Describe una decisión de diseño y el procedimiento exacto con el que
> se activaría y se revertiría. La frase `APRUEBO_FCM_EGRESO_FINOPS` fue
> transmitida por el orquestador en la orden de esta unidad (2026-11-24) con
> alcance limitado a diseño/IaC; no se presenta aquí como firma del Scrum
> Master ni sustituye el Gate por SHA-256 del plan.

## 1. Hechos verificados en el árbol y en la cuenta

1. **VPC privada sin NAT ni rutas a Internet.** El módulo `infra/terraform/modules/red`
   (I-010) declara `enable_dns_support = true` y `enable_dns_hostnames = true`
   (líneas 73–74), dos subredes privadas derivadas y **ninguna** subred pública.
   El comentario de cabecera lo dice explícitamente: «Sin NAT».
2. **Internet Gateway ya adjunto, sin ruta por defecto.** El IGW existe porque
   CloudFront VPC Origins lo exige (I-030/D2). Antes de esta propuesta no tenía
   ninguna ruta `0.0.0.0/0`, y la subred pública nueva es la primera del entorno.
3. **Gateway endpoints S3 y DynamoDB ya creados** (`aws_vpc_endpoint.s3`,
   `aws_vpc_endpoint.dynamodb`, sin cargo). Su ruta por prefix list es más
   específica que un `0.0.0.0/0`, por lo que un NAT **no** desvía ese tráfico.
4. **Interface endpoints**: `logs` permanente; `secretsmanager`, `ssm` y
   `ssmmessages` sólo con `ventana_mantenimiento_activa = true`; más el endpoint
   `com.amazonaws.us-east-2.email` (SES API v2) habilitado para E005. Ninguno
   sustituye a FCM.
5. **Egress actual del SG del backend**: 6379 a Redis, 443 a las prefix lists de
   S3 y DynamoDB, 443 al SG de los interface endpoints, y 8080/6379 internos.
   **No hay 443 a Internet**. Añadir sólo una ruta NAT sin regla de SG no
   habilita FCM: hacen falta las dos cosas.
6. **NACL privada ya permite el flujo**: egress 443 a `0.0.0.0/0` y efímeros
   1024–65535 a `0.0.0.0/0` (flujo stateless completo). El NAT no requiere
   cambiar la NACL privada.
7. **EC2 privadas y ALB interno**: las instancias se lanzan en subredes privadas
   con `map_public_ip_on_launch` desactivado en el módulo de cómputo y **sin
   SSH**; el ALB es `internal` y CloudFront llega por VPC Origin. Esta propuesta
   no cambia ninguno de esos tres hechos.
8. **Créditos verificados** por lectura `freetier get-account-plan-state` el
   2026-11-24: **130.491975 USD**, plan `FREE`/`ACTIVE`, expiración 2027-05-04.
   Es saldo, **no** presupuesto ni autorización de gasto; agotarlo puede cerrar
   la cuenta.
9. **Precios**: `IPV4_HORA` (0.005 USD/h) y `TRANSFERENCIA_INTERAZ_GB`
   (0.01 USD/GB) están `VERIFIED_AWS`; hora de NAT (0.045), procesamiento
   (0.045 USD/GB) y salida a Internet (0.09 USD/GB) son **`SUPUESTO`**. El SKU
   original `NAT_GATEWAY_HORA` sigue `UNKNOWN`. Detalle y aritmética en
   `MATRIZ_FINOPS_EGRESO_FCM.md`.
10. **No existe PrivateLink de Google**: no hay VPC endpoint para FCM/OAuth.
    Tampoco hay prefix list administrada por AWS con los rangos de Google.

## 2. Decisión

**NAT Gateway administrado en una sola AZ (`us-east-2a` por defecto)**, con:

- una **subred pública nueva y dedicada** sólo al NAT (`10.42.16.0/24`,
  `map_public_ip_on_launch = false`; no aloja EC2);
- una route table pública con `0.0.0.0/0 -> IGW existente`;
- **un** EIP y **un** NAT (`connectivity_type = "public"`);
- ruta `0.0.0.0/0 -> NAT` en la route table privada real de I-010;
- regla de egress **TCP 443** en el SG real del backend, a `0.0.0.0/0` mientras
  no exista una prefix list real de Google aportada por el operador.

```
EC2 privadas (2, sin IPv4 pública, sin SSH)
  └─ route table privada 0.0.0.0/0 ──> NAT Gateway (us-east-2a)
        └─ subred pública nueva (sólo NAT) ──> IGW existente
              └─ HTTPS 443 ──> oauth2.googleapis.com / fcm.googleapis.com
CloudFront ──> VPC Origin ──> ALB interno :80 ──> EC2 :8080   (sin cambios)
```

## 3. Por qué esta opción

| Criterio | NAT Gateway 1 AZ | Comentario |
|---|---|---|
| EC2 siguen privadas | **Sí** | No se asigna IPv4 pública a las instancias ni se abre SSH |
| ALB sigue interno | **Sí** | No se toca I-030 |
| Componentes nuevos | 1 NAT + 1 EIP + 1 subred + 2 rutas + 1 regla SG | 8 recursos AWS + 1 `terraform_data` local |
| Operación | Gestionado por AWS | Sin parcheo, sin AMI, sin iptables, sin proxy |
| Coste 8 días (base) | **9.6044 USD** | Bajo el límite propuesto de 20 USD para la certificación |
| Coste 45 días (base) | **54.0248 USD** | El total del entorno (178.4078) **no** cabe en 130.491975 |
| Reversión | `habilitar_egreso_fcm = false` + apply | Destruye NAT, EIP, subred, rutas y regla |
| Allowlist por dominio | **No** | NAT no filtra por host; riesgo residual declarado |

Es la única alternativa que no crea una EC2 con IP pública, no añade un proxy
que haya que operar y se destruye con un cambio de variable revisado.

## 4. Alternativas descartadas y su coste

Costes calculados con la misma aritmética `Decimal` y los mismos estados de
evidencia; **no** son facturas.

| Alternativa | Coste modelado | Por qué se descarta |
|---|---:|---|
| **NAT Gateway 2 AZ** | 19.20 USD / 8 d · 108.00 USD / 45 d (horas) | Duplica el coste fijo; hoy ambas EC2 están en `us-east-2a`, así que la segunda AZ no protege el tráfico real. El SPOF sigue existiendo por nodo si el ASG no se reparte. Añade 2 EIP y 2 subredes |
| **NAT instance (`t4g.nano`)** | ≈ 1.94 / 10.90 / 39.00 USD (8/45/161 d) | EC2 con IPv4 pública, parcheo, iptables, hardening y responsabilidad operativa; el ahorro no compensa el riesgo y exige SSM en ventana para administrarla |
| **NAT instance HA (2 × `t4g.nano`)** | ≈ 3.87 / 21.79 / 78.00 USD | Sigue creando EC2 públicas y ahora exige una route table por AZ; más superficie y más operación |
| **Proxy de egress (Squid/Envoy sobre `t4g.nano`)** | ≈ 1.94 + logs / 10.90 / 39.00 USD | Única opción con allowlist real de hosts, pero añade EC2 pública, certificados internos, configuración, bypass por IP directa y operación continua. Queda como **alternativa si el Gate de seguridad rechaza el 443 abierto** |
| **Serverless (API Gateway/Lambda/EventBridge API Destinations)** | ≈ 23–26 USD / 45 d (endpoint `execute-api`/`lambda`/`events` 0.01 USD/h/AZ + peticiones) | Código nuevo, IAM y secretos nuevos, otro pipeline y una API privada que el backend debe alcanzar; EventBridge API Destinations no cubre con certeza el OAuth2 de cuenta de servicio de Google |
| **AWS Network Firewall** | 0.395 USD/h ≈ 75.84 USD / 8 d · 426.60 USD / 45 d | 327 % de los créditos verificados; inviable |
| **VPC endpoint para FCM** | — | Google no publica PrivateLink; no existe el servicio |
| **IPv6 / egress-only IGW** | — | La VPC no tiene IPv6 y FCM/OAuth resuelven por IPv4 |
| **IPv4 pública directa en las EC2** | 0.005 USD/h por IP | Viola la topología, expone el backend y obliga a abrir ingress; rechazada por seguridad |

## 5. Punto único de fallo de egreso (declarado)

- **Un NAT en una AZ es SPOF de egreso.** Si `us-east-2a` (o el NAT) falla, los
  dos nodos siguen sanos y sirviendo API por CloudFront/ALB, pero **no pueden**
  renovar el token de Google ni enviar push. El outbox acumula trabajos y los
  reintentos se agotan.
- No se mitiga con un segundo NAT porque duplica el coste fijo y el reparto real
  de AZ no está garantizado: hoy ambas instancias corren en `us-east-2a`
  (limitación de capacidad `t4g.micro` en `us-east-2b`, registrada en
  `memoria.md`). Un NAT en `2b` no ayudaría a instancias que no están allí.
- **Mitigación aceptada**: el outbox es durable en DynamoDB; un fallo de egreso
  produce reintentos con backoff y `failed` visible, no pérdida de datos. El
  scheduler puede recuperar el retraso al volver el NAT. No se promete entrega
  durante la caída.
- **No se declara alta disponibilidad de egreso.** Queda como decisión abierta
  si el proyecto exige `TWO_AZ_EGRESS` en el futuro.

## 6. Configuración concreta del módulo

Módulo: `infra/terraform/modules/egreso_fcm` (`terraform { required_version >= 1.4.0, < 2.0.0 }`,
provider `hashicorp/aws ~> 5.72`). Instanciado en `infra/terraform/envs/h08i/main.tf`
con `count = var.habilitar_egreso_fcm ? 1 : 0`.

### 6.1 Variables y valores

| Variable | Valor en `envs/h08i` | Efecto |
|---|---|---|
| `habilitar_egreso_fcm` | `false` | `count = 0`: no crea nada y no cambia el plan |
| `egreso_fcm_az` | `"us-east-2a"` | AZ única del NAT y de su subred |
| `egreso_fcm_cidr_publica` | `"10.42.16.0/24"` | Subred pública nueva, fuera de `10.42.0.0/24` y `10.42.1.0/24` |
| `egreso_fcm_cidr_https` | `"0.0.0.0/0"` | Destino del egress 443 cuando no hay prefix list |
| `egreso_fcm_prefix_list_id` | `""` | Si se aporta un `pl-*` real, se usa en lugar del CIDR |
| `vpc_id` | `module.red.vpc_id` | VPC existente |
| `internet_gateway_id` | `module.red.internet_gateway_id` | IGW existente |
| `private_route_table_ids` | `[module.red.route_table_privada]` | Recibe `0.0.0.0/0 -> NAT` |
| `backend_security_group_id` | `module.red.security_groups["backend"]` | Recibe el egress 443 |

### 6.2 Recursos que crea al habilitarse

| # | Recurso | Nota |
|---:|---|---|
| 1 | `aws_subnet.publica_nat` | `map_public_ip_on_launch = false`, sólo NAT |
| 2 | `aws_route_table.publica_nat` | Tabla pública dedicada |
| 3 | `aws_route.publica_internet` | `0.0.0.0/0 -> IGW` |
| 4 | `aws_route_table_association.publica_nat` | Asocia la subred nueva |
| 5 | `aws_eip.nat` | Único EIP, `domain = "vpc"` |
| 6 | `aws_nat_gateway.nat` | `connectivity_type = "public"` |
| 7 | `aws_route.privada_nat` | Una por route table privada (hoy 1) |
| 8 | `aws_vpc_security_group_egress_rule.backend_https_*` | 443 a CIDR **o** a prefix list (excluyentes) |
| — | `terraform_data.preconditions` | 9 preconditions locales; no crea infraestructura |

**Preconditions bloqueantes** (si fallan, el plan no produce cambios): VPC
presente, IGW presente, SG del backend presente, al menos una route table
privada, AZ coherente con `region`, subred `/27` o mayor, CIDR válido o prefix
list, y no declarar CIDR distinto de `0.0.0.0/0` cuando se aporta prefix list.

### 6.3 Reglas de seguridad que la activación debe conservar

1. **Las dos EC2 siguen privadas**: no se asigna IPv4 pública, no se cambia el
   Launch Template, no se abre SSH. La subred del NAT no aloja instancias.
2. **El ALB sigue `internal`**: CloudFront llega por VPC Origin; no se crea
   listener 443 ni se expone el ALB a Internet.
3. **El único elemento público es el EIP del NAT**, no las instancias.
4. **DNS ya está habilitado** en la VPC; sin `enable_dns_support`/`hostnames`
   los nombres de Google no resolverían. El módulo no muta la VPC.
5. **Riesgo residual**: `0.0.0.0/0:443` permite cualquier destino HTTPS, no sólo
   Google. Se acepta sólo si el código no permite URLs controladas por el
   usuario y no hay fetch arbitrario (hoy el backend no expone esa capacidad).
   La aceptación formal es la frase `APRUEBO_EGRESO_FCM_SEGURIDAD_443`, que
   **sigue pendiente** y es independiente de `APRUEBO_FCM_EGRESO_FINOPS`.

## 7. Procedimiento de activación (no ejecutado)

Cada paso exige su autorización. Ninguno se ejecuta en esta unidad.

1. **FinOps**: recalcular `infra/finops/egreso_fcm.py --creditos-verificados <saldo>`
   y comparar contra el saldo verificado del día. Si el total a 45 días supera
   el saldo, el Gate debe limitar el horizonte (8 días) o rechazar NAT.
2. **Seguridad**: obtener `APRUEBO_EGRESO_FCM_SEGURIDAD_443` o aportar una
   prefix list real (`egreso_fcm_prefix_list_id`) que restrinja los rangos.
3. **Cambio mínimo**: `habilitar_egreso_fcm = true` (y, si aplica, AZ, CIDR y
   prefix list). No se edita ningún otro módulo.
4. **Plan custodiado**:
   `terraform plan -input=false -var-file <tfvars-privado> -out <ruta-custodia>\egreso-fcm.tfplan`
   (forma separada de argumentos: el envoltorio rompe `-out=...`, D-BOOT-01).
   Revisar: **8 create, 0 destroy, 0 replace, 0 import**, ningún cambio en EC2,
   ASG, ALB, CloudFront, DynamoDB, Redis, bucket de secretos ni IAM.
5. **Gate por artefacto**: `APRUEBO_APPLY_EGRESO_FCM_SHA256_<HEX>` calculado
   sobre el `.tfplan` exacto. Sin esa frase no se aplica.
6. **Apply** del plan exacto, sin regenerarlo:
   `terraform apply <ruta-custodia>\egreso-fcm.tfplan`.
7. **Verificación de infraestructura** (lectura):
   - `aws ec2 describe-nat-gateways` → `State = available`, 1 NAT, AZ esperada;
   - `aws ec2 describe-route-tables` → `0.0.0.0/0` con `NatGatewayId` en la
     route table privada, y `0.0.0.0/0` con `GatewayId` en la pública;
   - `aws ec2 describe-security-group-rules` → una regla 443 nueva en el SG del
     backend, sin ingress nuevo;
   - `aws ec2 describe-instances` → las dos EC2 siguen sin `PublicIpAddress`.
8. **Verificación de egreso real** desde **cada** instancia, en ventana de
   mantenimiento (SSM sólo existe con `ventana_mantenimiento_activa = true`):

   ```bash
   getent ahosts oauth2.googleapis.com | head -1
   getent ahosts fcm.googleapis.com | head -1
   curl -sS -o /dev/null -w '%{http_code} %{remote_ip}\n' --max-time 5 https://oauth2.googleapis.com/token
   curl -sS -o /dev/null -w '%{http_code} %{remote_ip}\n' --max-time 5 https://fcm.googleapis.com/
   ```

   Criterio: cualquier código HTTP (400/404/405 son esperados en esas rutas)
   demuestra DNS + TCP 443 + TLS. `000`, timeout o error de resolución =
   **fallo de egreso**; no se declara éxito.
9. **Prueba de aplicación**: sólo cuando exista código de push/scheduler
   (Oleada 4/5), un envío a un dispositivo sintético y un `failed=0` en el
   outbox. Hoy **no** hay worker FCM ni scheduler en el árbol: no se puede
   declarar `FCM_DELIVERY_VERIFIED`.
10. **Registro**: anotar IDs de NAT/EIP/subred, SHA-256 del plan, fecha y
    responsable. Sin valores de secretos.

## 8. Rollback

1. `habilitar_egreso_fcm = false`.
2. Plan nuevo y Gate por SHA-256 (`APRUEBO_APPLY_EGRESO_FCM_SHA256_<HEX>`).
3. Apply: destruye la regla 443, la ruta privada, el NAT, el EIP, la asociación,
   la route table pública y la subred. **No** toca EC2, ASG, ALB, CloudFront,
   DynamoDB, Redis, secretos ni state.
4. Verificar: 0 NAT `available`, ruta `0.0.0.0/0` ausente en la privada, EIP
   liberado, regla 443 eliminada, subred eliminada.
5. Consecuencia esperada: el push/OAuth deja de funcionar de inmediato; las
   conexiones abiertas se cortan. El outbox permanece y se reintenta cuando se
   reactive.

Orden de destrucción correcto (lo resuelve Terraform): regla SG y ruta privada
antes del NAT; NAT antes del EIP; EIP antes de la subred. No se borra nada a
mano.

## 9. Coste

Resumen (detalle y fórmulas en `MATRIZ_FINOPS_EGRESO_FCM.md`):

| Horizonte | NAT 1 AZ (base) | Alto (10×) | Límite 20 USD | Cabe en 130.491975 |
|---|---:|---:|---|---|
| 8 días | **9.6044 USD** | 9.6443 USD | Sí | Sí |
| 45 días | **54.0248 USD** | 54.2489 USD | n/a | **No** (el total del entorno es 178.4078) |

El coste **no** es cero: la hora del NAT domina (8.64 USD en 8 días). El
`apply` de esta propuesta debe ir acompañado de una fecha de reversión.

## 10. Riesgos y pendientes

| ID | Riesgo | Estado |
|---|---|---|
| R-EGR-01 | Precio de hora/procesamiento/egreso del NAT `SUPUESTO`; factura real no medida | Abierto; revalidar Price List antes del Gate de apply |
| R-EGR-02 | Facturación del EIP asociado al NAT no revalidada | Abierto; la matriz muestra con y sin ese cargo (0.96 USD/8 d) |
| R-EGR-03 | Egress 443 sin allowlist de dominio | Aceptación humana pendiente (`APRUEBO_EGRESO_FCM_SEGURIDAD_443`) |
| R-EGR-04 | SPOF de egreso en una AZ | Declarado y aceptado para 8 días; no se declara HA |
| R-EGR-05 | 45 días no cubiertos por créditos (178.4078 > 130.491975) | Abierto; limita la ventana |
| R-EGR-06 | Saldo de créditos puede cambiar y no es autorización | Regla permanente |
| R-EGR-07 | Sin código FCM/scheduler, no hay prueba de entrega | `FCM_DELIVERY_VERIFIED = NOT_RUN` |
| R-EGR-08 | `AWS_COST_REAL`, IVA e `AWS_BILLING_NOT_VERIFIED` | Abiertos desde I-000 |

## 11. Registro de Gate

| Frase | Alcance concedido | Qué **no** autoriza |
|---|---|---|
| `APRUEBO_FCM_EGRESO_FINOPS` | Diseño de la topología NAT 1 AZ, escritura del módulo `egreso_fcm`, variables y documentación | `terraform plan`, `terraform apply`, crear NAT/EIP/subred, publicar release, desplegar |
| `APRUEBO_EGRESO_FCM_SEGURIDAD_443` | **Pendiente** | Aceptar el egress sin allowlist |
| `APRUEBO_APPLY_EGRESO_FCM_SHA256_<HEX>` | **Pendiente**; se emitirá por plan | Cualquier cambio fuera del plan |
| `APRUEBO_ROTACION_CLAVES_FCM` | **Pendiente**; ver `PROCEDIMIENTO_ROTACION_CLAVES_FCM.md` | Rotar claves sin ventana |

**Estado de la propuesta:** `DISENO_CERRADO_IAC_PREPARADA · AWS_NOT_RUN ·
FCM_DELIVERY_VERIFIED=NOT_RUN · APLICACION_BLOQUEADA_POR_GATE_DE_PLAN`.
