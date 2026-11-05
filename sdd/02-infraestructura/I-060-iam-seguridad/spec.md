# I-060-iam-seguridad — IAM, límites SCP, secretos y accesos

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-050, I-000, I-010, I-020, I-040, I-050 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## 1. Objetivo y definición de terminado

Definir identidades separadas para ejecución Go, Terraform y CI/CD con mínimo privilegio, MFA humano, rotación y límites SCP, sin pedir ni revelar credenciales en archivos.

## Autoridad y restricciones

- `constitution.md` (GOV-01..18, R-001..060) y el Gate H04 aprobado explícitamente el **2026-11-05** tienen precedencia; H05 diseña infraestructura, no la ejecuta.
- Arquitectura ratificada: Go (Clean Architecture), AWS, DynamoDB, **Redis real**, **2 instancias simultáneas**, balanceador HTTPS y CI/CD/DevSecOps. Ninguno es demostración opcional.
- El PDF del docente v3 exige Backend cloud con costo cero; el catálogo V6.0.2 enumera 75 rutas propuestas E001..E075 y utiliza stack histórico GCP/MongoDB **descartado**.
- No crear código Go, archivos Terraform `.tf`, pipelines ni endpoints H06 durante H05; `terraform apply/destroy`, `git push`, cambios AWS y facturación están prohibidos hasta fases y aprobaciones autorizadas.
- **Regla de evidencia:** documentos pueden alcanzar DRAFT completo sin acceso cloud; Gate I-GO requiere demostrar **viabilidad operacional y económica**, no asumirla. Campos pendientes no se cambian a PASS ni se solventan con créditos.

## 2. Contrato de infraestructura propuesto

### INF-060-01 — Identidades

Role EC2 de runtime con permisos Dynamo específicos (PK/bounds, actions), métricas/logs y secreto Redis con acceso puntual; rol de CI solo pruebas/artefactos; rol deployment sólo asumible tras approval y trust GitHub OIDC repo/ref/environment exactos.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-060-02 — SCP

Comprobación de identidad (`sts get-caller-identity`) y consultas IAM/SCP por consola/read-only con permisos de administrador autorizado. Un SCP Deny no se resuelve agregando IAM Allow: escalar al titular. No usar root ni crear llaves permanentes en GitHub.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-060-03 — Secretos

Plan preferente con servicio aprobado y costo incluido; KMS/Secrets Manager/SSM Parameter Store evaluados por precio/plan. JWT signing key rotada por `kid`, refresh hashes en Dynamo; Redis password fuera de IaC state/logs.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-060-04 — Least privilege

Rutas de recurso IAM con ARN exacto por tabla/índice, Conditions donde apliquen; policy simulator o IAM Access Analyzer si disponible. Políticas de red y cifrado en tránsito/red.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-060-05 — GitHub OIDC

Condicionar aud, sub para organización `nexumpr` y branch/environment autorizado; separación de rol `plan` y `apply`. `permissions: id-token: write` únicamente job autorizado. No añadir secrets AWS_ACCESS_KEY_ID.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-060-06 — Auditoría

Registrar cambios IAM relevantes y alertas, no exponer tokens, AWS account ID ni rutas internas en output público; evidencia con ARN enmascarado.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

## 3. Dependencias y secuencia

I-000, I-040, I-050, I-070, I-080, I-090

## 4. Criterios de aceptación de diseño (Gate H05)

- Matriz principal→acción→recurso con IAM mínimo.
- SCP detectado no se elude: decisión responsable.
- OIDC trust política restringe repo/branch/environment.
- No hay claves persistentes o secretos GitHub para AWS.
- Retiro de credenciales y revocación de emergencia documentados.

## 5. Riesgos que impiden una aprobación sin evidencia

- **Riesgo:** SCP bloquea IAM/EC2/ALB. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.
- **Riesgo:** GitHub OIDC no permitido. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.
- **Riesgo:** Servicios secretos con tarifa. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.
- **Riesgo:** Estado Terraform contiene secretos. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.

## 6. Entregables y referencias

Los siguientes nombres identifican evidencias/artefactos **futuros** de I-GO y de implementación; no son archivos creados en H05: `MATRIZ_IAM_SCP.md`, `TRUST_OIDC.md`, `ROTACION_SECRETOS.md`.

## 7. Criterio de bloqueo y aprobación

**BLOCKED** si falta una obligación técnica, contradice alguna spec A aprobada, no puede comprobarse la viabilidad Free/SCP o no hay aceptación humana del riesgo. **DRAFT** significa que el diseño está documentado; nunca equivale a infraestructura aplicada.
