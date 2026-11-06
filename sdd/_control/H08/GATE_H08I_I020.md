# Gate H08-I — I-020 `computo-go` (propuesta de apertura)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/GATE_H08I_I020.md` |
| Fecha | 2026-11-06 |
| Spec | `sdd/02-infraestructura/I-020-computo-go/{spec,plan,tasks,checklist}.md` |
| Estado | **PROPUESTO — requiere aprobación humana antes de implementar** |
| Precondición | I-010 cerrada: PR #16 MERGED (`7039a39`), `main` al día |
| Rama preparada | `feat/h08i-i020-computo-go` (desde `main` actualizado) |

## 1. Alcance autorizable (implementación local)

Terraform y artefactos de cómputo, sin desplegar:

| Requisito | Implementación prevista |
|---|---|
| INF-020-01 cómputo | `aws_launch_template` + `aws_autoscaling_group` con `desired=2`, `min=2`, `max=2`, una instancia por AZ (I-010), `t4g.micro` ARM64, sin IP pública |
| INF-020-02 binario | Build Go `linux/arm64` con `-trimpath`, SHA-256 por release; unidad `systemd` con usuario no privilegiado, `NoNewPrivileges`, `ProtectSystem=strict`, `PrivateTmp`, `MemoryMax`, restart acotado, `TimeoutStopSec` para graceful shutdown |
| INF-020-03 bootstrap | `user-data` mínimo sin secretos; artefacto descargado por S3 con verificación de digest; configuración por entorno; rol de instancia (ARN como variable) |
| INF-020-04 salud | `/api/v1/health/live` y `/api/v1/health/ready` (ya en A-090); `target_group` con health check HTTP 8080; retiro por readiness |
| INF-020-05 simultaneidad | Garantizada por ASG `min=2` en AZ distintas; verificación real en el Gate de despliegue |
| INF-020-06 capacidad | `t4g.micro`: 2 vCPU/1 GiB; límites de memoria en systemd; cuota observada 16 vCPU (4 requeridas) |

## 2. Fuera de alcance

- `terraform apply/destroy`, `plan` contra AWS, creación de instancias, AMI, roles reales.
- Balanceador y HTTPS (I-030), DynamoDB (I-040), Redis (I-050), IAM (I-060).
- Despliegue del binario a AWS y verificación HTTP real (Gate de despliegue posterior).
- Endpoints E001–E075.

## 3. Archivos previstos

```
infra/terraform/modules/computo/{main.tf,variables.tf,outputs.tf}
infra/terraform/envs/h08i/main.tf                  (añade module "computo")
infra/terraform/envs/h08i/variables.tf
infra/systemd/nexum-backend.service
infra/systemd/nexum-backend.env.example            (sin secretos reales)
scripts/build-backend.ps1                          (cross-compile + SHA-256)
tests/infra/terraform/test_computo_contrato.py
tests/infra/systemd/test_unidad_endurecida.py
```

## 4. Pruebas previstas

1. Estáticas Terraform: ASG `min=max=desired=2`, una subred por AZ, sin IP pública, `instance_type` ARM64, `iam_instance_profile` presente, health check a `/api/v1/health/live`.
2. Unidad systemd: usuario no root, `NoNewPrivileges`, `ProtectSystem`, `PrivateTmp`, `Restart=` acotado, `TimeoutStopSec`, `MemoryMax`, `ExecStart` con ruta absoluta.
3. Bootstrap: el `user-data` no contiene tokens, claves ni contraseñas; el artefacto se verifica por digest.
4. Build: `GOOS=linux GOARCH=arm64 go build` + SHA-256 reproducible (local).
5. Regresión: FinOps, SDD, H06, permisos, `go build/vet/test`.
6. `terraform fmt -check` y `terraform validate` (permisos ya aprobados).

## 5. Permisos solicitados

- Escritura en `infra/terraform/**`, `infra/systemd/**`, `scripts/**` y `tests/infra/**`.
- `terraform fmt -check`, `terraform init -backend=false`, `terraform validate` (ya autorizados en I-010).
- `go build` para `linux/arm64` y cálculo de hashes.
- Git: rama propia, commit, push, PR.

**Denegado:** `plan/apply/destroy`, cualquier `aws` mutante, creación de AMI/roles/instancias.

## 6. Dependencias y condiciones

- **I-010** (cerrada): subredes privadas, SG `sg-backend` (8080), `sg-alb` (80), endpoints.
- **I-030** consumirá el `target_group` y el ALB; I-020 sólo publica el puerto 8080 en el SG existente.
- **I-060** debe definir el rol de instancia real; aquí sólo se referencia el perfil por variable.
- **Dependencia bloqueante de despliegue (arrastrada de I-000/I-010):** el mecanismo de secretos
  (caché cifrada en EBS + ventana) debe demostrarse antes de desplegar; I-020 no lo resuelve.

## 7. Coste

Sin partidas nuevas sobre el diseño E: `t4g.micro` ×2 ya está en el presupuesto (18.1440 USD/45 d),
EBS gp3 30 GB (2.4000) y no hay IP pública. Presupuesto vigente: A 113.0613 · B **118.5613** ≤ 120.

## 8. Aprobación solicitada (una sola)

- `APRUEBO_GATE_I020_LOCAL` → implemento Terraform de cómputo, unidad systemd, script de build y pruebas
  estáticas, sin `plan`/`apply` ni recursos AWS.
- `APRUEBO_GATE_I020_LOCAL_Y_VALIDACION_TERRAFORM` → añade `fmt/validate` (ya habilitados).

Cualquier `plan`/`apply` contra la cuenta es una autorización distinta que no se solicita aquí.
