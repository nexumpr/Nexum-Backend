# I-070-terraform — IaC Terraform: stacks, state, drift y seguridad

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-110, I-000..I-060 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## 1. Estrategia de diseño documental de H05

Definir blueprint Terraform reproducible para crear y destruir TODO el runtime de AWS, sin ejecutar comandos ni crear código .tf en H05.

## Autoridad y restricciones

- `constitution.md` (GOV-01..18, R-001..060) y el Gate H04 aprobado explícitamente el **2026-11-05** tienen precedencia; H05 diseña infraestructura, no la ejecuta.
- Arquitectura ratificada: Go (Clean Architecture), AWS, DynamoDB, **Redis real**, **2 instancias simultáneas**, balanceador HTTPS y CI/CD/DevSecOps. Ninguno es demostración opcional.
- El PDF del docente v3 exige Backend cloud con costo cero; el catálogo V6.0.2 enumera 75 rutas propuestas E001..E075 y utiliza stack histórico GCP/MongoDB **descartado**.
- No crear código Go, archivos Terraform `.tf`, pipelines ni endpoints H06 durante H05; `terraform apply/destroy`, `git push`, cambios AWS y facturación están prohibidos hasta fases y aprobaciones autorizadas.
- **Regla de evidencia:** documentos pueden alcanzar DRAFT completo sin acceso cloud; Gate I-GO requiere demostrar **viabilidad operacional y económica**, no asumirla. Campos pendientes no se cambian a PASS ni se solventan con créditos.

## 2. Plan secuencial y verificaciones

### P-070-01 — Roots y módulos

**Entrada:** H04 aprobado, requisitos `A-110, I-000..I-060`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Definir `infra/terraform/bootstrap` (state si aprobado), `infra/terraform/environments/dev` y módulos vpc/security, compute, lb/tls, dynamodb, redis, observability, iam, dns. Cada entorno variables explícitas, labels y tagging.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-070-02 — State

**Entrada:** H04 aprobado, requisitos `A-110, I-000..I-060`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Backend remoto S3 cifrado/versionado con locking compatible elegido tras precio/permiso; bootstrap con control humano. Alternativa local solo para diseño/offline; nunca state real sensible en Git, log o release. Costo bucket/state incluido en I-000.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-070-03 — Plan y lock

**Entrada:** H04 aprobado, requisitos `A-110, I-000..I-060`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** `terraform fmt -check`, `terraform init -backend=false` cuando aplique, `validate`, `plan -out=...` solo H08 con credenciales autorizadas; artifact plan revisado por humano antes de apply. Almacenamiento seguro y TTL del plan.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-070-04 — Drift

**Entrada:** H04 aprobado, requisitos `A-110, I-000..I-060`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** `plan -refresh-only`/diff documentado, no corregir cambios inesperados automáticamente. `prevent_destroy` para datos persistentes, doble confirmación en eliminación de recursos no efímeros.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-070-05 — Secuencia

**Entrada:** H04 aprobado, requisitos `A-110, I-000..I-060`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Bootstrap (fuera de runtime), network, IAM, Dynamo, Redis, EC2/ALB, observability, DNS. Destruir runtime en orden inverso; datos y state se preservan salvo autorización separada.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-070-06 — Condiciones

**Entrada:** H04 aprobado, requisitos `A-110, I-000..I-060`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** No hardcode secrets, no `-auto-approve`, no deploy desde PR no confiable, no `terraform destroy` masivo por ahorro, ni cambiar AWS Free a Paid.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

## 3. Orden de ejecución futura H08 (no ejecutar ahora)

1. Auditar plan AWS/credenciales/precios y límites SCP sin cambios.
2. Preparar revisión de cambios, artefactos IaC y análisis económico en PR autorizado.
3. Ejecutar validaciones (fmt/validate/plan/pruebas), someter al gate humano.
4. Aprovisionar o modificar únicamente después de autorización de release y protección de datos.
5. Ejecutar pruebas reales, registrar métricas, costos y rollback, y no declarar DONE antes de las evidencias.

## 4. Dependencias cruzadas

I-000..I-060, I-080/I-090/I-120/I-130

## 5. Riesgos y tratamiento

1. `I-070-terraform-R01`: Servicio backend state cobrable o SCP bloqueado. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.
2. `I-070-terraform-R02`: Lock incompatible o terraform version drift. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.
3. `I-070-terraform-R03`: Destroy borra datos y provoca pérdida. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.

## 6. Puerta de entrada a H06

Gate H05/I-GO NO se logra con instalación de Markdown: requiere pruebas de viabilidad económica/cuenta, trazabilidad física Dynamo/Redis, controles IAM y aceptación expresa del responsable. Los contratos detallados E001..E075 se formalizan exclusivamente en H06.
