# H09-90 — Preparación cloud, gobierno y FinOps (2026-11-27)

**Estado:** `PREPARADO_LOCAL` / `AWS_NOT_RUN` / `GITHUB_SETTINGS_NOT_CHANGED` / `MERGE_NO_AUTORIZADO`
**Base:** `origin/main` `1d5778dec3f5276bcc8f7e6a5979044a83418682` · **Release:** `v0.21.4+5bef784b2d1c`
**Alcance:** cambios Terraform preparados (sin apply), RFC-007 propuesta, decisión WAF/TLS, propuesta de gobierno GitHub, estrategia DAST y presupuesto. No cambia endpoints, contrato Android ni recursos productivos.

## 1. Terraform preparado (sin apply)

| Cambio | Archivo | Efecto en el próximo plan | Coste |
|---|---|---|---|
| `dynamodb_proteccion_borrado` por defecto **true** | `envs/h08i/variables.tf` | Update in-place de `aws_dynamodb_table` (deletion protection) | 0 |
| ARN determinista del log group para el rol runtime | `envs/h08i/main.tf` (`locals.iam_log_group_arns`) | Update in-place de la política IAM del runtime: `logs:CreateLogStream`/`PutLogEvents` sobre `/nexum/h08i/backend` | 0 |
| Invariante anti-ciclo | mismo bloque | Sin referencia a `module.observabilidad`; evita el ciclo IAM→observabilidad→balanceo→computo→IAM | 0 |

**No aplicado.** El plan exacto requiere `terraform plan` con las variables de custodia y un Gate por SHA-256. No se crean recursos, alarmas, SNS ni agentes.

**Pendiente para H09-2 (no incluido aquí):** instalación del agente CloudWatch y su configuración de journald. El agente debe leer `journald` (no solo archivos) y publicar en el log group existente; requiere actualizar el Launch Template y ejecutar un `instance refresh` con Gate. No se instala un agente que no pueda leer el journal.

## 2. RFC-007 — notificación operativa (propuesta, no aprobada)

- **Problema:** 14 alarmas definidas sin `alarm_actions`; `ALERT_DELIVERY=BLOCKED_RFC007_SNS_KMS`.
- **Opción recomendada:** SNS topic con suscripción email, cifrado con CMK (`alias/nexum-h08i-alerts`), y `alarm_actions` en las alarmas críticas (A1 targets, A2 5xx, A4 readiness, A5 sin logs, A7 Redis, A8 DynamoDB).
- **Coste estimado:** CMK 1 USD/mes + SNS email dentro del free tier + solicitudes mínimas ≈ **1–2 USD/mes** (21 días: ≈0.7–1.4 USD). Recotizar antes del apply.
- **Prueba de recepción:** publicar un mensaje de prueba en el topic con `Subject=PRUEBA H09-2` y confirmar recepción humana; no se provoca una caída ni se fuerza una alarma real.
- **Alternativa sin CMK:** notificación por email de AWS Budgets solo para coste; **no** sustituye alertas operativas de incidentes.
- **Gate requerido:** `APRUEBO_RFC_007_NOTIFICACION_OPERATIVA`.

## 3. WAF y TLS — decisión pendiente antes del 2026-12-02

### WAF
- `RFC-006` está **APROBADA** y vence el **2026-12-02** o en el Gate de despliegue AWS, lo que ocurra primero. La excepción **no** es un control implementado.
- CloudFront no tiene WebACL (`WebACLId` vacío) y no hay `wafv2` en Terraform.
- **Propuesta:** WebACL `CLOUDFRONT` con reglas administradas básicas (p. ej. `AWSManagedRulesCommonRuleSet`) en modo **Count** inicial, límite de tasa y métricas; pasar a Block tras revisar falsos positivos. Coste orientativo: ~5 USD/ACL/mes + ~1 USD/regla/mes + solicitudes; **no verificado** contra la cuenta Free.
- **No migrar** CloudFront a flat-rate Free: no admite VPC Origins y Nexum depende de ellos.
- Si WAF no es elegible en el plan Free, la alternativa real es renovar RFC-006 con controles compensatorios (origen privado, SG por prefijo, rate limiting de aplicación, headers) y **no** declarar WAF implementado.

### TLS
- CloudFront con certificado por defecto impone **TLSv1** (`MinimumProtocolVersion=TLSv1`, verificado). No se puede forzar TLS1.2 con ese certificado.
- Opciones legítimas: dominio propio bajo control + certificado ACM en `us-east-1` + `MinimumProtocolVersion=TLSv1.2_2021`. Requiere DNS y no cambia la URL Android sin RFC.
- Mientras no exista dominio/certificado, se mantienen HSTS y headers de seguridad; **no se otorgan puntos** por TLS moderno.

## 4. Gobierno GitHub — propuesta (no aplicada)

**Verificado:** `main` con `strict=true`, 13 checks, 1 revisor, force-push y borrado OFF; `enforce_admins=false`; sin rulesets; sin environments; repositorio público; sin tags ni Releases.

Propuesta de ruleset (aplicar solo con Gate):

```json
{
  "name": "main-protegida-h09",
  "target": "branch",
  "enforcement": "active",
  "conditions": { "ref_name": { "include": ["refs/heads/main"], "exclude": [] } },
  "rules": [
    { "type": "deletion" },
    { "type": "non_fast_forward" },
    { "type": "required_linear_history" },
    { "type": "pull_request", "parameters": {
        "required_approving_review_count": 1,
        "dismiss_stale_reviews_on_push": true,
        "require_code_owner_review": false,
        "require_last_push_approval": false,
        "required_review_thread_resolution": true
    }},
    { "type": "required_status_checks", "parameters": {
        "strict_required_status_checks_policy": true,
        "required_status_checks": [
          "Deteccion de cambios","Formato, vet, build y pruebas","Analisis estatico y vulnerabilidades",
          "Escaneo de secretos","Validacion Terraform e IaC","Contratos OpenAPI y herramientas",
          "SBOM de dependencias","SAST Semgrep","Integracion HTTP y concurrencia","PR Quality Gate",
          "Hardening Report","CI Success","validar"
        ]
    }}
  ],
  "bypass_actors": []
}
```

**Tags y Release:** tag anotado `v0.21.4+5bef784b2d1c` sobre `5bef784b2d1c` (SHA productivo, no el merge documental), Release con binario SHA-256 `79058a64…7f50`, run CI `45872103515`, SBOM y notas. **No creado.**

**CD:** environment `aws-deploy` inexistente, OIDC con **0 proveedores** y SCP que bloquea `iam:CreateOpenIDConnectProvider`. No se usan claves permanentes. La alternativa es CD asistido reproducible (plan custodiado por SHA-256, refresh con `MinHealthyPercentage=100`, rollback y verificación), documentado como **no automatizado**.

## 5. DAST — estado y estrategia

- `DAST_ZAP = BLOCKED_TOOLING`: no hay ZAP/Java instalado y el uso de Docker está prohibido.
- `DAST_LOCAL_ATTACK_SUITE = PASS`: `tests/security/adversarial_test.go` y `tests/integration` se ejecutan en CI con `-race`; cubren JWT manipulado, IDOR, replay, cuotas, CORS, cabeceras, cuerpos >1 MB y 429. **No se etiqueta como ZAP ni como escaneo autenticado completo.**
- Estrategia autorizada: job de CI que levante el binario en `127.0.0.1` con dobles sintéticos, importe el OpenAPI y ejecute ZAP baseline/API scan solo contra loopback, sin egreso externo. Requiere añadir ZAP al runner (acción fijada por SHA) y un Gate `DAST`; no se ejecutó contra producción.

## 6. Lecturas AWS necesarias (una sola autorización acotada)

```text
aws dynamodb describe-continuous-backups --table-name NEXUM_CORE_H08I --region us-east-2
aws dynamodb describe-time-to-live --table-name NEXUM_CORE_H08I --region us-east-2
aws s3api get-bucket-versioning --bucket nexum-h08i-artefactos-713797602090
aws s3api get-bucket-encryption --bucket nexum-h08i-artefactos-713797602090
aws s3api get-public-access-block --bucket nexum-h08i-artefactos-713797602090
aws s3api get-bucket-policy-status --bucket nexum-h08i-artefactos-713797602090
aws cloudwatch describe-alarms --region us-east-2
aws logs describe-log-streams --log-group-name /nexum/h08i/backend --order-by LastEventTime --descending --max-items 10
aws sns list-topics --region us-east-2
aws elasticache describe-users --region us-east-2
aws elasticache describe-user-groups --region us-east-2
aws elbv2 describe-load-balancer-attributes --load-balancer-arn <arn-alb>
```

Ninguna es mutante. Sin ellas, PITR, S3, alarmas, SNS, RBAC y modo XFF efectivo quedan `NOT_VERIFIED`.

## 7. FinOps — continuidad hasta el 18-dic

- Créditos: **121.623752 USD** (2026-11-27T17:30Z). Consumo medido: **3.90 USD/día**.
- 23 días × 3.90 = **89.70 USD** ⇒ saldo ≈ **31.92 USD**; reserva de 25 USD cumplida con ≈6.92 USD de holgura.
- Umbral para conservar 25 USD en 23 días: **4.20 USD/día**. El margen es 0.30 USD/día: se mide a diario.
- Presupuesto incremental H09-2/H09-3: CloudWatch (logs dentro del free tier, 4 alarmas facturables ≈0.40 USD/mes), SNS+CMK ≈1–2 USD/mes, WAF ≈6–8 USD/mes, restore temporal ≈0.30 USD por prueba. Total orientativo **8–12 USD/mes**; **requiere Gate y recotización**.
- Los 40 USD de Explore no se contabilizan hasta acreditación.

## 8. Rúbrica (sin cambios por preparación)

| Dimensión | Actual | Meta | Bloqueo real |
|---|---:|---:|---|
| SAST/SCA | 9 | 9 | — |
| Gestión de secretos | 7 | 9 | inventario/rotación y lectura de buckets |
| Seguridad IaC/cloud | 8 | 9 | WAF y remediación Trivy |
| Autenticación/sesión | 9 | 9 | — |
| Validación API | 9 | 10 | E015 legal |
| Protección de datos | 7 | 9 | apply deletion protection + PITR/S3 verificados |
| Red/TLS | 4 | 7 | WAF + dominio/ACM |
| Pruebas/verificación | 7 | 10 | DAST real + más cobertura |
| CI/CD/gobierno | 5 | 9 | ruleset, tags, environment/CD |
| Observabilidad/respuesta | 4 | 9 | agente + ingesta + acciones de alarma |
| **Total** | **69** | **90** | — |

**Máximo acreditable sin mutaciones AWS/GitHub:** ≈72 (solo Pruebas +1 al fusionar #162 y documentación verificable). **85–90 exige ejecutar los Gates cloud**; preparar Terraform no otorga puntos.
