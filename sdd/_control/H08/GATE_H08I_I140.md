# Gate H08-I / I-140 — Operación, incidentes y drills (LOCAL)

**FRASE HUMANA REGISTRADA:** `APRUEBO_GATE_I140_LOCAL` (2026-11-09).

**ESTADO:** `AUTHORIZED_I140_LOCAL` — implementación **exclusivamente local**. Sin `terraform
plan/apply/destroy`, sin AWS, sin despliegue y sin drills reales.

## 1. Alcance autorizado

- `sdd/_control/H08/RUNBOOK_OPERATIVO.md` con 10 procedimientos y 8 campos obligatorios.
- `sdd/_control/H08/MATRIZ_INCIDENTES.md` (S1–S4) y `PLAN_PRUEBAS_OPERACION.md` (5 drills).
- `herramientas/verificar_operacion_i140.py` con 5 comandos de validación local.
- Pruebas adversas, documentos de contrato y actualización de estado/integridad.
- Commit, push, PR y seguimiento de CI. **Merge requiere autorización humana separada.**

## 2. NO autorizado

- Ejecutar drills, pruebas de carga o DAST contra AWS.
- `terraform plan/apply/destroy`, refresh remoto y cualquier mutación cloud.
- Abrir SSH público, usar root, cambiar la SCP o crear claves estáticas.
- Modificar I-020, I-030, I-050, I-090, I-110, I-120 o I-130.
- Despliegue, restauración de datos o cierre de incidentes reales.
- I-150 y H08-E: siguen `NOT_AUTHORIZED`.

## 3. Decisiones de diseño

1. **Nada escrito acredita ejecución:** los 10 runbooks y los 5 drills quedan `DOCUMENTADO_NO_EJECUTADO`
   / `NOT_RUN`, y el validador lo exige explícitamente.
2. **Prioridades coherentes, no inventadas:** se valida la forma (S1 ≤ S2 ≤ S3 en tiempo de
   notificación) y se declara `PENDIENTE_DE_RATIFICACION_HUMANA`; ningún SLA se da por aprobado.
3. **Drills sin destrucción:** se rechazan estados `EJECUTADO_AWS` en fase local, acciones
   `delete/destroy/deshabilitar` y drills con AWS sin datos sintéticos.
4. **Inventario desde el código:** el verificador cuenta recursos declarados por módulo y declara
   `existe_en_aws = NOT_VERIFIED`; un nombre no prueba existencia.
5. **Acceso mínimo:** se rechazan root, SSH público, claves estáticas, cambio de SCP y SSM fuera de
   ventana.

## 4. Entregables

| Artefacto | Contenido |
|---|---|
| `RUNBOOK_OPERATIVO.md` | 10 procedimientos (`deployment`, `health-fail`, `dynamo-throttle`, `redis-unavailable`, `certificados`, `rotacion-secretos`, `auth-inesperado`, `presupuesto`, `incidente`, `eliminacion-iac`) |
| `MATRIZ_INCIDENTES.md` | S1–S4 con notificación, escalamiento y rollback |
| `PLAN_PRUEBAS_OPERACION.md` | 5 drills con degradación, criterio, rollback y AWS requerido |
| `MATRIZ_INF_140.md` | contratos INF-140-01..06 y dependencias |
| `verificar_operacion_i140.py` | `runbooks`, `prioridades`, `drills`, `inventario`, `acceso` |
| `tests/infra/operacion/test_i140_operacion.py` | 36 pruebas adversas |
| `tests/fixtures/operacion/` | prioridades, drills y política de acceso |

## 5. Verificación

| Comprobación | Resultado |
|---|---|
| `verificar_operacion_i140.py runbooks` | `RUNBOOKS_COMPLETOS` |
| `... prioridades` | `PRIORIDADES_COHERENTES` (ratificación pendiente) |
| `... drills` | `PLAN_VALIDO`, `ejecucion_aws = NOT_RUN` |
| `... inventario` | `INVENTARIO_DERIVADO`, `existe_en_aws = NOT_VERIFIED` |
| `... acceso` | `ACCESO_CONFORME` |
| Suite `tests/infra/operacion` | 140 OK (104 de I-120 + 36 de I-140) |
| `terraform fmt/init -backend=false/validate` | PASS (sin cambios de IaC en esta spec) |
| Trivy / Gitleaks / Go / SDD / SHA-256 | PASS |

## 6. Bloqueos y dependencias

- `RFC-007` (`BLOCKED_RFC007_SNS_KMS`): sin notificación automática de alarmas.
- Agente CloudWatch pendiente (I-020): logs reales `NOT_RUN`.
- `RFC-004` `PROPUESTO`: rotación de secretos no ejecutable.
- `H-01`, `RFC-006`, `AWS_SCP_NOT_VERIFIED`, readiness 503, OIDC y branch protection sin cambios.
- Drills reales y cierre de incidentes: `NOT_RUN` hasta Gate de despliegue.

## 7. Registro

| Fecha | Hecho |
|---|---|
| 2026-11-09 | `APRUEBO_GATE_I140_LOCAL` concedido; rama `feat/h08i-i140-operacion` desde `main` `da2aee1` (incluye el cierre documental de I-130) |
| 2026-11-09 | Implementación local completada; PR y CI por el agente; merge pendiente de autorización humana ligada a PR + HEAD |
