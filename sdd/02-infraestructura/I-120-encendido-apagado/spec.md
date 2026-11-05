# I-120-encendido-apagado — Encendido, apagado y preservación de datos

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | I-000, I-020, I-030, I-040, I-050 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## 1. Objetivo y definición de terminado

Permitir ventanas de prueba con dos instancias simultáneas y runtime desconectado fuera de uso, sin falsificar cero costo ni borrar los datos de DynamoDB.

## Autoridad y restricciones

- `constitution.md` (GOV-01..18, R-001..060) y el Gate H04 aprobado explícitamente el **2026-11-05** tienen precedencia; H05 diseña infraestructura, no la ejecuta.
- Arquitectura ratificada: Go (Clean Architecture), AWS, DynamoDB, **Redis real**, **2 instancias simultáneas**, balanceador HTTPS y CI/CD/DevSecOps. Ninguno es demostración opcional.
- El PDF del docente v3 exige Backend cloud con costo cero; el catálogo V6.0.2 enumera 75 rutas propuestas E001..E075 y utiliza stack histórico GCP/MongoDB **descartado**.
- No crear código Go, archivos Terraform `.tf`, pipelines ni endpoints H06 durante H05; `terraform apply/destroy`, `git push`, cambios AWS y facturación están prohibidos hasta fases y aprobaciones autorizadas.
- **Regla de evidencia:** documentos pueden alcanzar DRAFT completo sin acceso cloud; Gate I-GO requiere demostrar **viabilidad operacional y económica**, no asumirla. Campos pendientes no se cambian a PASS ni se solventan con créditos.

## 2. Contrato de infraestructura propuesto

### INF-120-01 — Estados

`OFF` runtime efímero deshabilitado; `STARTING`, `ON_2_HEALTHY` (único estado operativo validable), `DRAINING`, `ERROR`, `COST_REVIEW`. Toda transición requiere actor, timestamp y runbook; datos persisten.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-120-02 — Encender

Aprobación humana, check plan/credits, desiredCapacity=2 y healthcheck de targets y Redis; no exponer URL hasta 2 healthy; pruebas HTTP y auditoría de costo inicial.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-120-03 — Apagar

Drenar requests, bloquear trabajos, recoger logs, bajar o destruir runtime efímero autorizado; detener EC2 no equivale a eliminar ALB/IPv4/Redis/EBS/billing. Persistir Dynamo y Terraform state; registrar recursos residuales con precios.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-120-04 — DNS/ALB

Evaluar si ALB puede permanecer y cuánto factura; modo más económico puede requerir destruir ALB y reprovisionar posteriormente (cambio DNS y TLS), previa autorización. No eliminar tablas por defecto.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-120-05 — Seguridad

Nunca utilizar destroy-all en un script de apagado. Separar explícitamente `destroy-runtime` de `destroy-data` y `destroy-state` con protección fuerte.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-120-06 — Prueba

Tras OFF, obtener Resource Inventory y Billing, verificar datos en Dynamo intactos y restaurar ON con 2 nodos; comparar costos reales.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

## 3. Dependencias y secuencia

I-000, I-020, I-030, I-040, I-050, I-070, I-130

## 4. Criterios de aceptación de diseño (Gate H05)

- Control de estados on/off y acciones por recurso.
- Nunca pérdida de DynamoDB ni secretos durante apagado.
- No publicar servicio con 1 nodo fingiendo doble instancia.
- Costo residual enumerado tras OFF.
- Prueba restart/restauración desde OFF con datos existentes.

## 5. Riesgos que impiden una aprobación sin evidencia

- **Riesgo:** Apagado incompleto mantiene ALB/Redis facturados. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.
- **Riesgo:** Destroy accidental de datos. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.
- **Riesgo:** Gastos mientras runtime OFF. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.

## 6. Entregables y referencias

Los siguientes nombres identifican evidencias/artefactos **futuros** de I-GO y de implementación; no son archivos creados en H05: `MAQUINA_ESTADOS_RUNTIME.md`, `RUNBOOK_START_STOP.md`, `INVENTARIO_RESIDUAL.md`.

## 7. Criterio de bloqueo y aprobación

**BLOCKED** si falta una obligación técnica, contradice alguna spec A aprobada, no puede comprobarse la viabilidad Free/SCP o no hay aceptación humana del riesgo. **DRAFT** significa que el diseño está documentado; nunca equivale a infraestructura aplicada.

## 8. Cierre tecnico-financiero H05 (propuesta v2; aun no aprobada)

- A seleccionado para evaluación hasta ratificación: dos Go EC2 ON 1080 h; ALB/Redis 1080 h. Para B 8h/día: solo Go EC2 e IPv4 asociadas consumen 360 h; ALB, IPv4 ALB, Redis, EBS, Dynamo, state, logs, zone y secretos siguen activos 1080 h. No usar `stop-cache-cluster` inexistente ni afirmar que un ASG=0 ahorra el ALB.
- ON: verificar créditos, plan Free, alerta/IVA, cuota, dominio, Redis, y **2 targets healthy** antes de publicar; OFF: escalar Go a 0, retirar IPv4 EC2 automáticamente en instancia terminada, preservar ALB+Redis+datos; si desean cero residual, solicitar RFC separado de destruir runtime y recrear DNS/Redis con backups y certificados. No scripts de eliminación de datos.


> **PRECEDENCIA H05:** esta enmienda V3 es la propuesta vigente frente a cualquier alternativa incompatible de secciones V2 anteriores. No se considera aprobada hasta firma humana Gate H05.
## 9. Actualización v3: Core encendido 24×7 y apagado selectivo de NO esenciales

La instrucción del responsable obliga a mantener **API y DynamoDB disponibles continuamente**; para preservar funciones de login, autorización y rate limiting, mantener también **2 EC2 Go, CloudFront, ALB y Redis real encendidos 24×7** durante los 45 días. El estado `OFF` de INF-120-01/03 anterior aplica sólo a laboratorios/entornos **NO productivos de revisión**, nunca al core compartido; la alternativa B de 8h no está seleccionada. Nadie, incluido OpenCode, puede apagar un recurso core por su cuenta. El baseline de GitHub Actions runners efímeros deja de consumir al finalizar cada job y no requiere «dormir» otro servicio AWS.

Módulos **susceptibles de apagar/destruir y recrear por IaC con autorización separada** en H08+: sandboxes EC2 *extras* de pruebas si existieran, instancias temporales de benchmarking, entornos preview aislados, escáner DAST *efímero*, recursos de carga de prueba y stacks no core. Registrar inventario/tags, plan terraform por entorno, backup y confirmación previa. Nunca apagar/eliminar: DynamoDB y backups necesarios; las 2 instancias activas Go; ALB + origen CloudFront; Redis; estado Terraform; IAM mínimo; monitoreo preventivo. No existe API Stop de ALB/ElastiCache: cortar costo requiere ELIMINAR y reprovisionar, con pérdida de disponibilidad, por lo que **no procede para core**. No crear un schedule de apagado del core ni facilitar botón de destroy accidental en runbook.

Antes de cualquier despliegue H08: revisión humana de Free plan/saldo, monitorización diaria, evidencia de IAM y límites de gasto, umbrales internos de escalado a revisión cuando saldo <= 45, <=35, <=30 y cuando forecast del período restante exceda crédito restante. Los umbrales generan alertas/no cortes automáticos ni autorizan Paid. Si el gasto real hace inviable 45d, el equipo decide rediseño con antelación o suspende SOLO recursos no esenciales; la cuenta Free puede cerrarse si se agota crédito y dejar API y datos inaccesibles.


## 10. Contrato obligatorio IaC para OpenCode (enmienda H05-v4)

- **Prohibido operar infraestructura mediante clicks rutinarios.** OpenCode implementará en H08+ Terraform declarativo versionado y flujos de operación repetibles para `plan`, `apply`, `status`, `suspend-aux`, `resume-aux`, `reconcile`, `rollback` y `destroy-aux`; no se desarrollan archivos .tf en H05.
- Las autorizaciones humanas se realizan en gates/PR o ejecución de pipeline, **no** mediante manipulación manual de recursos en consola. Inspecciones read-only son admitidas como evidencia.
- Recursos `core`: CloudFront, ALB, **dos** instancias API Go simultáneas, Redis OSS, DynamoDB, identidad/red/seguridad imprescindibles y estado Terraform: protegidos contra suspender, detener, eliminar, escalar a menos de 2 o `destroy` por el comando auxiliar. Ninguna automatización de ahorro alterará ese conjunto.
- `suspend-aux` admite exclusivamente recursos explícitamente etiquetados `nexum:criticality=auxiliary` y dentro de un allowlist revisado; primero plan y estimado de impacto; ejecución con aprobación humana por ambiente. Recursos desconocidos o sin etiqueta => **deny**.
- `resume-aux`: terraform plan contra estado remoto preservado, verificación de prerrequisitos/cupo/créditos, despliegue idempotente y pruebas funcionales; no regenerar secretos ni sustituir datos persistentes.
- Los pipelines y permisos IAM tendrán roles separados `read-only`, `plan`, `apply-aux` y `apply-core`; los runners no tendrán permiso implícito de elevar plan Free a Paid. `iam:PassRole` será limitado.
- Terraform state/backups se preservarán de forma cifrada, con acceso limitado, locking y política de recuperación; no suponer que mantener S3/lock y artefactos es coste cero. DynamoDB jamás se destruirá por rutina de ahorro.
- Un chequeo de control diario deberá verificar `FREE/ACTIVE`, créditos y vencimiento, tendencia 45 días y 2 targets healthy. Umbrales de ejemplo: aviso < $45, bloqueo cambios no esenciales < $35, incidente crítico < $20; umbrales finales calculados contra gasto pendiente y tiempo restante. Las alertas **no son un hard cap** y no sustituyen una política de seguridad.
- Con créditos insuficientes para mantener 45 días + reserva, el proceso debe **bloquear nuevos despliegues auxiliares**, emitir evidencia y escalar al responsable. No puede prometer disponibilidad perpetua ni continuar después del cierre automático del plan Free.
- Fallas de autorización / servicios inaccesibles en Free se registrarán como bloqueo en implementación; está prohibido resolverlas con `upgrade` o servicios de pago. Se exigirá prueba automática de idempotencia, drift, rollback, recuperación y presupuesto antes del primer apply autorizado.

**Invariante:** el comando `suspend-aux` nunca toca ningún elemento del core; el autor humano puede aprobar releases sin dar clic manual en AWS.
