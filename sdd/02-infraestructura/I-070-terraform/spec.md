# I-070-terraform — IaC Terraform: stacks, state, drift y seguridad

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-110, I-000..I-060 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## 1. Objetivo y definición de terminado

Definir blueprint Terraform reproducible para crear y destruir TODO el runtime de AWS, sin ejecutar comandos ni crear código .tf en H05.

## Autoridad y restricciones

- `constitution.md` (GOV-01..18, R-001..060) y el Gate H04 aprobado explícitamente el **2026-11-05** tienen precedencia; H05 diseña infraestructura, no la ejecuta.
- Arquitectura ratificada: Go (Clean Architecture), AWS, DynamoDB, **Redis real**, **2 instancias simultáneas**, balanceador HTTPS y CI/CD/DevSecOps. Ninguno es demostración opcional.
- El PDF del docente v3 exige Backend cloud con costo cero; el catálogo V6.0.2 enumera 75 rutas propuestas E001..E075 y utiliza stack histórico GCP/MongoDB **descartado**.
- No crear código Go, archivos Terraform `.tf`, pipelines ni endpoints H06 durante H05; `terraform apply/destroy`, `git push`, cambios AWS y facturación están prohibidos hasta fases y aprobaciones autorizadas.
- **Regla de evidencia:** documentos pueden alcanzar DRAFT completo sin acceso cloud; Gate I-GO requiere demostrar **viabilidad operacional y económica**, no asumirla. Campos pendientes no se cambian a PASS ni se solventan con créditos.

## 2. Contrato de infraestructura propuesto

### INF-070-01 — Roots y módulos

Definir `infra/terraform/bootstrap` (state si aprobado), `infra/terraform/environments/dev` y módulos vpc/security, compute, lb/tls, dynamodb, redis, observability, iam, dns. Cada entorno variables explícitas, labels y tagging.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-070-02 — State

Backend remoto S3 cifrado/versionado con locking compatible elegido tras precio/permiso; bootstrap con control humano. Alternativa local solo para diseño/offline; nunca state real sensible en Git, log o release. Costo bucket/state incluido en I-000.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-070-03 — Plan y lock

`terraform fmt -check`, `terraform init -backend=false` cuando aplique, `validate`, `plan -out=...` solo H08 con credenciales autorizadas; artifact plan revisado por humano antes de apply. Almacenamiento seguro y TTL del plan.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-070-04 — Drift

`plan -refresh-only`/diff documentado, no corregir cambios inesperados automáticamente. `prevent_destroy` para datos persistentes, doble confirmación en eliminación de recursos no efímeros.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-070-05 — Secuencia

Bootstrap (fuera de runtime), network, IAM, Dynamo, Redis, EC2/ALB, observability, DNS. Destruir runtime en orden inverso; datos y state se preservan salvo autorización separada.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-070-06 — Condiciones

No hardcode secrets, no `-auto-approve`, no deploy desde PR no confiable, no `terraform destroy` masivo por ahorro, ni cambiar AWS Free a Paid.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

## 3. Dependencias y secuencia

I-000..I-060, I-080/I-090/I-120/I-130

## 4. Criterios de aceptación de diseño (Gate H05)

- Lista exhaustiva de recursos/variables/outputs y dependencias.
- Plan reproducible y sin secretos en H08.
- Gates humanos before apply/destroy, backend/state preservado.
- Drift identificable, rollback de IaC definido.
- Modelos completos de tables/GSI y Redis sin reemplazos.

## 5. Riesgos que impiden una aprobación sin evidencia

- **Riesgo:** Servicio backend state cobrable o SCP bloqueado. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.
- **Riesgo:** Lock incompatible o terraform version drift. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.
- **Riesgo:** Destroy borra datos y provoca pérdida. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.

## 6. Entregables y referencias

Los siguientes nombres identifican evidencias/artefactos **futuros** de I-GO y de implementación; no son archivos creados en H05: `BLUEPRINT_MODULOS_TF.md`, `MATRIZ_RECURSOS_STATE.md`, `RUNBOOK_IAC.md`.

## 7. Criterio de bloqueo y aprobación

**BLOCKED** si falta una obligación técnica, contradice alguna spec A aprobada, no puede comprobarse la viabilidad Free/SCP o no hay aceptación humana del riesgo. **DRAFT** significa que el diseño está documentado; nunca equivale a infraestructura aplicada.
