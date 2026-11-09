# I-120-encendido-apagado — Encendido, apagado y preservación de datos

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | I-000, I-020, I-030, I-040, I-050 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## Gestión de tareas H05 (solo especificación y aceptación de diseño)

Estado inicial de todos los trabajos: `TODO`, salvo evidencia local de integridad H04 documentada por el responsable. No marcar como completado por generar archivos.

### T-120-01 — Cerrar estados

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y I-000, I-020, I-030, I-040, I-050.
- **Trabajo:** `OFF` runtime efímero deshabilitado; `STARTING`, `ON_2_HEALTHY` (único estado operativo validable), `DRAINING`, `ERROR`, `COST_REVIEW`. Toda transición requiere actor, timestamp y runbook; datos persisten.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-120-02 — Cerrar encender

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y I-000, I-020, I-030, I-040, I-050.
- **Trabajo:** Aprobación humana, check plan/credits, desiredCapacity=2 y healthcheck de targets y Redis; no exponer URL hasta 2 healthy; pruebas HTTP y auditoría de costo inicial.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-120-03 — Cerrar apagar

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y I-000, I-020, I-030, I-040, I-050.
- **Trabajo:** Drenar requests, bloquear trabajos, recoger logs, bajar o destruir runtime efímero autorizado; detener EC2 no equivale a eliminar ALB/IPv4/Redis/EBS/billing. Persistir Dynamo y Terraform state; registrar recursos residuales con precios.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-120-04 — Cerrar dns/alb

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y I-000, I-020, I-030, I-040, I-050.
- **Trabajo:** Evaluar si ALB puede permanecer y cuánto factura; modo más económico puede requerir destruir ALB y reprovisionar posteriormente (cambio DNS y TLS), previa autorización. No eliminar tablas por defecto.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-120-05 — Cerrar seguridad

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y I-000, I-020, I-030, I-040, I-050.
- **Trabajo:** Nunca utilizar destroy-all en un script de apagado. Separar explícitamente `destroy-runtime` de `destroy-data` y `destroy-state` con protección fuerte.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-120-06 — Cerrar prueba

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y I-000, I-020, I-030, I-040, I-050.
- **Trabajo:** Tras OFF, obtener Resource Inventory y Billing, verificar datos en Dynamo intactos y restaurar ON con 2 nodos; comparar costos reales.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-120-90 — Revisión de riesgos y dependencias

- **Estado:** TODO.
- **Trabajo:** resolver o escalar: Apagado incompleto mantiene ALB/Redis facturados, Destroy accidental de datos, Gastos mientras runtime OFF.
- **Criterio DoD:** cada riesgo clasificado RESOLVED/BLOCKED/ACCEPTED con firma/fecha; no usar ACCEPTED para incumplir requisito docente sin aclaración.

### T-120-99 — Ratificación de la especificación

- **Estado:** REVIEW pendiente.
- **Entrada:** `spec.md`, `plan.md`, `tasks.md`, `checklist.md` actualizados.
- **Salida:** decisión explícita del responsable y vínculo a evidencia en informe I-GO.
- **Bloqueo:** no crear Terraform/endpoints ni ejecutar AWS.

## Implementacion H08-I (2026-11-09) — LOCAL, sin AWS

Gate `APRUEBO_GATE_I120_LOCAL`. Rama `feat/h08i-i120-operacion`. Nada se despliega: `AWS_NOT_RUN`.

- **T-120-01 (Estados):** IMPLEMENTADO_LOCAL. Seis estados en `MAQUINA_ESTADOS_RUNTIME.md` y en el operador; transiciones con actor, runbook y timestamp; `OFF -> ON_2_HEALTHY` prohibida. Evidencia: `tests/infra/operacion` (54 OK).
- **T-120-02 (Encender):** IMPLEMENTADO_LOCAL. `encender` exige aprobacion exacta `APRUEBO_ENCENDER_<ENTORNO>`, actor y runbook; no alcanza `ON_2_HEALTHY` sin 2 targets sanos. Ejecucion real: NOT_RUN.
- **T-120-03 (Apagar):** IMPLEMENTADO_LOCAL. Core intocable 24x7 (spec seccion 9); solo auxiliares en allowlist. Residual enumerado en `INVENTARIO_RESIDUAL.md` (0.0902 USD/h; 97.4412 USD/45 d), con partidas variables y bloqueadas excluidas explicitamente.
- **T-120-04 (DNS/ALB):** IMPLEMENTADO_LOCAL como decision: ALB no se apaga (no existe API Stop). Sin cambios de DNS ni de I-030.
- **T-120-05 (Seguridad):** IMPLEMENTADO_LOCAL. `destroy-data`, `destroy-state` y `destroy-all` no existen: comandos rechazados (codigo 2) y variables con validacion permanente en `false`. Deny por defecto en inventario, modulo y operador.
- **T-120-06 (Prueba):** PENDIENTE_AWS. La prueba OFF/ON con inventario y facturacion reales requiere Gate de despliegue; hoy `AWS_NOT_RUN` y `AWS_COST_REAL = NOT_VERIFIED`.
- **T-120-90 (Riesgos):** R01 (apagado incompleto) mitigado por politica core + residual documentado; R02 (destroy de datos) mitigado por capacidad inexistente; R03 (gasto con runtime OFF) mitigado por residual enumerado. Los tres quedan CONDICIONADOS a verificacion real.
- **T-120-99 (Ratificacion):** REVIEW pendiente; merge requiere autorizacion humana ligada a PR + HEAD.

### Ampliacion v2 de la implementacion local (2026-11-09)

- **T-120-01:** registro con 12 campos; `off_sobre_core` como incidente. Pruebas: `RegistroDeTransiciones`.
- **T-120-02:** `reanudar-aux` exige creditos y forecast; 7 prerrequisitos declarados. Pruebas: `test_reanudar_bloquea_sin_pronostico`, `test_reanudar_ok_con_pronostico_cubierto`.
- **T-120-03:** `impacto_core` + allowlist + etiqueta + alcance de plan; `BLOQUEANTE` deniega. Pruebas: `ImpactoYAuxiliares`.
- **T-120-04:** rechazo de planes sobre core y de capacidad < 2 en Terraform y operador.
- **T-120-05:** `apply` deshabilitado, `--force`/`--yes` rechazados, autorizacion con procedencia revisada.
- **T-120-06:** 20 categorias de prueba, reintentos, fallos parciales y concurrencia.
- **FinOps diario:** herramienta y 18 pruebas con umbrales 45/35/30/20 y forecast.

### Cierre de fail-open (2026-11-09)

- **T-120-03:** allowlist fail-closed (None/vacia no autorizan; core, inexistentes y etiquetas no auxiliares rechazados). Pruebas: `AllowlistExplicita`.
- **T-120-04:** plan obligatorio; ausente/vacio/malformado rechazado; etiquetas `PLAN_LOCAL_VALIDADO` y `PLAN_AWS_NO_VERIFICADO`. Pruebas: `PlanObligatorio`.
- **T-120-06:** regresiones de FinOps fail-closed (13 pruebas) y de la excepcion gitleaks (3 pruebas).
