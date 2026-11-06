# Gate H08-I — I-010 `red-vpc` (propuesta de apertura)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/GATE_H08I_I010.md` |
| Fecha | 2026-11-06 |
| Spec | `sdd/02-infraestructura/I-010-red-vpc/{spec,plan,tasks,checklist}.md` |
| Estado | **PROPUESTO — requiere aprobación humana antes de implementar** |
| Precondición | I-000 cerrada: PR #15 MERGED (`585df4b`), D6 efectiva |
| Rama preparada | `feat/h08i-i010-red-vpc` |

## 1. Alcance autorizado (si se aprueba)

Implementación **local** del diseño de red conforme a INF-010-01..06:

1. VPC dedicada no superpuesta y dos subredes privadas en AZ distintas.
2. **Internet Gateway** adjunto (requisito de CloudFront VPC Origins, sin rutas públicas).
3. Tablas de rutas privadas sin ruta por defecto a internet; `rutas_publicas = false`.
4. **Gateway endpoints** S3 y DynamoDB (sin cargo).
5. `logs` como único interface endpoint permanente; `secretsmanager`, `ssm`, `ssmmessages` como
   recursos temporales de ventana (diseño E aprobado en I-000).
6. Security groups: ALB 443 desde CloudFront (prefix list), backend sólo desde SG del ALB, Redis sólo
   desde SG del backend; NACL de salida permite puertos efímeros a las IP de CloudFront.
7. Ausencia justificada de NAT, de IPv4 públicas en EC2 y de interface endpoints redundantes.
8. Matriz origen-destino documentada por flujo (INF-010-02).

## 2. Fuera de alcance

- `terraform apply/destroy`, creación de cualquier recurso AWS, cuentas, facturación, IAM.
- Cómputo (I-020), balanceo/HTTPS (I-030), DynamoDB (I-040), Redis (I-050), IAM (I-060).
- Endpoints H06/E001–E075.

## 3. Permisos solicitados (mínimos)

| Herramienta | Uso | Condición |
|---|---|---|
| `edit`/`write` en `infra/terraform/**` | Módulos de red, variables, outputs | Sólo archivos de I-010 |
| `shell`: `terraform fmt -check`, `terraform validate` | Validación sintáctica local | Sin backend remoto; sin `init` contra AWS |
| `shell`: pruebas Go y Python del repositorio | Regresión | Sin cambios fuera de I-010 |
| Git: rama propia, commit, push y PR | Publicación | Nunca push directo a `main` |

**Denegado expresamente:** `terraform init` con backend remoto, `plan` contra credenciales reales,
`apply`, `destroy`, cualquier `aws * create/update/delete/put`.

## 4. Archivos previstos

```
infra/terraform/modules/red/main.tf
infra/terraform/modules/red/variables.tf
infra/terraform/modules/red/outputs.tf
infra/terraform/envs/h08i/main.tf
infra/terraform/envs/h08i/backend.tf.example   (sin secretos, sin estado real)
infra/terraform/envs/h08i/terraform.tfvars.example
infra/terraform/README.md
tests/infra/terraform/test_red_contrato.py     (análisis estático de .tf)
tests/infra/terraform/test_matriz_flujos.py
sdd/02-infraestructura/I-010-red-vpc/tasks.md      (registro de evidencias)
sdd/02-infraestructura/I-010-red-vpc/checklist.md
```

## 5. Pruebas previstas

1. Análisis estático del `.tf`: dos subredes en AZ distintas, IGW presente, sin `0.0.0.0/0` en rutas,
   gateway endpoints S3/DynamoDB, `map_public_ip_on_launch = false`, sin `aws_nat_gateway`.
2. Matriz origen-destino: SG del backend sólo permite el puerto interno del ALB; Redis sólo desde el
   backend; ningún SG con `0.0.0.0/0` en ingress.
3. Reconciliación con I-000: los SKU y el diseño E deben coincidir con
   `infra/finops/MATRIZ_CONECTIVIDAD_PRIVADA_I000.md` y no introducir endpoints no presupuestados.
4. `terraform fmt -check` y `terraform validate` (si se habilitan los permisos).
5. Regresión completa: FinOps, SDD, H06, permisos, `go build/vet/test`.

## 6. Criterios de aceptación

- INF-010-01..06 cubiertos por código y por prueba estática.
- Cero recursos AWS creados; `terraform state` inexistente.
- Coste de red idéntico al presupuestado en I-000 (23.7400 USD de conectividad, 45 días).
- Sin regresiones en las 75 operaciones API.
- `terraform validate` en verde (una vez habilitados los permisos).

## 7. Dependencias y secuencia

- **Depende de:** I-000 (cerrada), decisiones D1–D6 registradas, diseño E de conectividad.
- **Habilita a:** I-020 (cómputo) e I-030 (balanceo/HTTPS), que consumirán los outputs de la VPC.
- **No adelanta** ninguna otra especificación.

## 8. Riesgos

1. Si se habilita `monitoring` permanente, el presupuesto vuelve a quedar sin cobertura (documentado).
2. Terraform con backend remoto exigiría S3 + DynamoDB de estado (ya presupuestados vía gateway endpoints).
3. Cualquier `init` real contra la cuenta requiere autorización AWS específica, no cubierta por este Gate.

## 10. Corrección de trazabilidad antes de implementar

El Gate propuesto decía “SG de entrada ALB:443”. Eso **contradice D2**, aprobada por el responsable
(`ACEPTO_TLS_CLOUDFRONT_ALB_HTTP_PRIVADO`). Corrección aplicada y registrada:

| Elemento | Valor efectivo | Base |
|---|---|---|
| Cliente → CloudFront | HTTPS 443 | D2, spec §9 |
| CloudFront → ALB | **HTTP 80** (privado, VPC Origins) | D2 |
| ALB → backend Go | **HTTP 8080** (subred privada) | A-030/A-090 (puerto interno) |
| Health check | HTTP `GET /api/v1/health/live` en 8080 | A-090 |
| Certificados | sólo el de CloudFront (`*.cloudfront.net`) | D2 |
| ALB público | **no**; ALB `internal` | D2 + spec §9 |

El texto “443” de `spec.md` (estado `DRAFT / REVIEW — NO APROBADO`) queda superado por D2 sin editar la
spec: la decisión humana tiene precedencia y la discrepancia se documenta aquí.



## 11. Aprobación solicitada (una sola)

Para implementar I-010 en local hace falta una de estas respuestas:

- `APRUEBO_GATE_I010_LOCAL` — habilita escritura en `infra/terraform/**` y validaciones locales sin
  backend remoto ni mutación AWS.
- `APRUEBO_GATE_I010_LOCAL_Y_VALIDACION_TERRAFORM` — añade `terraform fmt/validate` (sin `init`
  remoto, sin `plan` contra AWS).

Cualquier autorización de `init`/`plan`/`apply` contra la cuenta real es una decisión distinta y no se
solicita aquí.
