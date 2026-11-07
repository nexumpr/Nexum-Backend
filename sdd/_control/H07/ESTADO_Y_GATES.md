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
| H08-I / I-040 | **IMPLEMENTADO_LOCAL — PR PENDIENTE** | `APRUEBO_GATE_I040_LOCAL` 2026-11-07; tabla única `NEXUM_CORE_<ENV>` (PAY_PER_REQUEST, GSI1, TTL `expiresAt`, SSE, PITR), `fmt`/`validate` y 33 pruebas locales; GSI2 opt-in sin scheduler; AWS NOT_RUN |
| H08-I / I-050..I-150; H08-E / E001..E075 | NOT_AUTHORIZED | Gates por spec futuros; despliegue requiere autorización humana independiente |

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
