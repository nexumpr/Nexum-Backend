# I-030-balanceo-https — Balanceador real, TLS, DNS, health y failover

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.2.0 |
| Estado | **D2 RATIFICADO (2026-11-06) — IMPLEMENTACIÓN NO AUTORIZADA** |
| Trazabilidad | A-030, I-000, I-010, I-020 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |
| Enmienda | v0.2.0 — `RATIFICO_D2_PARA_I030`; precedencia fijada en `sdd/_control/H08/RFC-005-ENMIENDA_I030_TLS_D2.md` |

## Checklist de revisión (cada punto requiere evidencia, no basta la presencia de este archivo)

### Integridad y autoridad

- [ ] H04 tiene aprobación humana y el contenido coincide con las reglas superiores.
- [ ] Se analizó el PDF docente v3 y el catálogo V6.0.2 sin asumir GCP/MongoDB.
- [ ] Se mantiene el alcance de Backend, no se exige validación Mobile.
- [ ] No se crearon recursos ni artefactos de código/EP antes de H08/H06.

### Decisiones técnicas

- [x] `INF-030-01` **Alternativa candidata:** diseño justificado, referencias, supuestos y evidencia trazable.
  — **D2 ratificada 2026-11-06** (`RATIFICO_D2_PARA_I030`, RFC-005). Decisión humana, **no** prueba de nube.
- [x] `INF-030-02` **Certificado:** diseño justificado, referencias, supuestos y evidencia trazable.
  — **D2 ratificada 2026-11-06**: certificado por defecto de CloudFront; sin ACM ni dominio. No es evidencia AWS.
- [ ] `INF-030-03` **Health:** diseño justificado, referencias, supuestos y evidencia trazable.
  — implementado en el módulo (30 s/5 s/2/3, `/api/v1/health/live`, matcher 200); **comportamiento en AWS NOT_RUN**.
- [ ] `INF-030-04` **Redirección:** diseño justificado, referencias, supuestos y evidencia trazable.
  — implementado en CloudFront (`redirect-to-https`); **verificación HTTP real NOT_RUN**.
- [x] `INF-030-05` **Failover:** diseño justificado, referencias, supuestos y evidencia trazable.
  — diseño de dos targets en AZ distintas; **prueba real NOT_RUN**.
- [ ] `INF-030-06` **Evidencias:** diseño justificado, referencias, supuestos y evidencia trazable.
  — salidas reales `NOT_RUN`; sólo hay evidencia estática local.

### Pruebas y cobertura de diseño

- [ ] URL pública con certificado de CA confiable, TLS 1.2+.
  — **NO CUMPLIDO como TLS 1.2**: con certificado por defecto AWS fija TLSv1 (hallazgo H-01, RFC-005 §8).
- [ ] Distribución a dos targets simultáneos demostrada después del deploy.
- [ ] Failover de un nodo sin pérdida de persistencia.
- [ ] Cálculo ALB hourly+LCU y uso IPv4 / DNS verificado.
- [ ] No publicar credenciales ni origen interno en headers.
  — diseño cumple (sin credenciales; ALB no expuesto); verificación HTTP `NOT_RUN`.

### Riesgos, economía y aprobaciones

- [ ] Resuelto o escalado formalmente: ALB no habilitado en plan Free concreto.
  — **OPEN**: sólo verificable con AWS; Gate de despliegue.
- [x] Resuelto o escalado formalmente: Certificado/dominio no disponibles.
  — **RESOLVED** por D2 (sin dominio ni ACM); ver RFC-005 §3.
- [ ] Resuelto o escalado formalmente: ALB mantiene cobro cuando los nodos están apagados.
  — **OPEN**: residual 6.8440 USD/45 d documentado en I-000; aceptación en Gate de despliegue.
- [x] Costos potenciales (incluido apagado) identificados por servicio en I-000; no se afirma $0 por usar créditos.
  — D6 `APRUEBO_CIERRE_I000_CON_CONDICIONES`; presupuesto A 113.0613 / B 118.5613 ≤ 120.0000.
- [ ] SCP y disponibilidad de servicios relevantes documentados (no simulados).
- [ ] Gate I-GO tiene resolución expresa del responsable, o esta spec continúa DRAFT/BLOCKED.

### Gate de implementación local I-030

- [x] `APRUEBO_GATE_I030_LOCAL` concedido de forma explícita (2026-11-06).
- [x] Módulo `infra/terraform/modules/balanceo` escrito sin ACM, sin Route 53, sin listener 443 ni ALB público.
- [x] Target group 8080 con health check `/api/v1/health/live` y `deregistration_delay` ≤ 60 s.
- [x] CloudFront con VPC Origin, `redirect-to-https`, sin caché y métodos REST completos.
- [x] `origin.domain_name` = DNS del ALB (`aws_lb.principal.dns_name`), **no** el ARN del VPC origin (corrección 2026-11-07).
- [x] Excepción de gitleaks dentro de la regla `generic-api-key` con `condition = "AND"` y sin allowlist global; credencial sintética dentro del manifiesto sigue detectándose.
- [x] `fmt -check`, `init -backend=false`, `validate` ejecutados con salida real (Success).
- [x] Pruebas estáticas de contrato y regresión FinOps/SDD/H06/Go en verde en local.
- [ ] PR único revisado; merge y despliegue requieren autorización humana independiente.
- [ ] Aceptación humana del hallazgo H-01 (TLSv1 con certificado por defecto) antes del Gate de despliegue.

### Ejecución futura H08 (NO ejecutada en este lote)

- [ ] Pruebas AWS/CI/HTTP y sus evidencias ejecutadas realmente tras aprobación y release; no cambiar a PASS antes.
