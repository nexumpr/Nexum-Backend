# Gate H08-I / I-140 — Operación, incidentes y drills (LOCAL)

**FRASE HUMANA REGISTRADA:** `APRUEBO_GATE_I140_LOCAL` (2026-11-09).

**ESTADO:** `AUTHORIZED_I140_LOCAL` — implementación **exclusivamente local**. Sin `terraform
plan/apply/destroy`, sin AWS, sin despliegue y sin drills reales.

## 1. Alcance autorizado

- `sdd/_control/H08/RUNBOOK_OPERATIVO.md` con **12** procedimientos y **13** campos obligatorios.
- `sdd/_control/H08/MATRIZ_INCIDENTES.md` (S1–S3 con ciclo completo), `PLAN_PRUEBAS_OPERACION.md`
  (**12** drills) y `ACCESO_OPERACIONAL.md`.
- `INVENTARIO_INFRAESTRUCTURA.md` derivado de I-120/I-130 y del catálogo, con `AWS_NOT_VERIFIED`.
- `MATRIZ_INF_140.md` con trazabilidad de **12 A + 16 I + 75 E** y criterios cloud pendientes.
- `herramientas/verificar_operacion_i140.py` con 6 comandos (`runbooks`, `incidentes`, `drills`,
  `aceptacion`, `acceso`, `inventario`).
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

1. **Nada escrito acredita ejecución:** los 12 runbooks y los 12 drills quedan
   `DOCUMENTADO_NO_EJECUTADO` / `NOT_RUN_AWS`, y el validador lo exige.
2. **Incidentes sin SLA inventado:** S1–S3 con propietario, escalamiento y ciclo completo; tiempos
   `PENDIENTE_DECISION_HUMANA`. Se rechazan severidad desconocida, incidente sin propietario,
   S1/S2 sin escalamiento, datos sensibles y evidencia AWS simulada.
3. **Drills sin destrucción ni falsa nube:** estados `LOCAL_PASS`/`LOCAL_FAIL`/`NOT_RUN_AWS`/
   `BLOCKED_DEPENDENCY`; se rechaza `demuestra_aws=true`, `EJECUTADO_AWS`, acciones destructivas y
   un drill con AWS marcado `LOCAL_PASS`.
4. **Inventario desde el código:** el verificador cuenta recursos declarados por módulo y declara
   `existe_en_aws = NOT_VERIFIED`; un nombre no prueba existencia.
5. **Acceso mínimo:** se rechazan root, SSH público, claves estáticas, cambio de SCP y SSM fuera de
   ventana.

## 4. Entregables

| Artefacto | Contenido |
|---|---|
| `RUNBOOK_OPERATIVO.md` | 12 procedimientos × 13 campos |
| `MATRIZ_INCIDENTES.md` | S1–S3, ciclo de 10 fases y reglas de clasificación |
| `PLAN_PRUEBAS_OPERACION.md` | 12 drills con estado y criterio |
| `ACCESO_OPERACIONAL.md` | identidades temporales, SSM en ventana y prohibiciones |
| `INVENTARIO_INFRAESTRUCTURA.md` | recursos core/auxiliares con owner, costo y `AWS_NOT_VERIFIED` |
| `MATRIZ_INF_140.md` | 12 A + 16 I + 75 E y criterios cloud |
| `verificar_operacion_i140.py` | 6 comandos de validación local |
| `tests/infra/operacion/test_i140_operacion.py` | 45 pruebas adversas |
| `tests/fixtures/operacion/` | incidentes, drills y política de acceso |

## 5. Verificación

| Comprobación | Resultado |
|---|---|
| `... runbooks` | `RUNBOOKS_COMPLETOS` (12 × 13 campos) |
| `... incidentes` | `INCIDENTES_VALIDOS` (tiempos `PENDIENTE_DECISION_HUMANA`) |
| `... drills` | `PLAN_VALIDO`, `ejecucion_aws = NOT_RUN` |
| `... aceptacion` | `MATRIZ_COMPLETA` (12 A / 16 I / 75 E) |
| `... inventario` | `INVENTARIO_TRAZABLE`, `existe_en_aws = NOT_VERIFIED` |
| `... acceso` | `ACCESO_CONFORME` |
| Suite `tests/infra/operacion` | 149 OK (104 de I-120 + 45 de I-140) |
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

## 8. Corrección final (2026-11-09)

- **Evidencia de drills:** `LOCAL_PASS` exige `evidencia` con archivo y prueba existentes, resultado
  `PASS` y sin duplicados entre escenarios. Seis drills tienen evidencia real:
  `TestHTTPServerStatusesAndErrors`, `TestGoalReversionPreservesInvariant`,
  `TestIdempotencyManagerBehavior`, `TestRefreshTokenReplayFamilyRevocation`,
  `TestReadinessProbeDegradation503` y `DecisionRollback`. Los cinco que requieren AWS siguen
  `NOT_RUN_AWS` y `perdida-observabilidad` queda `BLOCKED_DEPENDENCY` por I-020.
- **Inventario y costes:** se contabilizan partidas mensuales (÷730 solo para la vista horaria), se
  separan USD/h, USD/mes y consumo variable, y un precio ausente **anula** el total del recurso en vez
  de contarse como 0 (queda en `precios_desconocidos`). Se consolidan los recursos repetidos entre
  I-120 e I-130 (`dynamodb-core`, `terraform-state`, `secretos-bootstrap`) sin duplicar costos y se
  conservan los distintos. Owners y etiquetas salen de fuentes declarativas; lo no declarado se marca
  `PENDIENTE_DECLARACION`. La capacidad del ASG se contrasta con el módulo `computo` (min = desired =
  max = 2) sin afirmar instancias reales.

Evidencia: 167 pruebas en `tests/infra/operacion` (104 de I-120 + 63 de I-140), SDD y SHA-256 PASS.
`AWS_NOT_RUN`, `DRILLS_AWS_NOT_RUN`, `AWS_COST_REAL_NOT_VERIFIED`.

## 9. Corrección de revisión (2026-11-09)

- **A — costos duplicados (defecto real, corregido).** La consolidación concatenaba partidas y facturaba
  dos veces PITR y S3: `dynamodb-core` daba 1.9500 USD/mes (debía 1.3500) y `terraform-state-s3`
  0.1380 (debía 0.0690). Ahora cada cargo se contabiliza una vez por `(sku, base)`, ambas fuentes se
  conservan como trazabilidad, los cargos distintos se mantienen y una cantidad contradictoria se
  informa como conflicto sin elegir en silencio.
- **B — análisis del ASG (ya corregido antes, verificado).** La comprobación no depende del formateo:
  acepta alineación, tabulaciones y espacios irregulares; exige `asg_capacidad = 2`, la restricción
  `== 2` y los tres campos ligados a la variable. Se añadieron pruebas con formato real, con
  tabulaciones, con capacidad 1 y con `max_size` desligado.
- **C — evidencia de drills (precisión corregida).** El validador comprueba trazabilidad (archivo,
  presencia de la prueba y ejecución en el workflow de CI mediante pasos con `-run` focalizado); **no**
  ejecuta las pruebas ni sustituye el log de GitHub Actions. Se añadieron dos pasos de CI con los
  nombres exactos y la documentación se ajustó para no atribuir garantías que el validador no da.

Evidencia: 181 pruebas en `tests/infra/operacion` (104 de I-120 + 77 de I-140); `AWS_NOT_RUN`,
`DRILLS_AWS_NOT_RUN`, `AWS_COST_REAL_NOT_VERIFIED`.
