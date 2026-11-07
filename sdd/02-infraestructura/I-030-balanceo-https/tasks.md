# I-030-balanceo-https — Balanceador real, TLS, DNS, health y failover

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.2.0 |
| Estado | **D2 RATIFICADO (2026-11-06) — IMPLEMENTACIÓN NO AUTORIZADA** |
| Trazabilidad | A-030, I-000, I-010, I-020 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |
| Enmienda | v0.2.0 — `RATIFICO_D2_PARA_I030` registrada en `sdd/_control/H08/RFC-005-ENMIENDA_I030_TLS_D2.md`. `spec.md`/`plan.md` no se editan (política `edit` denegada); la precedencia la fija el RFC-005. |

## Gestión de tareas H05 (solo especificación y aceptación de diseño)

Estado inicial de todos los trabajos: `TODO`, salvo evidencia local de integridad H04 documentada por el responsable. No marcar como completado por generar archivos.

### T-030-01 — Cerrar alternativa candidata

- **Estado:** **RESUELTO_POR_ENMIENDA_D2 (2026-11-06)** — no `TODO`. Decisión cerrada: ALB **interno** +
  target group EC2 de 2 targets + listener **HTTP:80** privado + CloudFront HTTPS con VPC Origin.
  Registro: `sdd/_control/H08/RFC-005-ENMIENDA_I030_TLS_D2.md`.
- **Responsable:** responsable del proyecto (ratificación `RATIFICO_D2_PARA_I030`).
- **Entrada:** H04 aprobado y A-030, I-000, I-010, I-020.
- **Trabajo:** Application Load Balancer + target group EC2 con 2 targets y listener HTTPS:443 usando ACM público, reglas mínimas; opción concreta requiere precio/servicio permitido. No agregar API Gateway redundante sin ADR.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-030-02 — Cerrar certificado

- **Estado:** **RESUELTO_POR_ENMIENDA_D2 (2026-11-06)** — certificado por defecto de CloudFront
  (`*.cloudfront.net`); **sin** ACM propio, Route 53 ni dominio de pago. La exigencia de ACM de
  `INF-030-02` queda superada por D2 (RFC-005 §3). Riesgo R-02 cerrado; el límite de §5 del RFC-005
  (tramo privado sin TLS de aplicación) sigue aceptado y no puede presentarse como TLS extremo a extremo.
- **Responsable:** responsable del proyecto (ratificación `RATIFICO_D2_PARA_I030`).
- **Entrada:** H04 aprobado y A-030, I-000, I-010, I-020.
- **Trabajo:** ACM público para nombre de dominio bajo control del proyecto, validación DNS, SNI/TLS 1.2+ y renovación; sin certificado autofirmado en URL pública. DNS o proveedor externo se costea y documenta.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-030-03 — Cerrar health

- **Estado:** **DISEÑO_CERRADO_POR_D2 — ejecución pendiente de `APRUEBO_GATE_I030_LOCAL`**. Parámetros fijados:
  interval 30 s, timeout 5 s, healthy 2, unhealthy 3, matcher `200`, path `/api/v1/health/live`.
  `/api/v1/health/ready` **no** puede usarse como health check del target group mientras devuelva 503 por
  diseño (sin dependencias registradas hasta I-040/I-050); usarlo marcaría unhealthy todo el par.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-030, I-000, I-010, I-020.
- **Trabajo:** Health interval/timeout/threshold y matcher 200 parametrizados, liveness separado de readiness. Estado healthy por instancia consultable; detectar 5xx y target unhealth.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-030-04 — Cerrar redirección

- **Estado:** **DISEÑO_CERRADO_POR_D2 — ejecución pendiente de `APRUEBO_GATE_I030_LOCAL`**. La redirección
  HTTP→HTTPS se aplica en CloudFront (`redirect-to-https`); el ALB interno no atiende tráfico público y no
  expone listener 443. HSTS sólo puede anunciarse en el dominio de CloudFront, no en un dominio propio.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-030, I-000, I-010, I-020.
- **Trabajo:** HTTP:80 solo si hay decisión de redirección permanente a HTTPS y costo/SG permitido; de otra forma bloquear puerto 80. HSTS donde dominio/operación lo permitan, forwarded headers solo de proxies confiables.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-030-05 — Cerrar failover

- **Estado:** **DISEÑO_CERRADO_POR_D2 — ejecución pendiente de `APRUEBO_GATE_I030_LOCAL`**. Prueba prevista
  con dos destinos del ASG en AZ distintas; el failover real requiere AWS y queda `NOT_RUN` hasta Gate de
  despliegue. No se declarará cumplido con pruebas locales.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-030, I-000, I-010, I-020.
- **Trabajo:** En prueba autorizada H08, impedir servicio de un nodo, health lo marca unhealthy y tráfico continúa al otro. Recuperar y comprobar retorno de ambos healthy; no demostrar solo round-robin.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-030-06 — Cerrar evidencias

- **Estado:** **EVIDENCIAS_NOT_RUN** (no existe AWS desplegada; no se simulan). Formato previsto en el RFC-005
  y en el Gate: `curl -I https://<dist>.cloudfront.net/api/v1/health/live`, `describe-target-health`, IDs de
  instancia reales y costo observado. Sólo se marcará `PASS` con salida real y fecha.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-030, I-000, I-010, I-020.
- **Trabajo:** curl HTTPS con validación de certificado, cabeceras, status/latencia y requestId; listado de target health con IDs distintos; nombres de dominio/ALB reales y factura/costos de ALB.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-030-90 — Revisión de riesgos y dependencias

- **Estado:** PARCIAL — 1 riesgo cerrado, 2 abiertos con dueño y Gate asociado.
- **Trabajo:** resolver o escalar: ALB no habilitado en plan Free concreto, Certificado/dominio no disponibles, ALB mantiene cobro cuando los nodos están apagados.
- **Clasificación vigente (2026-11-06):**
  - `I-030-R02` (certificado/dominio no disponibles): **RESOLVED** por D2 — sin dominio ni ACM propio
    (evidencia: `RATIFICO_D2_PARA_I030`, RFC-005).
  - `I-030-R01` (ALB no habilitado en plan Free): **OPEN** — la elegibilidad real del servicio y el plan
    sólo se verifican con AWS; no se asume. Gate asociado: despliegue.
  - `I-030-R03` (ALB cobra con nodos apagados): **OPEN** — documentado en I-000 (residual 6.8440 USD/45 d con
    runtime apagado); decisión de aceptación corresponde al Gate de despliegue, no a I-030 local.
- **Criterio DoD:** cada riesgo clasificado RESOLVED/BLOCKED/ACCEPTED con firma/fecha; no usar ACCEPTED para incumplir requisito docente sin aclaración.

### T-030-99 — Ratificación de la especificación

- **Estado:** **D2 RATIFICADO 2026-11-06; Gate de implementación pendiente.** Falta `APRUEBO_GATE_I030_LOCAL`.
- **Entrada:** `spec.md`, `plan.md`, `tasks.md`, `checklist.md` actualizados.
- **Salida:** decisión explícita del responsable y vínculo a evidencia en informe I-GO.
- **Bloqueo:** no crear Terraform/endpoints ni ejecutar AWS.

## Tareas de implementación H08-I (bloqueadas hasta `APRUEBO_GATE_I030_LOCAL`)

Todas en estado `BLOQUEADO_POR_GATE`. No se ha escrito ni ejecutado nada de esta sección.

### T-030-10 — Módulo `infra/terraform/modules/balanceo`
- ALB `internal`, subredes privadas de I-010, SG propio, target group 8080 con health check
  `/api/v1/health/live`, listener HTTP:80, `deregistration_delay` 30 s.

### T-030-11 — Asociación ASG ↔ target group
- `aws_autoscaling_attachment` sobre el ASG de I-020 (min=desired=max=2, AZ distintas); sin tercer servidor.

### T-030-12 — CloudFront con VPC Origin
- Distribución HTTPS, `*.cloudfront.net`, origen VPC (ALB), `redirect-to-https`, TTL 0, métodos completos,
  `Authorization`/`Idempotency-Key` reenviados, sin `CacheEverything`, sin WAF de pago.

### T-030-13 — Variables, outputs y composición en `envs/h08i`
- Entradas documentadas, salidas `distribution_domain`, `alb_arn`, `target_group_arn`; `Spec` de etiquetas
  actualizado sin romper I-010/I-020.

### T-030-14 — Pruebas estáticas
- `tests/infra/terraform/test_balanceo_contrato.py`: ALB interno, ausencia de ACM/Route 53/443 público,
  TTL 0, métodos, SG restringido, 2 targets, health check correcto, ausencia de `0.0.0.0/0` en 8080.

### T-030-15 — Validación y FinOps
- `terraform fmt -check`, `terraform init -backend=false`, `terraform validate`; recálculo del presupuesto
  sin partidas nuevas (≤ 118.5613 USD/45 d).

### T-030-16 — Integridad y CI
- `validar_sdd_integral.py --check-git`, `validar_h06.py`, pruebas de herramientas, `go build/vet/test`,
  SHA-256 sincronizado, PR único con checks en verde.

## Registro de ratificación D2 (2026-11-06)

| Campo | Valor |
|---|---|
| Frase humana | `RATIFICO_D2_PARA_I030` |
| Instrumento | `sdd/_control/H08/RFC-005-ENMIENDA_I030_TLS_D2.md` |
| Efecto | TLS: CloudFront HTTPS (`*.cloudfront.net`) → VPC Origin → ALB interno HTTP:80 → 2 instancias Go |
| Superado | `INF-030-01`/`INF-030-02` en lo relativo a listener 443 y ACM/dominio; §8 del `spec.md` |
| Conservado | ALB interno, subredes privadas, 0 IPv4 públicas, SG por prefix list, TLS 1.2+, TTL 0, JWT intacto, 2 targets |
| No autorizado | Terraform, merge, despliegue AWS, otras specs, gastos |
| Estado de la spec | 0.2.0 — D2 ratificado; implementación pendiente de `APRUEBO_GATE_I030_LOCAL` |
