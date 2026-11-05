# A-070 — Plan técnico de arquitectura

**Estado:** DRAFT H04. **Entrada obligatoria:** `spec.md` y fuentes de A-070. **Salida:** decisión lista para implementar después de H07; este plan NO ejecuta infraestructura ni código.

## 1. Estrategia de construcción

1. **Dinero.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Valor objeto Money{cents:int64,currency:MXN}; validar decimal JSON `^-?(0|[1-9][0-9]*)\.[0-9]{2}$` con rango permitido y no usar float64. Inputs negativos rechazados salvo operación explícita de reversión.
2. **Equivalencia mensual.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Estimación factor once=0, daily=365/12, weekly=52/12, fortnightly=2, monthly=1, bimonthly=1/2, quarterly=1/3, semiannual=1/6, annual=1/12, en fracciones racionales; redondeo HALF_UP a centavo en frontera bien documentada. No confundir estimación con calendario de vencimientos.
3. **Disponible V6.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. incomeEquivalent - recurringFixedEstimate - oneTimeFixed - antExpenses - goalReservations (todos del mes). Fixed recurrente pagado registra movement pero no se descuenta otra vez. Variable recurrente no se descuenta como obligación hasta pago (gasto ant). No afirmar saldo bancario.
4. **Metas.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. initialSaved no crea movement de mes; target>0, 0<=openingSaved<=target; saved=opening+aportaciones no revertidas. Reducción de target bajo saved requiere rechazo 422. Meta completada no desaparece del listado.
5. **Split igual y personalizado.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Usar centavos exactos; dividir equitativo con orden estable por goalId y redistribución sobrante si una meta se satura, sin exceder target; personalizado suma <= total y cada allocation <= remaining; devolver `undistributed` sin asignarlo ficticiamente.
6. **Reversión.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Eliminar movement saving revierte únicamente allocation vinculada; reverse group reversa todas exactamente una vez; restablece sumarios, presupuestos y streak. Mantener evento auditado; no perder historia al cancelar recurring.
7. **Idempotencia.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Idempotency-Key por userId+operation, hash canonical de payload, TTL según retención, check misma key distinto payload 409, mismo payload replica misma respuesta; guardar resultado atómicamente con efectos. Protege double-tap, retries y timeouts.
8. **Resiliencia.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Timeout aguas abajo, backoff jitter exponencial y límite, errores clasificados por transitoriedad, circuit breakers solo si medidos. `readiness` 503 ante incapacidad de escribir; degrade cache opcional pero Redis real requerido despliegue.

## 2. Orden interno de diseño y dependencias

1. Revisar `constitution.md` y `AGENTS.md` antes de aceptar cambios; cotejar los PDF/prototipo cuyos nombres y hashes se registran en H04.
2. Identificar invariantes y entradas/salidas de cada módulo; mapear al catálogo E001..075 sin crear contratos endpoints H06.
3. Verificar acoplamiento con A-020 (capas), A-040 (DTOs), A-050 (auth), A-060 (persistencia), A-070 (consistencia), A-080 (Redis), A-090 (telemetría) y A-100 (QA).
4. Mantener como *blocker de H05* capacidad/costo de 2 instancias + balanceador + Redis, restricciones SCP/IAM, TLS y mecanismos de on/off; no inferir disponibilidad de AWS Free.
5. Registrar la decisión humana y conflictos si alguna regla altera el negocio del prototipo; no solicitar gate Mobile.

## 3. Estrategia de pruebas posteriores por capa

| Nivel | Diseño a comprobar | Evidencia futura obligatoria |
|---|---|---|
| Dominio | Invariantes y money/identidad | go test, race detector, entradas límite |
| Aplicación | Orquestación con mocks y transacciones | unit tests, casos negativos, retries |
| Infraestructura | DynamoDB/Redis reales según fase | integración, latencia, permisos mínimos, gasto |
| HTTP | Schemas OpenAPI 3.1 y respuestas | pruebas contractuales y HTTP reales |
| CI/CD | Seguridad y releases | PR checks, escaneo, autorización humana |

## 4. Riesgos y mitigación explícita

- Cálculo monetario en JS de prototipo float no debe copiarse a Go. **Mitigación:** resolver antes de aprobar su dependencia; si requiere cambio de constitución emitir RFC.
- Aportaciones split con > máximo transaccional deben rechazarse de modo determinista antes de cambiar datos. **Mitigación:** resolver antes de aprobar su dependencia; si requiere cambio de constitución emitir RFC.

## 5. Criterio de cierre

Se considera listo para revisión cuando los 8 ejes técnicos de `spec.md` tienen trazabilidad a pruebas en `checklist.md`, los elementos futuros permanecen `NOT_RUN`, la matriz de alcance está intacta y las decisiones pendientes tienen dueño/fase, sin `APPROVED` automático.
