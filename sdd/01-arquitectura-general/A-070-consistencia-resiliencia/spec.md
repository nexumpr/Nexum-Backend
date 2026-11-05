# A-070 — Dinero exacto, idempotencia, reglas y resiliencia

| Campo | Valor |
|---|---|
| Fase | H04 — Arquitectura general (lote consolidado) |
| Versión | 0.2.0 |
| Estado | **DRAFT — Gate H04 pendiente** |
| Trazabilidad | E024-E056, GOV-10 |
| Proveedor vigente | AWS; DynamoDB NoSQL; Redis real; Backend Go |
| Fuente original | ZIP Android V6.0.2 `(3)`; PDF 75 rutas `(2)`; PDF docente v3 |

## 1. Objetivo y condiciones de aceptación

Esta especificación establece los **requisitos de diseño vinculantes propuestos**, no código implementado. Se rechazan decisiones sin trazabilidad, seguridad o viabilidad. Si contradice `constitution.md`, el Gate se bloquea y se tramita RFC. No crea recursos AWS, endpoints HTTP reales ni modifica cliente móvil.

## Base documental y autoridad

1. Autoridad superior: `constitution.md` v0.2.0, `AGENTS.md`, H03 humano APPROVED (2026-11-05). Ningún texto de esta entrega cambia permisos ni política.
2. Fuente académica: **Requerimientos para el desarrollo del proyecto Móvil v3(2).pdf**, cuatro páginas; requisitos de Backend y dependencias transversales. Se consultó íntegramente en esta entrega.
3. Fuente funcional: **Nexum_V6_0_2_AndroidStudio_Demo_Eliminar_Cuenta(3).zip**, 16 entradas ZIP. Se leyeron `app.js`, `index.html`, `MainActivity.java`, `AndroidManifest.xml`, `README.md` y se comprobó ausencia de `fetch(` en `app.js`. Código cliente no constituye Backend existente.
4. Catálogo: **Nexum_V6_0_2_Catalogo_Completo_Endpoints_Backend(2).pdf**, 27 páginas; 75 identificadores/rutas propuestas E001..E075. Etiquetas M/S/E/O preservadas. **No adoptar** su stack histórico Google Cloud/MongoDB; el stack vigente del proyecto es **AWS + DynamoDB + Redis**.
5. Evidencia H03: 54 decisiones del motor real (53 DENY, 1 ALLOW) y aprobación humana del Gate H03; ejecución en Windows aportada en Warp. En este lote NO se llama a OpenCode y sus permisos siguen restringidos.

**Trazabilidad:** la etiqueta M/S/E/O describe el origen de la ruta, no constituye por sí sola decisión de implementación ni reemplaza lo exigido por el docente. La formalización OpenAPI por ruta y su aprobación corresponde a H06. Las decisiones físicas de despliegue y costo cero corresponden a H05 y no están presumidas.


## 2. Contrato arquitectónico

### 1. Dinero

Valor objeto Money{cents:int64,currency:MXN}; validar decimal JSON `^-?(0|[1-9][0-9]*)\.[0-9]{2}$` con rango permitido y no usar float64. Inputs negativos rechazados salvo operación explícita de reversión.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 2. Equivalencia mensual

Estimación factor once=0, daily=365/12, weekly=52/12, fortnightly=2, monthly=1, bimonthly=1/2, quarterly=1/3, semiannual=1/6, annual=1/12, en fracciones racionales; redondeo HALF_UP a centavo en frontera bien documentada. No confundir estimación con calendario de vencimientos.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 3. Disponible V6

incomeEquivalent - recurringFixedEstimate - oneTimeFixed - antExpenses - goalReservations (todos del mes). Fixed recurrente pagado registra movement pero no se descuenta otra vez. Variable recurrente no se descuenta como obligación hasta pago (gasto ant). No afirmar saldo bancario.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 4. Metas

initialSaved no crea movement de mes; target>0, 0<=openingSaved<=target; saved=opening+aportaciones no revertidas. Reducción de target bajo saved requiere rechazo 422. Meta completada no desaparece del listado.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 5. Split igual y personalizado

Usar centavos exactos; dividir equitativo con orden estable por goalId y redistribución sobrante si una meta se satura, sin exceder target; personalizado suma <= total y cada allocation <= remaining; devolver `undistributed` sin asignarlo ficticiamente.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 6. Reversión

Eliminar movement saving revierte únicamente allocation vinculada; reverse group reversa todas exactamente una vez; restablece sumarios, presupuestos y streak. Mantener evento auditado; no perder historia al cancelar recurring.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 7. Idempotencia

Idempotency-Key por userId+operation, hash canonical de payload, TTL según retención, check misma key distinto payload 409, mismo payload replica misma respuesta; guardar resultado atómicamente con efectos. Protege double-tap, retries y timeouts.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 8. Resiliencia

Timeout aguas abajo, backoff jitter exponencial y límite, errores clasificados por transitoriedad, circuit breakers solo si medidos. `readiness` 503 ante incapacidad de escribir; degrade cache opcional pero Redis real requerido despliegue.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

## 3. Límites del alcance

- Solamente Backend, datos, seguridad, contratos API e infraestructura gestionada por el equipo Backend; ninguna implementación de Android, landing, Play Store o aprobación del equipo móvil.
- H04 congela **arquitectura lógica**; H05 congela viabilidad y topología física; H06 congela OpenAPI detallado de cada operación; H08 implementa conforme a esas decisiones.
- No convertir opciones propuestas de un catálogo histórico en compromisos silenciosos; no hacer optativas las obligaciones docentes.

## 4. Riesgos específicos y decisiones que deberán sustentarse

- **R-A-070-01:** Cálculo monetario en JS de prototipo float no debe copiarse a Go.
- **R-A-070-02:** Aportaciones split con > máximo transaccional deben rechazarse de modo determinista antes de cambiar datos.

## 5. Condiciones de aceptación demostrables

- **AC-A-070-01:** Casos centavos 0.01, 0.02/3, saturación meta, retries 100 veces.
- **AC-A-070-02:** Pago recurring fixed no disminuye doble.
- **AC-A-070-03:** Reversión preserva invariante `saved=opening+allocations activas`.

## 6. Trazabilidad y desbloqueo

- H04: aprobar este diseño en conjunto con otros once, sin ocultar discrepancias de fuentes.
- H05: seleccionar recursos AWS permitidos, demostrar costo cero y capacidad para DOS instancias simultáneas + LB/HTTPS + Redis sin simulaciones.
- H06: ratificar las 75 rutas con contratos y categorías M/S/E/O y su cobertura contra pantallas; incluir las operaciones académicas.
- H07: comprobar contradicciones, seguridad, aceptación y que nada indispensable se pospone a implementación por falta de diseño.
- H08: cada caso se implementa y prueba de forma real, aislada, con autorización de release.

## 7. Matriz de reglas críticas heredadas de la maqueta

| Regla | Fórmula / condición invariante |
|---|---|
| Disponible mensual | incomeEq - recurringFixedEstimate - oneTimeFixed - ant - savingReserved |
| Recurring fixed pagado | movimiento histórico sí; doble descuento mensual no |
| Variable recurrente | no resta proyección, gasto real sí cuenta como hormiga |
| Saldo inicial de meta | apertura no es aporte mensual |
| Reparto igual | centavos, prioridades estables, redistribución al saturar |
| Reparto custom | sum(assignments)<=total y capacidad por meta |
| Reversión movimiento saving | decrementa solo su meta/allocation una vez |
| Borrado regla recurrente | conserva movimientos anteriores |
| Racha | día local IANA, contribuciones >=1/día, recálculo en reversión |
| Demo | datos ficticios y borrado local, jamás identidad real |
