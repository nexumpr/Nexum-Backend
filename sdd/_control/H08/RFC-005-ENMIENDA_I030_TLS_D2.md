# RFC-005 — Enmienda de TLS de I-030 por ratificación D2

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/RFC-005-ENMIENDA_I030_TLS_D2.md` |
| Fecha | 2026-11-06 |
| Spec afectada | `sdd/02-infraestructura/I-030-balanceo-https/` (v0.1.0, **DRAFT / REVIEW — NO APROBADO**) |
| Origen | Contradicción normativa entre el DRAFT de I-030 y la decisión humana D2, registrada en `sdd/_control/H08/GATE_H08I_I030.md` §1 |
| Ratificación humana | `RATIFICO_D2_PARA_I030` (2026-11-06) |
| Estado | **RATIFICADO — enmienda registrada. Implementación NO autorizada** (requiere `APRUEBO_GATE_I030_LOCAL`) |
| Autoriza | registrar la decisión y resolver la contradicción normativa de I-030 |
| No autoriza | Terraform, merge, despliegue AWS, cambios en otras especificaciones, generación de costos |

## 1. Problema

El texto de `spec.md` I-030 §2 `INF-030-01`/`INF-030-02` y §8 exige **ALB con listener HTTPS:443 y certificado ACM público para un dominio bajo control del proyecto**. La decisión humana **D2** (`ACEPTO_TLS_CLOUDFRONT_ALB_HTTP_PRIVADO`, registrada en `sdd/_control/H08/DECISION_CIERRE_I000.md`) establece **CloudFront HTTPS con certificado por defecto `*.cloudfront.net` → VPC Origin → ALB interno HTTP:80**.

Ambas no pueden cumplirse a la vez: el dominio propio implicaría gasto personal, prohibido por `GOV-14` y por la política de cero desembolso.

## 2. Restricción de herramienta y mecanismo elegido

`opencode.json` **deniega** la acción `edit` sobre `sdd/**/spec.md` y `sdd/**/plan.md`. El agente **no** puede ni debe eludir esa denegación (R-021). Por tanto la enmienda **no** se escribe dentro de `spec.md`/`plan.md`: se registra como RFC de control con precedencia declarada, y se refleja en `tasks.md`, `checklist.md`, el Gate de I-030 y el estado único.

El texto original de `spec.md`/`plan.md` permanece íntegro (historial conservado) y sigue marcado `DRAFT / REVIEW — NO APROBADO`. Cualquier lectura futura debe aplicar el orden de precedencia de §3.

## 3. Enmienda normativa (sustituye, no borra)

| Cláusula original | Estado tras esta RFC |
|---|---|
| `INF-030-01` — «listener HTTPS:443 usando ACM público» | **SUPERADA** en el tramo CloudFront→ALB. Vigente: ALB **interno** con listener HTTP:80 privado; el TLS termina en CloudFront. El ALB sigue siendo el balanceador real de los dos targets. |
| `INF-030-02` — «ACM público para dominio bajo control del proyecto, validación DNS» | **SUPERADA**. Certificado por defecto de CloudFront (`*.cloudfront.net`); sin ACM propio, sin Route 53, sin compra de dominio. |
| §8 — «ALB internet-facing, 2 IPv4 públicas facturables, listener 443 ACM» | **SUPERADA** por §9 + esta RFC: ALB `internal`, 0 IPv4 públicas, sin NAT. |
| §9 — CloudFront TLS público con ALB privado | **VIGENTE** y ratificada como diseño de I-030. |
| `INF-030-03` health, `INF-030-04` redirección, `INF-030-05` failover, `INF-030-06` evidencias | **VIGENTES** sin cambios, con la precisión de §4.3. |

## 4. Requisitos de seguridad que se conservan (no negociables)

1. **ALB `internal`**: sin esquema `internet-facing` en ningún entorno.
2. **Cero exposición pública del backend**: subredes privadas, sin IPv4 públicas, sin NAT, sin SSH.
3. **Ingreso al ALB restringido** a la prefix list administrada de CloudFront en el puerto 80; el SG del backend sólo acepta 8080 desde el SG del ALB.
4. **IGW adjunto sin rutas públicas**, exigido por CloudFront VPC Origins; no habilita tráfico de entrada arbitrario.
5. **TLS 1.2+ en el borde** con `viewer_protocol_policy = redirect-to-https`; HTTP del cliente no se sirve.
6. **Sin caché de API financiera**: `default_ttl = 0`, `max_ttl = 0`, sin `CacheEverything`; no se cachean respuestas con `Authorization` ni datos personalizados; métodos REST completos (GET/POST/PUT/PATCH/DELETE/OPTIONS/HEAD).
7. **Identidad intacta**: `Authorization` e `Idempotency-Key` reenviados; JWT no se altera ni se registra.
8. **Sin secretos** en Terraform, Git, user-data ni cabeceras propias.
9. **Dos targets reales** del ASG de I-020 en AZ distintas; failover por health check.
10. **Nombres reales del ALB (`*.elb.amazonaws.com`) no se publican** como URL de servicio.

## 5. Límite de seguridad aceptado expresamente

El tramo CloudFront→ALB discurre por VPC Origin dentro de la VPC y **no usa TLS a nivel de aplicación** (HTTP:80). Es tráfico privado de red, no cifrado de extremo a extremo en la capa HTTP. **D2 lo acepta de forma explícita.** Esta RFC prohíbe presentarlo como TLS extremo a extremo. Si un Gate posterior exigiera TLS hasta el ALB, se requiere nueva RFC/ADR, dominio financiado y recotización; nada de eso está autorizado aquí.

## 6. Efectos y límites

- **Versión de la spec:** 0.1.0 → 0.2.0 (enmienda). No se altera el cuerpo original por restricción de herramienta; la versión efectiva se declara en este RFC, en `tasks.md`, en `checklist.md` y en el Gate.
- **Costos:** sin partidas nuevas. ALB interno y CloudFront ya figuran en el diseño E de I-000 (bruto verificado 79.0430 + conectividad 23.7400 = 102.7830 USD/45 d; con margen 10 % = 113.0613 ≤ 120.0000 verificados).
- **Dependencias:** I-010 (red), I-020 (ASG, 2 instancias), A-090 (rutas `/api/v1/health/live|ready`).
- **Bloqueo vigente:** RFC-004 (secretos del bootstrap) impide el **despliegue**, no la implementación local.
- **Estado de infraestructura:** `DESIGN_RATIFIED`, `INFRA_NOT_DEPLOYED`, `AWS_NOT_RUN`.

## 7. Aprobaciones

| Control | Estado |
|---|---|
| `RATIFICO_D2_PARA_I030` | **CONCEDIDA** 2026-11-06 |
| `APRUEBO_GATE_I030_LOCAL` | **PENDIENTE** — sin ella no se escribe Terraform de I-030 |
| Autorización de merge | **PENDIENTE** — posterior al PR y a checks en verde |
| Autorización de despliegue AWS | **NO SOLICITADA** — Gate independiente, fuera de I-030 |
