# Gate H08-I / I-120 — Encendido, apagado y preservación de datos (LOCAL)

**FRASE HUMANA REGISTRADA:** `APRUEBO_GATE_I120_LOCAL` (2026-11-09, responsable del proyecto).

**ESTADO:** `AUTHORIZED_I120_LOCAL` — implementación **exclusivamente local**. No se autoriza
`terraform plan/apply/destroy`, ninguna mutación AWS, ningún despliegue ni DAST.

## 1. Alcance autorizado

Implementar y probar en local, sobre la rama `feat/h08i-i120-operacion`, la spec
`sdd/02-infraestructura/I-120-encendido-apagado`:

- guardas declarativas Terraform (`infra/terraform/modules/operacion`) que fallan cerrado;
- inventario declarativo único (`infra/terraform/operacion/inventario.json`);
- operador determinista `herramientas/nexum_operacion.py` con los flujos `plan`, `estado`,
  `encender`, `suspender-aux`, `reanudar-aux`, `reconciliar`, `rollback` y `destruir-aux`;
- documentos `MAQUINA_ESTADOS_RUNTIME.md`, `RUNBOOK_START_STOP.md`, `INVENTARIO_RESIDUAL.md`;
- pruebas de contrato, adversas y de alineación FinOps;
- commit, push del rama, PR y seguimiento de CI. **Merge requiere autorización humana separada.**

## 2. NO autorizado

- `terraform plan`, `apply`, `destroy`, `import`, `state` y cualquier backend remoto activo.
- Cualquier llamada AWS mutante; consultas de solo lectura tampoco se ejecutan en esta corrida.
- Cost Explorer y facturación (`NOT_VERIFIED`).
- Crear/modificar/eliminar recursos, presupuestos, alarmas, SNS, KMS o certificados.
- Cambiar el plan Free, IAM, SCP, OIDC o entornos protegidos.
- Despliegue, DAST, publicación del servicio o declaración de `AWS_VERIFIED`.
- Modificar `spec.md`/`plan.md` de I-120 (política de edición) o cualquier otra spec.
- I-130 (backups) y I-140/I-150: siguen `NOT_AUTHORIZED`.

## 3. Decisiones de diseño aplicadas

1. **Core intocable 24×7** (spec §9): 2 EC2 Go, ALB, CloudFront, Redis OSS, DynamoDB, state y logs
   permanentes no se suspenden nunca. `OFF` aplica solo a auxiliares.
2. **Allowlist explícita y separada del inventario**: añadir un auxiliar obliga a revisar
   `envs/h08i/operacion.tf`; no basta ampliar el JSON.
3. **Deny por defecto**: sin etiqueta `nexum:criticality` válida, criticidad incoherente, core
   marcado como suspendible o auxiliar fuera de allowlist ⇒ el plan **falla** (precondition) y el
   operador devuelve código 2.
4. **Destroys separados**: `destroy-data`, `destroy-state` y `destroy-all` no existen como
   capacidades. Son variables con validación permanente en `false` y comandos rechazados.
5. **Doble instancia**: `ON_2_HEALTHY` exige `targets_sanos >= 2`; con 1 nodo el plan queda
   `STARTING` y el servicio no es publicable (`GOV-02`).
6. **Dinero exacto**: `Decimal`, mes de facturación de 730 h y residual separado en residente,
   presupuestado, no computable y variable. Nunca `float`, nunca costo cero.
7. **Sin AWS en el operador**: no importa SDK de AWS ni ejecuta procesos; todo plan declara
   `aws_consultado=false` y `terraform_ejecutado=false`.

## 4. Entregables verificables

| Artefacto | Contenido |
|---|---|
| `infra/terraform/modules/operacion/` | `main.tf` con `terraform_data` y 5 preconditions, `variables.tf` con validaciones permanentes, `outputs.tf` con política |
| `infra/terraform/envs/h08i/operacion.tf` | lectura del inventario + módulo + allowlist explícita + output |
| `infra/terraform/operacion/inventario.json` | 16 recursos clasificados, umbrales y política |
| `herramientas/nexum_operacion.py` | 8 comandos, 3 códigos de salida, aprobaciones exactas |
| `sdd/_control/H08/MAQUINA_ESTADOS_RUNTIME.md` | 6 estados, tabla de transiciones, guardas |
| `sdd/_control/H08/RUNBOOK_START_STOP.md` | procedimientos, prohibiciones, incidentes |
| `sdd/_control/H08/INVENTARIO_RESIDUAL.md` | residual 0.0902 USD/h y 97.4412 USD/45 d, con exclusiones explícitas |
| `tests/infra/operacion/` | 54 pruebas adversas |
| `tests/infra/terraform/test_operacion_contrato.py` | 27 pruebas de contrato estático |
| `tests/infra/finops/test_i120_alineacion.py` | 7 pruebas de alineación FinOps |

## 5. Verificación exigida y evidencia

| Comprobación | Resultado |
|---|---|
| `terraform fmt -check -recursive infra/terraform` | PASS |
| `terraform init -backend=false` + `validate` (envs/h08i) | PASS |
| Suite `tests/infra/terraform` | PASS |
| Suite `tests/infra/operacion` | 54 OK |
| Suite `tests/infra/finops` | PASS (incluye 7 de I-120) |
| Trivy HIGH/CRITICAL sobre `infra/terraform` | PASS (sin excepciones nuevas) |
| Validador SDD integral + SHA-256 | PASS |
| Go `build`/`vet`/`test` | Sin cambios de código Go en esta spec |
| Ejecución real en AWS | **`NOT_RUN`** |
| Costo observado | **`NOT_VERIFIED`** |

## 6. Bloqueos y dependencias externas

- **I-130 `NOT_AUTHORIZED`**: backups programados de DynamoDB; el residual los marca
  `PRESUPUESTADO_NO_CREADO` y `restore` no se simula.
- **RFC-004** (`PROPUESTO`): el secreto de Redis no se declara creado (`BLOQUEADO_RFC004`).
- **H-01**: TLSv1 por certificado por defecto de CloudFront; sin cambios.
- **RFC-007**: notificación de alarmas CloudWatch sigue `BLOCKED_RFC007_SNS_KMS`.
- **`AWS_SCP_NOT_VERIFIED`**, **OIDC/entorno de despliegue `NOT_VERIFIED`**, **readiness 503** por
  diseño, **D-100-01**, **D-100-02**, **D-110-01** abiertos.
- **`LOG_PIPELINE_AWS_VERIFIED = NOT_RUN`**: falta agente CloudWatch (I-020).

## 7. Criterio de cierre local

I-120 podrá declararse `IMPLEMENTADO_LOCAL — NO DESPLEGADO` cuando: las guardas existan y fallen
cerrado, el operador rechace toda operación sobre el core y toda capacidad de destroy de datos/estado,
el residual sea exacto y no se presente como costo cero, Terraform valide, Trivy pase sin ocultar
hallazgos, las suites locales pasen y el SDD/hashes permanezcan íntegros.

**No se declara** `AWS_DEPLOYED`, `RUNTIME_VERIFIED`, `COSTO_MEDIDO` ni `FINOPS_GARANTIZADO`.

## 8. Registro

| Fecha | Hecho |
|---|---|
| 2026-11-09 | `APRUEBO_GATE_I120_LOCAL` concedido; rama `feat/h08i-i120-operacion` creada desde `main` `071478a1` (incluye cierre documental de I-110) |
| 2026-11-09 | Implementación local completada; verificación y PR por agente; merge pendiente de autorización humana ligada a PR + HEAD |
| 2026-11-09 | Ampliación v2: registro completo de transiciones, alcance de plan (rechazo de destrucción indirecta), `apply` deshabilitado, reintentos y fallos parciales, impacto `BLOQUEANTE`, control FinOps diario con 4 umbrales y forecast, `MATRIZ_INF_120.md` y excepción gitleaks acotada a una constante de estado. Merge sigue pendiente de autorización humana ligada a PR + HEAD |

## 9. Ampliación v2 (2026-11-09)

- **INF-120-01:** registro con 12 campos (estado anterior/nuevo, actor, timestamp UTC, entorno,
  recurso, motivo, operación, autorización, resultado, evidencia, rollback); `OFF` marcado como
  estado auxiliar y `off_sobre_core` tratado como incidente.
- **INF-120-02:** `reanudar-aux` exige créditos **y** forecast; sin ellos queda `BLOQUEADO`.
- **INF-120-03:** rechazo por impacto `BLOQUEANTE` declarado en el inventario; `VENTANA_DECLARADA`
  (D6) es el único impacto admitido para los 3 endpoints de mantenimiento.
- **INF-120-04:** cualquier cambio de plan sobre ALB/CloudFront/core se rechaza; capacidad < 2 se
  rechaza en Terraform y en el operador.
- **INF-120-05:** `apply` devuelve `EJECUCION_DESHABILITADA` (código 2); `--force`/`--yes`
  rechazados; `cargar_autorizacion` valida el registro revisado y declara
  `habilitacion_ejecucion = DENEGADA_EN_I120_LOCAL`.
- **INF-120-06:** 20 categorías cubiertas, incluidas reintentos, fallos parciales, concurrencia y
  manipulación de planes.
- **FinOps:** `herramientas/verificar_finops_diario.py` con umbrales 45/35/30/20, regla de forecast
  y fallo cerrado (`BLOQUEADO_SIN_DATOS`).
- **gitleaks:** excepción acotada dentro de `generic-api-key` (ruta `tests/infra/operacion/**` y
  valor `^BLOQUEADO_[A-Z0-9]+$`), sin allowlist global ni reglas desactivadas.

Evidencia local v2: 83 pruebas de operación, 36 de contrato Terraform, 18 de FinOps diario,
`terraform fmt/init -backend=false/validate` PASS, Trivy PASS, SDD y SHA-256 PASS.
`AWS_NOT_RUN`, `TWO_NODES_AWS_NOT_VERIFIED`, `BLOCKED_I130`.

## 10. Corrección de cierre (2026-11-09)

Defectos de fail-open corregidos y verificados con pruebas negativas:

- **Allowlist fail-open:** `clasificar` aceptaba auxiliares con `permitidos` vacío y la CLI
  derivaba la allowlist del inventario cuando se omitía `--allowlist`. Ahora `None` y `[]` no
  autorizan nada, la allowlist es obligatoria en operaciones que mutan, y se rechaza cualquier
  allowlist con core, nombre inexistente o etiqueta distinta de `auxiliary`. La lectura sin
  allowlist sigue mostrando el inventario.
- **Alcance fail-open:** `exigir_alcance_plan` solo miraba `denegados` y aceptaba
  `verificado=false`. Ahora un plan ausente, vacío con cambios solicitados o malformado se
  rechaza; solo la repetición idempotente admite plan vacío. El resultado se etiqueta
  `PLAN_LOCAL_VALIDADO` y el alcance AWS permanece `PLAN_AWS_NO_VERIFICADO` /
  `TERRAFORM_REAL_PLAN_NOT_VERIFIED`.
- **FinOps fail-open:** las banderas `verificado_*` no bloqueaban decisiones favorables. Ahora
  son requisito, el plan debe ser `FREE`, se rechazan negativos/`NaN`/infinitos, se detectan
  créditos caducados, el saldo restante explícito evita descontar dos veces el consumo y toda
  salida declara `autoriza_ejecucion_real = false`.
- **gitleaks:** la excepción se reduce a un único archivo y un único valor
  (`test_nexum_operacion.py` + `^BLOQUEADO_RFC004$`), dentro de `generic-api-key`, con
  `condition = "AND"`; se restauró la aserción natural de la prueba y se comprueba que otros
  valores y otras rutas siguen detectándose.

Evidencia: 104 pruebas de operación, 238 de FinOps, 36 de contrato Terraform, 11 de configuración
gitleaks, `terraform fmt/init -backend=false/validate`, Trivy, SDD y SHA-256. `AWS_NOT_RUN`,
`PLAN_AWS_NO_VERIFICADO`, `BLOCKED_I130`.
## 11. Registro de integración — 2026-11-09

- `APRUEBO_MERGE_PR_32_SHA_03b10cdb1a7c982f2077915cb324c9f880865da0` concedido por el responsable;
  HEAD verificado por igualdad exacta y `--match-head-commit` antes de fusionar; 7/7 checks `pass` y
  `MERGEABLE`/`CLEAN`.
- **PR #32 `MERGED`** (`2026-11-09T03:21:49Z`); merge commit y `main` = **`31d25127033cd63a4a74040caea4137f52732536`**;
  `git pull --ff-only` dejó `main` local idéntico a `origin/main`.
- CI post-merge sobre `main` en `31d2512`: **SDD documental `success`** (run `42555990713`) y
  **Go Backend CI `success`** (run `42555990909`, seis jobs en verde).
- Estado: **INTEGRADO_MAIN — NO DESPLEGADO**. `AWS_NOT_RUN`, `PLAN_AWS_NO_VERIFICADO`,
  `TWO_NODES_AWS_NOT_VERIFIED`, `BLOCKED_I130`, `DAST_NOT_RUN`.
- Defectos y RFC sin cambios: `D-100-01`, `D-100-02`, `D-110-01`, RFC-004, H-01, RFC-006, RFC-007,
  `ARTIFACT_DELIVERY`, agente CloudWatch (I-020), `ALERT_DELIVERY = BLOCKED_RFC007_SNS_KMS`,
  readiness 503, OIDC, `AWS_SCP_NOT_VERIFIED`, branch protection `BLOQUEADO_PLAN_GITHUB`.
- Este registro es documental: no se modificó Terraform, Go, OpenAPI ni workflows tras el merge.
