# A-040 — Plan técnico de arquitectura

**Estado:** DRAFT H04. **Entrada obligatoria:** `spec.md` y fuentes de A-040. **Salida:** decisión lista para implementar después de H07; este plan NO ejecuta infraestructura ni código.

## 1. Estrategia de construcción

1. **Contrato formal.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. OpenAPI 3.1.0 para cada operación, schema request/response/error, ejemplos y semántica; catálogo fuente 75 (33 M, 23 S, 12 E, 7 O) preservado con ID Nexum-E001..075. Especificaciones por endpoint se redactarán en H06, no aquí.
2. **Campos y convenciones.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. camelCase JSON, timestamps RFC3339 UTC, money cadena decimal dos decimales MXN y cálculo en int64 centavos, date-only YYYY-MM-DD, month YYYY-MM, IANA timezone. Rechazar float monetario y campos unknown en requests.
3. **Paginación.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. cursor opaco firmado/validado con userId, filtros, orden y expiración; limit default 25 max 100 (propuesta H04); orden estable por occurredAt+ID. Nunca Scan global ni offset para tablas masivas. En H06 ajustar el page/limit histórico del PDF con compatibilidad negociada.
4. **Errores.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Envelope {"error":{"code":"...","message":"...","details":[],"traceId":"..."}}; HTTP 400 request, 401 identidad, 403 política, 404 recurso ajeno/inexistente, 409 conflicto o idempotencia, 422 regla negocio, 429 cuota, 500 inesperado, 503 dependencia.
5. **HTTP methods.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. POST crea o ejecuta acción; PUT reemplaza; PATCH modificación parcial con versión; DELETE semántica idempotente según recurso; GET libre de efectos. 201 Location y 204 sin cuerpo. Idempotency-Key obligatorio en escritura financiera sensible.
6. **Seguridad.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Bearer JWT salvo salud, autenticación y catálogos/avisos públicos expresamente definidos; errores no filtran enumeración. Tag OpenAPI security distinto por endpoint.
7. **Compatibilidad.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. /api/v1 se mantiene; cambio incompatible exige /api/v2, ADR, estrategia de deprecación y pruebas consumidor en HTTP real. No depende de aprobación Mobile: comprobación propia de esquema y mocks + HTTP.
8. **Contract-tests.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Validar ejemplos OpenAPI con JSON Schema, códigos exactos y header auth; generated validation no basta: smoke HTTP de un backend real en H08 por endpoint liberado.

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

- PDF del catálogo usa Mongo y Google históricamente; no arrastrar esas decisiones a schemas. **Mitigación:** resolver antes de aprobar su dependencia; si requiere cambio de constitución emitir RFC.
- 75 rutas no son un mandato de publicar las 7 opcionales sin justificación; H06 resolverá cobertura docente y excepciones. **Mitigación:** resolver antes de aprobar su dependencia; si requiere cambio de constitución emitir RFC.

## 5. Criterio de cierre

Se considera listo para revisión cuando los 8 ejes técnicos de `spec.md` tienen trazabilidad a pruebas en `checklist.md`, los elementos futuros permanecen `NOT_RUN`, la matriz de alcance está intacta y las decisiones pendientes tienen dueño/fase, sin `APPROVED` automático.
