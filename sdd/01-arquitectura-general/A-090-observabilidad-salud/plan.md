# A-090 — Plan técnico de arquitectura

**Estado:** DRAFT H04. **Entrada obligatoria:** `spec.md` y fuentes de A-090. **Salida:** decisión lista para implementar después de H07; este plan NO ejecuta infraestructura ni código.

## 1. Estrategia de construcción

1. **Trazabilidad.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. OpenTelemetry trace/span contextual a cada HTTP y AWS SDK; propagate W3C traceparent y x-request-id, no usar id de sesión como trace id.
2. **Logs.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. log/slog JSON estructura timestamp, level, service, env, traceId, routeTemplate, durationMs, status; redacción PII/password/JWT/refresh, inputs financieros privados y transcript voz.
3. **Métricas.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. HTTP RPS, 4xx/5xx, P50/P95/P99, Dynamo latency/throttle/RCU/WCU, Redis availability/hit ratio, auth rejects, token replay, transaction conflict, queue/deletion backlog y counters negocio no sensibles.
4. **Health.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. GET /api/v1/health/live solo indica proceso vivo y sin datos secretos; GET /api/v1/health/ready verifica dependencias esenciales con timeout acotado y puede devolver 503. LB healthcheck planificado en H05.
5. **Alertas.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Alertas por 5xx, 429, errores persistentes Dynamo, fallos Redis, aumento latencia, expiración TLS, fallos del pipeline y presupuesto. Notificación destino definido en H05 sin fingir servicios activados.
6. **Retención.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Logs sin PII innecesaria, retención configurable para cumplir privacidad/costo; trazas muestreadas y auditoría sensible separada con acceso mínimo; no imprimir tokens en CI.
7. **Pruebas operativas.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Degradar Redis y Dynamo en pruebas controladas para verificar ready y fallback; simular SIGTERM en ambas réplicas; comprobar correlación HTTP-DB.
8. **Objetivos iniciales.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Definir SLI disponibilidad, éxito de escritura durable y p95 latencia; umbrales SLO se fijan con carga real antes producción, no inventar 99.9% sin capacidad/contrato.

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

- Proveedores de observabilidad AWS y costos no seleccionados hasta H05. **Mitigación:** resolver antes de aprobar su dependencia; si requiere cambio de constitución emitir RFC.
- Métricas con userId claro pueden violar privacidad o disparar cardinalidad. **Mitigación:** resolver antes de aprobar su dependencia; si requiere cambio de constitución emitir RFC.

## 5. Criterio de cierre

Se considera listo para revisión cuando los 8 ejes técnicos de `spec.md` tienen trazabilidad a pruebas en `checklist.md`, los elementos futuros permanecen `NOT_RUN`, la matriz de alcance está intacta y las decisiones pendientes tienen dueño/fase, sin `APPROVED` automático.
