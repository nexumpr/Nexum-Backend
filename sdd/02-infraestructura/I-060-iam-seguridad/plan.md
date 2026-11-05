# I-060-iam-seguridad — IAM, límites SCP, secretos y accesos

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-050, I-000, I-010, I-020, I-040, I-050 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## 1. Estrategia de diseño documental de H05

Definir identidades separadas para ejecución Go, Terraform y CI/CD con mínimo privilegio, MFA humano, rotación y límites SCP, sin pedir ni revelar credenciales en archivos.

## Autoridad y restricciones

- `constitution.md` (GOV-01..18, R-001..060) y el Gate H04 aprobado explícitamente el **2026-11-05** tienen precedencia; H05 diseña infraestructura, no la ejecuta.
- Arquitectura ratificada: Go (Clean Architecture), AWS, DynamoDB, **Redis real**, **2 instancias simultáneas**, balanceador HTTPS y CI/CD/DevSecOps. Ninguno es demostración opcional.
- El PDF del docente v3 exige Backend cloud con costo cero; el catálogo V6.0.2 enumera 75 rutas propuestas E001..E075 y utiliza stack histórico GCP/MongoDB **descartado**.
- No crear código Go, archivos Terraform `.tf`, pipelines ni endpoints H06 durante H05; `terraform apply/destroy`, `git push`, cambios AWS y facturación están prohibidos hasta fases y aprobaciones autorizadas.
- **Regla de evidencia:** documentos pueden alcanzar DRAFT completo sin acceso cloud; Gate I-GO requiere demostrar **viabilidad operacional y económica**, no asumirla. Campos pendientes no se cambian a PASS ni se solventan con créditos.

## 2. Plan secuencial y verificaciones

### P-060-01 — Identidades

**Entrada:** H04 aprobado, requisitos `A-050, I-000, I-010, I-020, I-040, I-050`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Role EC2 de runtime con permisos Dynamo específicos (PK/bounds, actions), métricas/logs y secreto Redis con acceso puntual; rol de CI solo pruebas/artefactos; rol deployment sólo asumible tras approval y trust GitHub OIDC repo/ref/environment exactos.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-060-02 — SCP

**Entrada:** H04 aprobado, requisitos `A-050, I-000, I-010, I-020, I-040, I-050`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Comprobación de identidad (`sts get-caller-identity`) y consultas IAM/SCP por consola/read-only con permisos de administrador autorizado. Un SCP Deny no se resuelve agregando IAM Allow: escalar al titular. No usar root ni crear llaves permanentes en GitHub.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-060-03 — Secretos

**Entrada:** H04 aprobado, requisitos `A-050, I-000, I-010, I-020, I-040, I-050`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Plan preferente con servicio aprobado y costo incluido; KMS/Secrets Manager/SSM Parameter Store evaluados por precio/plan. JWT signing key rotada por `kid`, refresh hashes en Dynamo; Redis password fuera de IaC state/logs.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-060-04 — Least privilege

**Entrada:** H04 aprobado, requisitos `A-050, I-000, I-010, I-020, I-040, I-050`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Rutas de recurso IAM con ARN exacto por tabla/índice, Conditions donde apliquen; policy simulator o IAM Access Analyzer si disponible. Políticas de red y cifrado en tránsito/red.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-060-05 — GitHub OIDC

**Entrada:** H04 aprobado, requisitos `A-050, I-000, I-010, I-020, I-040, I-050`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Condicionar aud, sub para organización `nexumpr` y branch/environment autorizado; separación de rol `plan` y `apply`. `permissions: id-token: write` únicamente job autorizado. No añadir secrets AWS_ACCESS_KEY_ID.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-060-06 — Auditoría

**Entrada:** H04 aprobado, requisitos `A-050, I-000, I-010, I-020, I-040, I-050`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Registrar cambios IAM relevantes y alertas, no exponer tokens, AWS account ID ni rutas internas en output público; evidencia con ARN enmascarado.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

## 3. Orden de ejecución futura H08 (no ejecutar ahora)

1. Auditar plan AWS/credenciales/precios y límites SCP sin cambios.
2. Preparar revisión de cambios, artefactos IaC y análisis económico en PR autorizado.
3. Ejecutar validaciones (fmt/validate/plan/pruebas), someter al gate humano.
4. Aprovisionar o modificar únicamente después de autorización de release y protección de datos.
5. Ejecutar pruebas reales, registrar métricas, costos y rollback, y no declarar DONE antes de las evidencias.

## 4. Dependencias cruzadas

I-000, I-040, I-050, I-070, I-080, I-090

## 5. Riesgos y tratamiento

1. `I-060-iam-seguridad-R01`: SCP bloquea IAM/EC2/ALB. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.
2. `I-060-iam-seguridad-R02`: GitHub OIDC no permitido. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.
3. `I-060-iam-seguridad-R03`: Servicios secretos con tarifa. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.
4. `I-060-iam-seguridad-R04`: Estado Terraform contiene secretos. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.

## 6. Puerta de entrada a H06

Gate H05/I-GO NO se logra con instalación de Markdown: requiere pruebas de viabilidad económica/cuenta, trazabilidad física Dynamo/Redis, controles IAM y aceptación expresa del responsable. Los contratos detallados E001..E075 se formalizan exclusivamente en H06.
