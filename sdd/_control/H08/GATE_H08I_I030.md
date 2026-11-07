# Gate H08-I — I-030 `balanceo-https`

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/GATE_H08I_I030.md` |
| Fecha | 2026-11-06 |
| Spec | `sdd/02-infraestructura/I-030-balanceo-https/` (v0.2.0 por enmienda) |
| Estado | **IMPLEMENTADO_LOCAL — PR PENDIENTE; DESPLIEGUE NO AUTORIZADO** |
| Precondición | I-020 cerrada (PR #17 `10f6327`); hotfixes A-030/A-050/A-090/A-100 en `main` `3de4fc6` |
| Rama | `feat/h08i-i030-balanceo-https` (sincronizada con `main`; sin PR creado) |
| Enmienda | `sdd/_control/H08/RFC-005-ENMIENDA_I030_TLS_D2.md` |

## Historial del Gate

| Versión | Estado | Cambio |
|---|---|---|
| v0.1.0 | `PROPUESTO` | Gate propuesto; conflicto TLS D2 vs DRAFT documentado, sin resolver |
| v0.2.0 | `D2_RATIFICADA` | Ratificación humana `RATIFICO_D2_PARA_I030`; conflicto resuelto por RFC-005. Sigue **sin** autorizar implementación |
| v0.3.0 | `IMPLEMENTADO_LOCAL` | `APRUEBO_GATE_I030_LOCAL` concedida 2026-11-06; módulo `balanceo` escrito y validado localmente. Sin AWS, sin PR aún |

## 1. Conflicto normativo — RESUELTO por ratificación

| Fuente | Exigía |
|---|---|
| `spec.md` I-030 `INF-030-01`/`INF-030-02`, §8 | listener **HTTPS:443** con **ACM público** de dominio propio |
| **D2** | CloudFront **HTTPS** por defecto → VPC Origin → ALB **interno HTTP:80** |

**Resolución (2026-11-06):** el responsable ratificó `RATIFICO_D2_PARA_I030` con alcance limitado a
registrar la decisión y resolver la contradicción, sin autorizar implementación, merge, despliegue ni gastos.
La precedencia queda fijada en `RFC-005` §3: se **superan** las cláusulas de listener 443 y ACM/dominio;
se **conservan** ALB interno, subredes privadas, 0 IPv4 públicas, SG restringido por prefix list, TLS 1.2+,
sin caché de API financiera, JWT intacto y dos targets reales.

**Restricción de herramienta registrada:** `opencode.json` deniega `edit` sobre `sdd/**/spec.md` y
`sdd/**/plan.md`. El agente no eludió la denegación (R-021); la enmienda vive en el RFC-005 y se refleja en
`tasks.md`, `checklist.md` y este Gate. El cuerpo original de `spec.md`/`plan.md` permanece intacto.

**Límite de seguridad aceptado:** el tramo CloudFront→ALB es privado de red pero **sin TLS de aplicación**.
No puede presentarse como TLS extremo a extremo (RFC-005 §5).

## 2. Bloqueos y dependencias

| ID | Estado | Impacto |
|---|---|---|
| **D-01** (rutas de salud no registradas) | **CERRADO** por PR #21 (`3de4fc6`): `/api/v1/health/live` y `/ready` registrados en el router | El target group puede usar `/api/v1/health/live` con matcher 200 |
| **Readiness 503 por diseño** | VIGENTE | `/ready` devuelve 503 sin DynamoDB/Redis (I-040/I-050); **no** puede usarse como health check del TG ni como prueba de disponibilidad |
| **RFC-004** (secretos del bootstrap) | PROPUESTO | Bloquea **despliegue**, no la implementación local |
| **VPC Origins / elegibilidad de cuenta** | `NOT_VERIFIED` | Sólo verificable con AWS; se difiere al Gate de despliegue |
| **`terraform plan/apply`** | DENEGADO por política | No se solicitan; ni siquiera se intentan |

## 3. Alcance autorizable por `APRUEBO_GATE_I030_LOCAL`

| Requisito | Implementación prevista |
|---|---|
| INF-030-01 | `aws_lb` **interno**, target group 8080 (matcher 200), listener **HTTP:80**, SG propio; `deregistration_delay` 30 s |
| INF-030-02 | CloudFront con certificado por defecto; **sin ACM ni Route 53**; VPC Origin hacia el ALB |
| INF-030-03 | Health check `/api/v1/health/live` (30 s / 5 s / 2 / 3); liveness y readiness separados |
| INF-030-04 | Redirección HTTP→HTTPS en CloudFront; ALB interno sin 443 |
| INF-030-05 | `aws_autoscaling_attachment` al ASG de I-020 (2 instancias, AZ distintas) |
| INF-030-06 | Evidencias locales (`fmt/validate`, pruebas estáticas); AWS `NOT_RUN` |

## 4. Fuera de alcance (denegado explícitamente)

- `terraform plan/apply/destroy`, creación de ALB/CloudFront/distribución/certificados, DNS, despliegue.
- I-040 (DynamoDB), I-050 (Redis), I-060 (IAM), E001–E075, H08-E.
- Modificación de los 75 contratos OpenAPI o de specs ajenas.
- Cambios de plan AWS, compras de dominio, gasto personal.
- Merge del PR sin autorización humana separada.

## 5. Archivos previstos

```
infra/terraform/modules/balanceo/{main.tf,variables.tf,outputs.tf}
infra/terraform/envs/h08i/{main.tf,variables.tf,outputs.tf}
tests/infra/terraform/test_balanceo_contrato.py
sdd/02-infraestructura/I-030-balanceo-https/{tasks,checklist}.md
sdd/_control/H08/GATE_H08I_I030.md
```

## 6. Pruebas exigidas antes del PR

1. ALB `internal` (nunca `internet-facing`); ausencia de `aws_acm_certificate`, `aws_route53_*`, listener 443.
2. Target group 8080, matcher `200`, health check `/api/v1/health/live`, `deregistration_delay` ≤ 60 s.
3. Dos destinos vía ASG; sin tercer servidor; sin `0.0.0.0/0` hacia 8080.
4. CloudFront: origen VPC, `redirect-to-https`, TTL 0, métodos completos, `Authorization` reenviado, sin `CacheEverything`.
5. `terraform fmt -check`, `terraform init -backend=false`, `terraform validate`.
6. Regresión: FinOps, SDD/SHA-256, H06/OpenAPI, permisos, `go build/vet/test`.
7. CI en verde en el PR.

## 7. Coste

Sin partidas nuevas: ALB interno + LCU y egreso CloudFront ya están en el diseño E de I-000.
Presupuesto vigente: A **113.0613** · B **118.5613** USD/45 d ≤ 120.0000 verificados. `terraform plan`
permanece denegado, por lo que no hay recálculo automático de costos: cualquier recurso adicional exige
Gate aparte.

## 8. Aprobaciones

| Control | Estado | Efecto |
|---|---|---|
| `RATIFICO_D2_PARA_I030` | **CONCEDIDA** 2026-11-06 | Sólo decisión normativa + ajustes documentales |
| `APRUEBO_GATE_I030_LOCAL` | **CONCEDIDA** 2026-11-06 | Autorizó escribir/validar I-030 localmente; no despliega ni fusiona |
| Autorización de merge | **PENDIENTE** | Requiere PR, checks verdes y frase humana específica |
| Autorización de despliegue AWS | **NO SOLICITADA** | Gate independiente y separado |

## 9. Registro de implementación local (2026-11-06)

**Alcance ejecutado:** únicamente archivos locales del repositorio. `terraform plan/apply/destroy` **no** se
ejecutaron (denegados por política); **ningún** recurso AWS existe; costo incurrido 0.0000 USD.

| Artefacto | Estado |
|---|---|
| `infra/terraform/modules/balanceo/{main,variables,outputs}.tf` | Creado — ALB interno, TG 8080, listener HTTP:80, `aws_autoscaling_attachment`, `aws_cloudfront_vpc_origin`, distribución CloudFront |
| `infra/terraform/envs/h08i/main.tf` | `module "balanceo"` compuesto + salida `balanceo`; proveedor AWS `~> 5.72` |
| `infra/terraform/envs/h08i/variables.tf` | `health_check_path`, `cloudfront_price_class` |
| `tests/infra/terraform/test_balanceo_contrato.py` | 35 pruebas estáticas |
| `tests/infra/finops/test_i030_alineacion.py` | 8 pruebas de alineación con el escenario C |

**Evidencia ejecutada:** `terraform fmt -check` exit 0 · `terraform init -backend=false` OK · `terraform validate`
`Success` · `tests/infra/terraform` 103 pruebas OK (76 previas + 35 nuevas, solapadas por clase) ·
`tests/infra/finops` 175 OK · `tests/infra/systemd` 11 OK · `herramientas/tests` 18 OK · `go build/vet/test` PASS ·
`validar_sdd_integral.py --check-git` PASS · `validar_h06.py` PASS.

**Decisiones de implementación registradas:**
- El security group del ALB **no** se duplica: se reutiliza el de I-010 (ingress 80 sólo desde la prefix list
  `com.amazonaws.global.cloudfront.origin-facing`, egress 8080 al SG del backend). Se añadió precondition
  `sg_alb_id != sg_backend_id`.
- Health check del target group en `/api/v1/health/live`; una precondition y una validación de variable
  **rechazan** `/ready` para no marcar unhealthy el par mientras I-040/I-050 no existan.
- La API no se cachea: `Managed-CachingDisabled` + `Managed-AllViewer` (Authorization e idempotencia reenviados).

**Hallazgo H-01 (abierto):** con el certificado por defecto de CloudFront, AWS fija la security policy en
**TLSv1** y no permite `TLSv1.2_2021`. No se declara TLS 1.2+ en el borde. Exige aceptación humana explícita
antes del Gate de despliegue (o certificado propio + dominio, con RFC y financiación). Ver RFC-005 §8.

**Requisito no suprimido:** no existe CI de Terraform todavía; corresponde a I-080. Hasta entonces la evidencia
es local (`fmt`/`validate` + pruebas estáticas) y el PR ejecutará los workflows existentes (SDD y Go).
