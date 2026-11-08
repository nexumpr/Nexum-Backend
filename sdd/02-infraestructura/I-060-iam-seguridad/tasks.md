# I-060-iam-seguridad — IAM, límites SCP, secretos y accesos

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-050, I-000, I-010, I-020, I-040, I-050 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## Gestión de tareas H05 (solo especificación y aceptación de diseño)

Estado inicial de todos los trabajos: `TODO`, salvo evidencia local de integridad H04 documentada por el responsable. No marcar como completado por generar archivos.

### T-060-01 — Cerrar identidades

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-050, I-000, I-010, I-020, I-040, I-050.
- **Trabajo:** Role EC2 de runtime con permisos Dynamo específicos (PK/bounds, actions), métricas/logs y secreto Redis con acceso puntual; rol de CI solo pruebas/artefactos; rol deployment sólo asumible tras approval y trust GitHub OIDC repo/ref/environment exactos.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-060-02 — Cerrar scp

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-050, I-000, I-010, I-020, I-040, I-050.
- **Trabajo:** Comprobación de identidad (`sts get-caller-identity`) y consultas IAM/SCP por consola/read-only con permisos de administrador autorizado. Un SCP Deny no se resuelve agregando IAM Allow: escalar al titular. No usar root ni crear llaves permanentes en GitHub.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-060-03 — Cerrar secretos

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-050, I-000, I-010, I-020, I-040, I-050.
- **Trabajo:** Plan preferente con servicio aprobado y costo incluido; KMS/Secrets Manager/SSM Parameter Store evaluados por precio/plan. JWT signing key rotada por `kid`, refresh hashes en Dynamo; Redis password fuera de IaC state/logs.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-060-04 — Cerrar least privilege

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-050, I-000, I-010, I-020, I-040, I-050.
- **Trabajo:** Rutas de recurso IAM con ARN exacto por tabla/índice, Conditions donde apliquen; policy simulator o IAM Access Analyzer si disponible. Políticas de red y cifrado en tránsito/red.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-060-05 — Cerrar github oidc

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-050, I-000, I-010, I-020, I-040, I-050.
- **Trabajo:** Condicionar aud, sub para organización `nexumpr` y branch/environment autorizado; separación de rol `plan` y `apply`. `permissions: id-token: write` únicamente job autorizado. No añadir secrets AWS_ACCESS_KEY_ID.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-060-06 — Cerrar auditoría

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-050, I-000, I-010, I-020, I-040, I-050.
- **Trabajo:** Registrar cambios IAM relevantes y alertas, no exponer tokens, AWS account ID ni rutas internas en output público; evidencia con ARN enmascarado.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-060-90 — Revisión de riesgos y dependencias

- **Estado:** TODO.
- **Trabajo:** resolver o escalar: SCP bloquea IAM/EC2/ALB, GitHub OIDC no permitido, Servicios secretos con tarifa, Estado Terraform contiene secretos.
- **Criterio DoD:** cada riesgo clasificado RESOLVED/BLOCKED/ACCEPTED con firma/fecha; no usar ACCEPTED para incumplir requisito docente sin aclaración.

### T-060-99 — Ratificación de la especificación

- **Estado:** REVIEW pendiente.
- **Entrada:** `spec.md`, `plan.md`, `tasks.md`, `checklist.md` actualizados.
- **Salida:** decisión explícita del responsable y vínculo a evidencia en informe I-GO.
- **Bloqueo:** no crear Terraform/endpoints ni ejecutar AWS.

## Implementacion H08-I (2026-11-07)

Gate APRUEBO_GATE_I060_LOCAL registrado en sdd/_control/H08/GATE_H08I_I060.md. Rama feat/h08i-i060-iam-seguridad. AWS_NOT_RUN: no se creo ningun recurso IAM ni se consulto AWS. Estado H05 conservado arriba sin reescritura.

- T-060-01 Identidades: IMPLEMENTADO_LOCAL. Rol EC2 runtime + instance profile en infra/terraform/modules/iam, consumido por I-020; roles CI/plan/apply como trust-only.
- T-060-02 SCP: NO_VERIFICADO (AWS_SCP_NOT_VERIFIED, sin consultas a Organizations ni policy simulator).
- T-060-03 Secretos: BLOQUEADO por RFC-004; sin permisos Secret Manager/SSM/KMS (ver ROTACION_SECRETOS_I060.md y Gate 4.2).
- T-060-04 Least privilege: IMPLEMENTADO_LOCAL. Politica DynamoDB por tabla/indices con LeadingKeys; sin Scan/Batch/administracion (MATRIZ_IAM_SCP.md).
- T-060-05 GitHub OIDC: DECLARADO_LOCAL / NO ACTIVO. Subjects inmutables exactos (owner 349666511, repo 1447636433), aud=sts.amazonaws.com; provider ARN vacio => roles no instanciados (TRUST_OIDC_I060.md).
- T-060-06 Auditoria: IMPLEMENTADO_LOCAL. Matrices de permisos, trust y secretos/rotacion; sin ARN completos ni credenciales.
- T-060-90 Riesgos: CLASIFICADOS. SCP y OIDC reales NOT_VERIFIED; state/secretos BLOCKED; sin recursos AWS.
- T-060-99 Ratificacion: IMPLEMENTADO_LOCAL. Gate registrado; merge y despliegue requieren autorizacion especifica separada.

Pruebas: 18 de contrato en tests/infra/terraform/test_iam_contrato.py (183 en la suite Terraform). fmt -check, init -backend=false y validate en verde. CI Terraform sigue NOT_IMPLEMENTED (I-080).
