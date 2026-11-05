# A-080 — Plan técnico de arquitectura

**Estado:** DRAFT H04. **Entrada obligatoria:** `spec.md` y fuentes de A-080. **Salida:** decisión lista para implementar después de H07; este plan NO ejecuta infraestructura ni código.

## 1. Estrategia de construcción

1. **Rol Redis.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Redis real obligatorio para caché distribuida y coordinación de límites; DynamoDB siempre fuente de verdad para sesiones, finanzas, estado idempotente. No guardar únicamente refresh o dinero en Redis.
2. **Keys y seguridad.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Key `nexum:v1:<env>:<tenant-user-hash>:<resource>:<params-hash>:<data-version>` con HMAC userId y no PII explícita; configurar TLS y credenciales en H05; TTL y eviction controlados, sin keys globales para resultados privados.
3. **Cachés elegibles.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Catálogos públicos 1h con invalidación por versión; dashboard 20-60s y analytics 1-5 min como valores iniciales de diseño sujetables a perf. Nunca cachear tokens, respuestas a errores auth ni información de otra cuenta; emitir no-store al cliente en sensibles.
4. **Invalidación.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Escrituras financieros invalidan o versionan dashboard, financialSummary, analytics, budgetsUsage, streak del user; invalidación después de commit durable; si Redis falla el dato sigue correcto por consulta a Dynamo.
5. **Rate limiting.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Buckets atomic en Redis por endpoint-clase + identidad hash/IP confiable; valores iniciales login 5/5min por cuenta y 20/min por IP, no son promesa de protección hasta load test; tratar fail-open/fail-closed específico: login fail-closed con 503, GET normales degradables.
6. **Paginación cursor.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Cursor incluye filtros/time-range/userId/order/LastEvaluatedKey firmados con expiración; rechazar cursor de usuario ajeno o de filtros distintos; max 100, default 25; no offset por barrido completo.
7. **Consistencia de agregados.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Post-mutation read-your-writes desde Dynamo en eventos críticos; no servir caché vieja después de transacción. ETag/version por usuario y proyección invalidada.
8. **Medición.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Medir hit ratio, misses, P95/P99 y claves por usuario, cardinalidad, Redis failure fallback; no optimización sin baseline; evitar crear caché de objetos enormes.

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

- Plan de Redis en AWS cuenta Free requiere auditoría costo real H05. **Mitigación:** resolver antes de aprobar su dependencia; si requiere cambio de constitución emitir RFC.
- Redis caída no debe volver a un rate limiter por instancia que permite evasión sin control. **Mitigación:** resolver antes de aprobar su dependencia; si requiere cambio de constitución emitir RFC.

## 5. Criterio de cierre

Se considera listo para revisión cuando los 8 ejes técnicos de `spec.md` tienen trazabilidad a pruebas en `checklist.md`, los elementos futuros permanecen `NOT_RUN`, la matriz de alcance está intacta y las decisiones pendientes tienen dueño/fase, sin `APPROVED` automático.
