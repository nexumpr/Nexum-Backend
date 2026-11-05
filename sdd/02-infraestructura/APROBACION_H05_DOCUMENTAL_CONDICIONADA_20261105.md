# H05 — Aprobación documental condicionada (registro H06)

**Fecha:** 2026-11-05. **Origen:** aceptación humana expresa en chat tras la entrega V4 de H05 y resultados `PREFLIGHT_PASS`, `INSTALL_PASS`, `VERIFY_PASS` del instalador.

**Decisión:** H05 **APPROVED_DOCUMENTAL_CONDICIONADO** para habilitar **exclusivamente el diseño H06**. No implica recursos AWS creados, permiso para `Create`, coste verificado, disponibilidad 45 días, infraestructura 24/7 real ni aprobación de `terraform apply`.

**Pendientes del despliegue:** SKU/tarifa inequívoca Redis; IAM/eligibilidad de `Create` en Free; créditos vigentes y coste integral de 45 días; seguridad de TLS tramo privado; IaC completa con pipelines seguros, GitHub OIDC/roles, operaciones plan/apply/status/suspend-aux/resume-aux/reconcile/rollback, y evidencias en dos instancias Go reales detrás de CloudFront+ALB. **El usuario exige cero pesos personales y no AWS Paid.**

**Evidencia:** H05 V4 usuario: 2 UPDATE +2 CREATE, verificación 4/4 SHA256, H03/H04 intactos. Consultas AWS de solo lectura: plan Free, cuota EC2 16 vCPU, VPC Origins list visible; Redis prices ambiguos 0.013/0.016/0.026 sin desambiguación SKU. No se realizó mutación AWS.

**Se autoriza:** especificar H06 y llevarlo a Gate propio. **No se autoriza:** recursos AWS, cambios de facturación, despliegues, gastos, reescritura de specs aprobadas o GitHub push.
