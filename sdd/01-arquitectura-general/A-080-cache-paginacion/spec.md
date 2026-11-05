# A-080 — Redis compartido, caché, rate limiting y paginación

| Campo | Valor |
|---|---|
| Fase | H04 — Arquitectura general (lote consolidado) |
| Versión | 0.2.0 |
| Estado | **DRAFT — Gate H04 pendiente** |
| Trazabilidad | E021, E027, E032, E038, E051, E052 |
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

### 1. Rol Redis

Redis real obligatorio para caché distribuida y coordinación de límites; DynamoDB siempre fuente de verdad para sesiones, finanzas, estado idempotente. No guardar únicamente refresh o dinero en Redis.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 2. Keys y seguridad

Key `nexum:v1:<env>:<tenant-user-hash>:<resource>:<params-hash>:<data-version>` con HMAC userId y no PII explícita; configurar TLS y credenciales en H05; TTL y eviction controlados, sin keys globales para resultados privados.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 3. Cachés elegibles

Catálogos públicos 1h con invalidación por versión; dashboard 20-60s y analytics 1-5 min como valores iniciales de diseño sujetables a perf. Nunca cachear tokens, respuestas a errores auth ni información de otra cuenta; emitir no-store al cliente en sensibles.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 4. Invalidación

Escrituras financieros invalidan o versionan dashboard, financialSummary, analytics, budgetsUsage, streak del user; invalidación después de commit durable; si Redis falla el dato sigue correcto por consulta a Dynamo.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 5. Rate limiting

Buckets atomic en Redis por endpoint-clase + identidad hash/IP confiable; valores iniciales login 5/5min por cuenta y 20/min por IP, no son promesa de protección hasta load test; tratar fail-open/fail-closed específico: login fail-closed con 503, GET normales degradables.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 6. Paginación cursor

Cursor incluye filtros/time-range/userId/order/LastEvaluatedKey firmados con expiración; rechazar cursor de usuario ajeno o de filtros distintos; max 100, default 25; no offset por barrido completo.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 7. Consistencia de agregados

Post-mutation read-your-writes desde Dynamo en eventos críticos; no servir caché vieja después de transacción. ETag/version por usuario y proyección invalidada.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 8. Medición

Medir hit ratio, misses, P95/P99 y claves por usuario, cardinalidad, Redis failure fallback; no optimización sin baseline; evitar crear caché de objetos enormes.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

## 3. Límites del alcance

- Solamente Backend, datos, seguridad, contratos API e infraestructura gestionada por el equipo Backend; ninguna implementación de Android, landing, Play Store o aprobación del equipo móvil.
- H04 congela **arquitectura lógica**; H05 congela viabilidad y topología física; H06 congela OpenAPI detallado de cada operación; H08 implementa conforme a esas decisiones.
- No convertir opciones propuestas de un catálogo histórico en compromisos silenciosos; no hacer optativas las obligaciones docentes.

## 4. Riesgos específicos y decisiones que deberán sustentarse

- **R-A-080-01:** Plan de Redis en AWS cuenta Free requiere auditoría costo real H05.
- **R-A-080-02:** Redis caída no debe volver a un rate limiter por instancia que permite evasión sin control.

## 5. Condiciones de aceptación demostrables

- **AC-A-080-01:** Dos instancias comparten rate limit e invalidación.
- **AC-A-080-02:** Acceso entre usuarios no comparte respuesta cacheada.
- **AC-A-080-03:** 10000 movimientos paginados sin Scan ni offset.

## 6. Trazabilidad y desbloqueo

- H04: aprobar este diseño en conjunto con otros once, sin ocultar discrepancias de fuentes.
- H05: seleccionar recursos AWS permitidos, demostrar costo cero y capacidad para DOS instancias simultáneas + LB/HTTPS + Redis sin simulaciones.
- H06: ratificar las 75 rutas con contratos y categorías M/S/E/O y su cobertura contra pantallas; incluir las operaciones académicas.
- H07: comprobar contradicciones, seguridad, aceptación y que nada indispensable se pospone a implementación por falta de diseño.
- H08: cada caso se implementa y prueba de forma real, aislada, con autorización de release.
