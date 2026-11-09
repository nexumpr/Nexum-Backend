# Estado único y Gates — Nexum-Backend (activación H08-A)

**H08-A ratificado por el operador; H08-I/I-000 sujeto a ratificación explícita del paquete de transición.** Nunca asumir que el archivo por sí solo firma o ejecuta el Gate.

| Lote | Estado | Límite |
|---|---|---|
| H03 | APPROVED | Constitución, 8 agentes, 9 skills base; RFC-001 adapta perfiles en H08-A |
| H04 | APPROVED | 12 specs A-000..A-110 sin cambios de contenidos |
| H05 | APPROVED_DOCUMENTAL_CONDICIONADO | I-000..I-150 intactas; AWS real bloqueada hasta FinOps, IAM, TLS y despliegue autorizado |
| H06 | APPROVED_DOCUMENTAL_BASELINE | 75 contratos iniciales, posible ampliación RFC; fuentes externas no cotejadas de nuevo; implementación NO verificada |
| H07 | APPROVED_DOCUMENTAL_PUBLICADO | PR #1 y `main` dc23e9b, GitHub Actions success run 42138280237 (evidencia del usuario) |
| H08-A | IMPLEMENTED_LOCAL_INTEGRATED_MAIN | PR #3..#13 mergeados; Go build/vet/test PASS (evidencia del operador); AWS NOT_RUN |
| H08-I / I-000 | AUTHORIZED_I000_LOCAL_AWS_READONLY **solo tras ratificación** | Trabajo FinOps/consultas AWS lectura y PR por agente; cero infraestructura mutable; 0 MXN personales |
| H08-I / I-010 | IMPLEMENTED_LOCAL_NOT_DEPLOYED | Módulo `red` validado (`fmt`/`validate`); AWS NOT_RUN |
| H08-I / I-020 | IMPLEMENTED_LOCAL_NOT_DEPLOYED | Módulo `computo` (2 ARM64, cpu_credits standard) y bootstrap systemd; AWS NOT_RUN; despliegue bloqueado por RFC-004 |
| H08-I / I-030 | **INTEGRADO_MAIN — NO DESPLEGADO** | PR #22 MERGED (merge `cc19a77b14d73f4a146baa899a2febe677c267fa`); ALB interno + CloudFront VPC origin validados y fusionados; H-01, RFC-004, readiness 503, CI Terraform (I-080) y verificación AWS siguen abiertos; AWS NOT_RUN |
| H08-I / I-040 | **INTEGRADO_MAIN — NO DESPLEGADO** | PR #24 MERGED (merge `51e01f6eae37d548b3b790a967859ff314ed6bc7`); tabla única `NEXUM_CORE_<ENV>`, GSI1, TTL `expiresAt`, SSE, PITR y matriz de accesos validados; GSI2 opt-in; AWS NOT_RUN |
| H08-I / I-050 | **INTEGRADO_MAIN — NO DESPLEGADO** | PR #25 MERGED (merge `8c314015dcd065f9188bde239be26ddf76cfc1c1`); ElastiCache Redis OSS `cache.t4g.micro` 1 nodo, TLS + RBAC con `default` deshabilitado, ACL `@transaction`, subnet group privado, sin backups; AWS NOT_RUN; secreto RBAC bloqueado por RFC-004/state |
| H08-I / I-060 | **INTEGRADO_MAIN — NO DESPLEGADO** | PR #26 MERGED (merge `c8e37a78efaa1c9ddc0e412b2b96d465eec4911b`); rol/instance profile runtime con DynamoDB exacto (Query en tabla e índices, LeadingKeys + Null:false); roles OIDC trust-only condicionales; `AWS_SCP_NOT_VERIFIED`; secretos `BLOCKED_RFC004` |
| H08-I / I-070 | **INTEGRADO_MAIN — NO DESPLEGADO** | PR #27 MERGED (merge `21eb9cb540e4d4a39a0851a2c3f730fd78a0846f`); raíz `bootstrap` con state S3 versionado/SSE-S3/privado y locking nativo `use_lockfile` (sin tabla DynamoDB); backend remoto y `apply` NO ejecutados; AWS NOT_RUN |
| H08-I / I-080 | **INTEGRADO_MAIN — NO DESPLEGADO** | PR #28 MERGED (merge `4262972e5d423a01ba8be89642fa423e6a764003`); CI de seis jobs con Trivy real, cobertura 92.0 %, OpenAPI, SBOM y `validar`; RFC-006 aprobada (WAF temporal hasta 2026-12-02); branch protection `BLOQUEADO_PLAN_GITHUB`; AWS NOT_RUN |
| H08-I / I-090 | **INTEGRADO_MAIN — NO DESPLEGADO** | PR #29 MERGED (merge `a67ea4b929e3ca5a9fe246f68f852931e516eb5d`); CD manual con entorno, autorización por SHA, artefacto verificado, S3 inmutable, refresh con estado terminal y rollback seguro; `ARTIFACT_DELIVERY` bloqueado por I-060/I-020; 0 ejecuciones CD; AWS NOT_RUN |
| H08-I / I-100 | **INTEGRADO_MAIN — NO DESPLEGADO** | PR #30 MERGED (merge `6542e724e0b203d8074f41b5abaad739a9b09feb`); threat model, escáner IaC por bloques, pruebas adversariales y DAST bloqueado sin Gate; `D-100-01`/`D-100-02` (A-030/A-080) abiertos; AWS NOT_RUN |
| H08-I / I-110 | **IMPLEMENTADO_LOCAL — PR PENDIENTE** | `APRUEBO_GATE_I110_LOCAL` 2026-11-08; módulo de observabilidad (logs 7 d, 3 metric filters, 5 alarmas, SNS, presupuesto y dashboard), `RequestRecord` y pruebas de contrato/FinOps; agente CloudWatch y AWS `NOT_RUN` |
| H08-I / I-120..I-150; H08-E / E001..E075 | NOT_AUTHORIZED | Gates por spec futuros; despliegue requiere autorización humana independiente |

El Gate H08-A se registra en `sdd/_control/H08/GATE_H06_H07_H08A_RATIFICACION.md`. Shell con ASK **no** es un sandbox: el usuario revisa cada orden. El registro de `main` debe seguir limpio; los cambios de H08-A se publican como PR y pasan checks.

## Transición operativa 6-nov-2026
RFC-002 habilita edición/shell/GitHub CI controlados por ASK; RFC-003 registra irregularidades documentales de los 24 tasks/checklists A-000..A-110 sin reescribir sus contenidos; Gate `sdd/_control/H08/GATE_H08I_I000.md` registra ratificación I-000. AWS real, presupuesto y aptitud 45 días siguen NOT_VERIFIED.

## Ratificación D2 de I-030 (6-nov-2026)
El responsable concedió `RATIFICO_D2_PARA_I030`, con alcance limitado a registrar la decisión TLS y resolver la
contradicción con el DRAFT de I-030. Se registra en `sdd/_control/H08/RFC-005-ENMIENDA_I030_TLS_D2.md` porque
la política `edit` deniega escribir `sdd/**/spec.md` y `sdd/**/plan.md`; el cuerpo original permanece intacto y
`tasks.md`/`checklist.md`/este Gate reflejan la precedencia. Implica: CloudFront HTTPS (`*.cloudfront.net`) →
VPC Origin → ALB interno HTTP:80 → 2 instancias Go. **No** autoriza Terraform, merge, despliegue ni gasto.

**Integración (2026-11-07):** el diseño D2 se implementó y fusionó en `main` con el PR **#22** (merge
`cc19a77b14d73f4a146baa899a2febe677c267fa`). Sigue **sin** desplegarse: `AWS_NOT_RUN`.
