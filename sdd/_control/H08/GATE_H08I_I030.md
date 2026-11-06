# Gate H08-I — I-030 `balanceo-https` (propuesta de apertura)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/GATE_H08I_I030.md` |
| Fecha | 2026-11-06 |
| Spec | `sdd/02-infraestructura/I-030-balanceo-https/{spec,plan,tasks,checklist}.md` |
| Estado | **PROPUESTO — requiere aprobación humana; contiene un conflicto normativo a resolver** |
| Precondición | I-020 cerrada: PR #17 MERGED (`10f6327`) |
| Rama preparada | `feat/h08i-i030-balanceo-https` (desde `main` actualizado) |

## 1. Conflicto normativo detectado (bloqueante de diseño)

| Fuente | Exige |
|---|---|
| `spec.md` I-030 (estado **DRAFT / REVIEW — NO APROBADO**), INF-030-01/02 | ALB con **listener HTTPS:443** y **certificado ACM público para un dominio bajo control del proyecto**, validación DNS, sin autofirmado |
| **D2**, decisión humana firmada (2026-11-06) | Cliente → CloudFront **HTTPS**; CloudFront → ALB **HTTP 80 privado**; certificado **`*.cloudfront.net`**; sin dominio propio |
| Constitución R-019/GOV-14 y D1/D2 | **Cero dinero personal**; comprar/renovar dominio está prohibido |

**Consecuencia:** el texto DRAFT de I-030 y D2 son incompatibles en TLS y en certificado. La jerarquía
vigente da precedencia a la **decisión humana expresa** (D2) sobre una spec en DRAFT, pero **no puedo
resolverlo unilateralmente**: se requiere ratificación explícita para I-030.

### Opciones para la ratificación

| Opción | Efecto | Coste | Estado |
|---|---|---|---|
| **A (recomendada)** | Ratificar D2 para I-030: CloudFront `*.cloudfront.net` + HTTP privado a ALB interno; ALB sin 443 ni ACM | 0 USD | Cumple cero gasto; **excepción de seguridad ya aceptada en D2** |
| B | Cumplir el DRAFT: ALB HTTPS con ACM + dominio propio | Dominio + Route 53 (>0 USD) | **Incompatible con cero dinero personal**; requeriría RFC y financiación |
| C | CloudFront + ALB HTTPS con certificado de un dominio ya financiado externamente | 0 USD nuevo para el proyecto | Requiere que el responsable aporte el dominio y su financiación |

Sin ratificación, I-030 **no puede implementarse** sin violar D2 o el DRAFT.

## 2. Bloqueos adicionales heredados

| ID | Bloqueo | Impacto en I-030 |
|---|---|---|
| **D-01** | `/api/v1/health/live` y `/ready` **no están registrados** en el router Go (solo existe el handler) | El target group no puede tener health checks válidos; I-030 queda bloqueada para pruebas reales hasta que A-090/H08-A los exponga |
| **RFC-004** | Secretos en reemplazo automático de instancias | Bloquea **despliegue**, no la implementación local de I-030 |

## 3. Alcance autorizable (una vez ratificado)

| Requisito | Implementación prevista |
|---|---|
| INF-030-01 | `aws_lb` **interno**, `aws_lb_target_group` (puerto 8080, matcher 200, health check a la ruta que resuelva D-01), `aws_lb_listener` **HTTP 80** (opción A) |
| INF-030-02 | CloudFront con certificado por defecto `*.cloudfront.net`; **sin ACM ni Route 53** (opción A). Si se ratifica B/C, se recotiza antes |
| INF-030-03 | Health interval/timeout/threshold parametrizados; `deregistration_delay` acotado; liveness vs readiness separados |
| INF-030-04 | Sin redirección 80→443 pública: el ALB es interno; la redirección se hace en CloudFront (HTTP→HTTPS) |
| INF-030-05 | Failover: dos targets, `target_group_attachment` por instancia del ASG (o `aws_autoscaling_attachment`), ALB retira destinos no sanos |
| INF-030-06 | Evidencias: salidas Terraform, pruebas estáticas y `NOT_RUN_AWS` explícito |

**CloudFront VPC Origin:** requiere `internet_gateway` (ya en I-010), subred privada con IPv4 libre,
SG del ALB permitiendo la prefix list de CloudFront (ya implementado) y el ALB creado (I-030).
La elegibilidad real de la cuenta sigue `NOT_VERIFIED` hasta el despliegue.

## 4. Fuera de alcance

- `terraform plan/apply/destroy`, crear ALB/CloudFront/certificados, DNS, despliegue y pruebas HTTP reales.
- I-040 (DynamoDB), I-050 (Redis), I-060 (IAM), E001–E075.
- Cambios en los 75 contratos OpenAPI.

## 5. Archivos previstos

```
infra/terraform/modules/balanceo/{main.tf,variables.tf,outputs.tf}
infra/terraform/envs/h08i/main.tf                  (module "balanceo")
infra/terraform/envs/h08i/variables.tf
tests/infra/terraform/test_balanceo_contrato.py
sdd/02-infraestructura/I-030-balanceo-https/{tasks,checklist}.md
```

## 6. Pruebas previstas

1. ALB `internal` (nunca `internet-facing`), listener HTTP 80 sólo si se ratifica A; ausencia de ACM.
2. Target group puerto 8080, matcher `200`, health check a la ruta acordada; `deregistration_delay` ≤ 60 s.
3. Dos targets (uno por instancia del ASG) o attachment del ASG; sin tercer servidor.
4. CloudFront con origen VPC (sin `custom_origin_config` público) y `viewer_protocol_policy = redirect-to-https`.
5. Sin `aws_acm_certificate` ni `aws_route53_*` en el módulo (opción A).
6. `terraform fmt -check` y `terraform validate`.
7. Regresión: FinOps, SDD, H06, permisos, Go.

## 7. Coste

ALB fijo + LCU (24.3000 + 8.6400 USD/45 d) y CloudFront (0.8500 de egreso + reserva) ya están en el
diseño E. No hay partidas nuevas si se ratifica A. La opción B añadiría dominio y Route 53 (no
presupuestados). Presupuesto vigente: A 113.0613 · B **118.5613** ≤ 120.

## 8. Aprobaciones solicitadas (dos, en este orden)

1. **Ratificar TLS de I-030**:
   - `RATIFICO_D2_PARA_I030` → opción A (recomendada).
   - `RATIFICO_I030_TLS_CON_DOMINIO` → opción B/C, indicando dominio y financiación (implica RFC).
2. **Gate de implementación local**:
   - `APRUEBO_GATE_I030_LOCAL` o `APRUEBO_GATE_I030_LOCAL_Y_VALIDACION_TERRAFORM`.

**D-01 debe corregirse en A-090/H08-A antes de cualquier prueba real de balanceo**; no se corrige aquí
porque no pertenece al alcance de I-030.
