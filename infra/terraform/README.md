# infra/terraform — I-010 red VPC

**Estado:** implementación local validada (`fmt` + `validate`). **Nada aplicado.** Sin backend
remoto, sin estado, sin credenciales ni identificadores de cuenta.

## 1. Estructura

```
infra/terraform/
├── modules/red/{main.tf,variables.tf,outputs.tf}
└── envs/h08i/{main.tf,variables.tf,terraform.tfvars.example,.terraform.lock.hcl}
```

- Módulo `red`: VPC, subredes privadas en 2 AZ, Internet Gateway, tabla de rutas privada, gateway
  endpoints S3/DynamoDB, 4 security groups, interface endpoints (1 permanente + 3 de ventana) y NACL.
- Entorno `h08i`: composición, provider AWS `~> 5.60`, `required_version >= 1.6`.

## 2. Diagrama

```
                 HTTPS 443
 Cliente  ──────────────────▶  CloudFront (*.cloudfront.net, D2)
                                   │  VPC Origin (túnel privado)
                                   │  HTTP 80
                                   ▼
                          ┌──────────────────┐
                          │ ALB internal     │  sg-alb
                          │ 80 ← prefix list │
                          └────────┬─────────┘
                                   │ HTTP 8080
                    ┌──────────────┴──────────────┐
                    ▼                             ▼
            ┌───────────────┐             ┌───────────────┐
            │ EC2 Go (2a)   │             │ EC2 Go (2b)   │  sg-backend
            └───┬───┬───┬───┘             └───┬───┬───┬───┘
                │   │   │ 6379                 │   │   │
                │   │   └────────────▶ Redis OSS (sg-redis, 6379 sólo backend)
                │   │ 443
                │   └──────────────▶ Interface endpoints (sg-endpoints)
                │                     · logs            (permanente)
                │                     · secretsmanager  (ventana)
                │                     · ssm, ssmmessages (ventana)
                │
                └──▶ Gateway endpoints S3 + DynamoDB (sin cargo, por tabla de rutas)
        Egress 443 del SG hacia prefix lists `s3` y `dynamodb` (obligatorio)
                     Internet Gateway adjunto: requerido por VPC Origins, SIN rutas públicas
```

## 3. Matriz origen → destino

| Origen | Destino | Puerto | Protocolo | Control |
|---|---|---|---|---|
| Internet | CloudFront | 443 | HTTPS/TLS | CloudFront (D2) |
| CloudFront | ALB | **80** | HTTP privado | SG `sg-alb` desde prefix list `cloudfront.origin-facing` |
| ALB | EC2 Go ×2 | **8080** | HTTP | SG `sg-backend` sólo desde `sg-alb` |
| ALB | EC2 Go ×2 | 8080 `GET /api/v1/health/live` | HTTP | health check |
| EC2 Go | Redis OSS | 6379 | TCP (TLS en tránsito según I-050) | SG `sg-redis` sólo desde `sg-backend` |
| EC2 Go | DynamoDB | 443 | HTTPS | gateway endpoint + IAM |
| EC2 Go | S3 | 443 | HTTPS | gateway endpoint + IAM |
| EC2 Go | logs / secretsmanager / ssm | 443 | HTTPS | interface endpoints `sg-endpoints` |
| Operador / CI | instancias | — | **sin SSH** | SSM en ventana |

## 4. Security groups y NACL

| Recurso | Ingress | Egress |
|---|---|---|
| `sg-alb` | TCP 80 desde prefix list de CloudFront | TCP 8080 a `sg-backend` |
| `sg-backend` | TCP 8080 desde `sg-alb` | 6379 a `sg-redis`; 443 a `sg-endpoints`; **443 a S3 y DynamoDB por prefix lists administradas** |
| `sg-redis` | TCP 6379 desde `sg-backend` | — |
| `sg-endpoints` | TCP 443 desde `sg-backend` | — |

Ningún SG tiene ingress desde `0.0.0.0/0`. Sin puerto 22.

NACL (stateless, una para ambas subredes, flujo completo por sentido):

- **ingress**: 80 (CloudFront→ALB), 8080 (ALB→EC2) y 6379 (EC2→Redis) desde el CIDR de la VPC,
  y **1024-65535** para el retorno de servicios.
- **egress**: **443 a `0.0.0.0/0`** (S3, DynamoDB, interface endpoints, Secrets Manager y SSM viven
  en rangos públicos de AWS), 8080 y 6379 al CIDR de la VPC, y **1024-65535** para el retorno hacia
  CloudFront/ALB/endpoints.

Las prefix lists administradas no pueden referenciarse en NACL; el mínimo privilegio fino se aplica
en los security groups (`prefix_list_id`), y la NACL garantiza que ningún flujo legítimo se corte.

## 5. Diseño de secretos (decisión D y riesgo residual)

1. **Obtención inicial:** en la ventana de despliegue, con `secretsmanager` activo, la instancia lee el
   secreto y lo escribe en un volumen **EBS cifrado** (cifrado transparente en el hipervisor: no hay
   llamada KMS desde la instancia) con permisos `0400` del usuario de la aplicación.
2. **Reinicio sin ventana:** la aplicación lee el archivo de la caché; no necesita endpoint.
3. **Rotación:** en ventana se actualiza el secreto en Secrets Manager y se reescribe la caché;
   el archivo anterior se sobrescribe.
4. **IAM:** la instancia sólo puede `secretsmanager:GetSecretValue` del ARN concreto; el ARN se pasa
   como variable, nunca el valor.
5. **Nunca** hay secretos en Git, `.tfvars` de ejemplo, AMI, logs ni Terraform state (sólo ARNs).
6. **Riesgo residual declarado:** quien tenga root en la instancia puede leer la caché (igual que la
   memoria del proceso); las snapshots EBS de ese volumen quedan cifradas y deben excluirse de
   copias compartidas.
7. **Pérdida/corrupción de caché:** la API no arranca hasta abrir ventana (~2 min para crear el
   endpoint). Se documenta como riesgo operativo, no se oculta.

## 6. Presupuesto (reconciliación con I-000)

| Escenario | Base | Conectividad | Margen 10 % | ≤ 120 |
|---|---|---|---|---|
| A (sólo verificado) | 79.0430 | 23.7400 | **113.0613** | sí |
| B (verificado + reservas) | 84.0430 | 23.7400 | **118.5613** | sí |

I-010 no añade partidas nuevas: reutiliza el diseño E. Cualquier endpoint adicional rompe la
cobertura (dos permanentes → 134.47).

## 7. Uso y límites

```powershell
terraform fmt -check -recursive
cd infra/terraform/envs/h08i
terraform init -backend=false        # única forma autorizada
terraform validate
# PROHIBIDO: plan/apply/destroy contra AWS, backend remoto, credenciales
```

`.terraform/`, `*.tfstate*` y `*.tfvars` reales están excluidos por `.gitignore`. El módulo no crea
recursos hasta que exista un Gate de despliegue con autorización específica.


## 10. Capacidad de subredes y versiones (auditoría final)

- Subredes **derivadas** con `cidrsubnet(var.cidr_vpc, var.bits_subredes, indice)` usando índices
  **0 y 1**: contención y ausencia de solape por construcción (antes 10 y 11, que limitaban los bits
  útiles).
- Política de tamaño: prefijo final entre **/20 y /27**. `/27` es el mínimo adoptado porque el ALB
  exige 8 direcciones libres y AWS reserva 5 por subred (32 − 5 = 27 utilizables); `/28` se rechaza
  por política de escalado. Con la VPC `/16` por defecto, `bits_subredes` 4..11 produce de `/20` a `/27`.
- Las validaciones cruzadas entre variables (prefijo final y coherencia AZ/región) se implementan con
  `terraform_data` + `lifecycle.precondition`, compatibles con Terraform **1.4+**; no se usan
  referencias entre bloques `validation` (requerirían 1.9). `required_version = ">= 1.4.0, < 2.0.0"`.
- `.terraform.lock.hcl` se versiona para reproducibilidad (aws ~> 5.60, resuelto 5.100.0). `.terraform/`
  y los estados siguen excluidos.
- Capacidad: /24 por AZ (251 utilizables) cubre ALB (1 IP por AZ y escalado), 1 EC2, 4 ENI de interface
  endpoints y la ENI que crea CloudFront para el VPC Origin. Las direcciones realmente libres sólo se
  confirman en el despliegue autorizado (`NOT_RUN_AWS`).
