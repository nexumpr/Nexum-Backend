# I-060-iam-seguridad â€” IAM, lÃ­mites SCP, secretos y accesos

| Campo | Valor |
|---|---|
| Etapa | H05 â€” Infraestructura AWS |
| VersiÃ³n | 0.1.0 |
| Estado | **DRAFT / REVIEW â€” NO APROBADO** |
| Trazabilidad | A-050, I-000, I-010, I-020, I-040, I-050 |
| ImplementaciÃ³n | H08+, tras H07 aprobado y autorizaciÃ³n humana por release |

## GestiÃ³n de tareas H05 (solo especificaciÃ³n y aceptaciÃ³n de diseÃ±o)

Estado inicial de todos los trabajos: `TODO`, salvo evidencia local de integridad H04 documentada por el responsable. No marcar como completado por generar archivos.

### T-060-01 â€” Cerrar identidades

- **Estado:** TODO / validaciÃ³n pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobaciÃ³n humana.
- **Entrada:** H04 aprobado y A-050, I-000, I-010, I-020, I-040, I-050.
- **Trabajo:** Role EC2 de runtime con permisos Dynamo especÃ­ficos (PK/bounds, actions), mÃ©tricas/logs y secreto Redis con acceso puntual; rol de CI solo pruebas/artefactos; rol deployment sÃ³lo asumible tras approval y trust GitHub OIDC repo/ref/environment exactos.
- **Salida verificable:** decisiÃ³n documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisiÃ³n supuesta ni requisito sustituido.

### T-060-02 â€” Cerrar scp

- **Estado:** TODO / validaciÃ³n pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobaciÃ³n humana.
- **Entrada:** H04 aprobado y A-050, I-000, I-010, I-020, I-040, I-050.
- **Trabajo:** ComprobaciÃ³n de identidad (`sts get-caller-identity`) y consultas IAM/SCP por consola/read-only con permisos de administrador autorizado. Un SCP Deny no se resuelve agregando IAM Allow: escalar al titular. No usar root ni crear llaves permanentes en GitHub.
- **Salida verificable:** decisiÃ³n documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisiÃ³n supuesta ni requisito sustituido.

### T-060-03 â€” Cerrar secretos

- **Estado:** TODO / validaciÃ³n pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobaciÃ³n humana.
- **Entrada:** H04 aprobado y A-050, I-000, I-010, I-020, I-040, I-050.
- **Trabajo:** Plan preferente con servicio aprobado y costo incluido; KMS/Secrets Manager/SSM Parameter Store evaluados por precio/plan. JWT signing key rotada por `kid`, refresh hashes en Dynamo; Redis password fuera de IaC state/logs.
- **Salida verificable:** decisiÃ³n documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisiÃ³n supuesta ni requisito sustituido.

### T-060-04 â€” Cerrar least privilege

- **Estado:** TODO / validaciÃ³n pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobaciÃ³n humana.
- **Entrada:** H04 aprobado y A-050, I-000, I-010, I-020, I-040, I-050.
- **Trabajo:** Rutas de recurso IAM con ARN exacto por tabla/Ã­ndice, Conditions donde apliquen; policy simulator o IAM Access Analyzer si disponible. PolÃ­ticas de red y cifrado en trÃ¡nsito/red.
- **Salida verificable:** decisiÃ³n documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisiÃ³n supuesta ni requisito sustituido.

### T-060-05 â€” Cerrar github oidc

- **Estado:** TODO / validaciÃ³n pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobaciÃ³n humana.
- **Entrada:** H04 aprobado y A-050, I-000, I-010, I-020, I-040, I-050.
- **Trabajo:** Condicionar aud, sub para organizaciÃ³n `nexumpr` y branch/environment autorizado; separaciÃ³n de rol `plan` y `apply`. `permissions: id-token: write` Ãºnicamente job autorizado. No aÃ±adir secrets AWS_ACCESS_KEY_ID.
- **Salida verificable:** decisiÃ³n documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisiÃ³n supuesta ni requisito sustituido.

### T-060-06 â€” Cerrar auditorÃ­a

- **Estado:** TODO / validaciÃ³n pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobaciÃ³n humana.
- **Entrada:** H04 aprobado y A-050, I-000, I-010, I-020, I-040, I-050.
- **Trabajo:** Registrar cambios IAM relevantes y alertas, no exponer tokens, AWS account ID ni rutas internas en output pÃºblico; evidencia con ARN enmascarado.
- **Salida verificable:** decisiÃ³n documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisiÃ³n supuesta ni requisito sustituido.

### T-060-90 â€” RevisiÃ³n de riesgos y dependencias

- **Estado:** TODO.
- **Trabajo:** resolver o escalar: SCP bloquea IAM/EC2/ALB, GitHub OIDC no permitido, Servicios secretos con tarifa, Estado Terraform contiene secretos.
- **Criterio DoD:** cada riesgo clasificado RESOLVED/BLOCKED/ACCEPTED con firma/fecha; no usar ACCEPTED para incumplir requisito docente sin aclaraciÃ³n.

### T-060-99 â€” RatificaciÃ³n de la especificaciÃ³n

- **Estado:** REVIEW pendiente.
- **Entrada:** `spec.md`, `plan.md`, `tasks.md`, `checklist.md` actualizados.
- **Salida:** decisiÃ³n explÃ­cita del responsable y vÃ­nculo a evidencia en informe I-GO.
- **Bloqueo:** no crear Terraform/endpoints ni ejecutar AWS.

## Implementacion H08-I (2026-11-07)

Gate APRUEBO_GATE_I060_LOCAL registrado en sdd/_control/H08/GATE_H08I_I060.md. Rama feat/h08i-i060-iam-seguridad. AWS_NOT_RUN: no se creo ningun recurso IAM ni se consulto AWS. Estado H05 conservado arriba sin reescritura.

- T-060-01 Identidades: IMPLEMENTADO_LOCAL. Rol EC2 runtime + instance profile en infra/terraform/modules/iam, consumido por I-020; roles CI/plan/apply como trust-only.
- T-060-02 SCP: NO_VERIFICADO (AWS_SCP_NOT_VERIFIED, sin consultas a Organizations ni policy simulator).
- T-060-03 Secretos: BLOQUEADO por RFC-004; sin permisos Secret Manager/SSM/KMS (ver ROTACION_SECRETOS_I060.md y Gate 4.2).
- T-060-04 Least privilege: IMPLEMENTADO_LOCAL. Politica DynamoDB con Query sobre tabla e indices, LeadingKeys y Null:false; sin Scan/Batch/administracion (MATRIZ_IAM_SCP.md).
- T-060-05 GitHub OIDC: DECLARADO_LOCAL / NO ACTIVO. Subjects inmutables exactos (owner 349666511, repo 1447636433), aud=sts.amazonaws.com; provider ARN vacio => roles no instanciados (TRUST_OIDC_I060.md).
- T-060-06 Auditoria: IMPLEMENTADO_LOCAL. Matrices de permisos, trust y secretos/rotacion; sin ARN completos ni credenciales.
- T-060-90 Riesgos: CLASIFICADOS. SCP y OIDC reales NOT_VERIFIED; state/secretos BLOCKED; sin recursos AWS.
- T-060-99 Ratificacion: IMPLEMENTADO_LOCAL. Gate registrado; merge y despliegue requieren autorizacion especifica separada.

Pruebas: 31 de contrato en tests/infra/terraform/test_iam_contrato.py (196 en la suite Terraform); incluye Query sobre tabla, Null:false y casos negativos de LeadingKeys. fmt -check, init -backend=false y validate en verde. CI Terraform sigue NOT_IMPLEMENTED (I-080).
