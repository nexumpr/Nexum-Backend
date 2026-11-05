# A-040 — Contratos REST/OpenAPI y versionamiento

| Campo | Valor |
|---|---|
| Fase | H04 — Arquitectura general (lote consolidado) |
| Versión | 0.2.0 |
| Estado | **DRAFT — Gate H04 pendiente** |
| Trazabilidad | E001-E075, CTX-001..014 |
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

### 1. Contrato formal

OpenAPI 3.1.0 para cada operación, schema request/response/error, ejemplos y semántica; catálogo fuente 75 (33 M, 23 S, 12 E, 7 O) preservado con ID Nexum-E001..075. Especificaciones por endpoint se redactarán en H06, no aquí.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 2. Campos y convenciones

camelCase JSON, timestamps RFC3339 UTC, money cadena decimal dos decimales MXN y cálculo en int64 centavos, date-only YYYY-MM-DD, month YYYY-MM, IANA timezone. Rechazar float monetario y campos unknown en requests.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 3. Paginación

cursor opaco firmado/validado con userId, filtros, orden y expiración; limit default 25 max 100 (propuesta H04); orden estable por occurredAt+ID. Nunca Scan global ni offset para tablas masivas. En H06 ajustar el page/limit histórico del PDF con compatibilidad negociada.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 4. Errores

Envelope {"error":{"code":"...","message":"...","details":[],"traceId":"..."}}; HTTP 400 request, 401 identidad, 403 política, 404 recurso ajeno/inexistente, 409 conflicto o idempotencia, 422 regla negocio, 429 cuota, 500 inesperado, 503 dependencia.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 5. HTTP methods

POST crea o ejecuta acción; PUT reemplaza; PATCH modificación parcial con versión; DELETE semántica idempotente según recurso; GET libre de efectos. 201 Location y 204 sin cuerpo. Idempotency-Key obligatorio en escritura financiera sensible.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 6. Seguridad

Bearer JWT salvo salud, autenticación y catálogos/avisos públicos expresamente definidos; errores no filtran enumeración. Tag OpenAPI security distinto por endpoint.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 7. Compatibilidad

/api/v1 se mantiene; cambio incompatible exige /api/v2, ADR, estrategia de deprecación y pruebas consumidor en HTTP real. No depende de aprobación Mobile: comprobación propia de esquema y mocks + HTTP.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 8. Contract-tests

Validar ejemplos OpenAPI con JSON Schema, códigos exactos y header auth; generated validation no basta: smoke HTTP de un backend real en H08 por endpoint liberado.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

## 3. Límites del alcance

- Solamente Backend, datos, seguridad, contratos API e infraestructura gestionada por el equipo Backend; ninguna implementación de Android, landing, Play Store o aprobación del equipo móvil.
- H04 congela **arquitectura lógica**; H05 congela viabilidad y topología física; H06 congela OpenAPI detallado de cada operación; H08 implementa conforme a esas decisiones.
- No convertir opciones propuestas de un catálogo histórico en compromisos silenciosos; no hacer optativas las obligaciones docentes.

## 4. Riesgos específicos y decisiones que deberán sustentarse

- **R-A-040-01:** PDF del catálogo usa Mongo y Google históricamente; no arrastrar esas decisiones a schemas.
- **R-A-040-02:** 75 rutas no son un mandato de publicar las 7 opcionales sin justificación; H06 resolverá cobertura docente y excepciones.

## 5. Condiciones de aceptación demostrables

- **AC-A-040-01:** Matrix 75/75 sin IDs duplicados.
- **AC-A-040-02:** Errores 401/403/404 no revelan recursos ajenos.
- **AC-A-040-03:** Headers idempotencia, versionado y paginación documentados.

## 6. Trazabilidad y desbloqueo

- H04: aprobar este diseño en conjunto con otros once, sin ocultar discrepancias de fuentes.
- H05: seleccionar recursos AWS permitidos, demostrar costo cero y capacidad para DOS instancias simultáneas + LB/HTTPS + Redis sin simulaciones.
- H06: ratificar las 75 rutas con contratos y categorías M/S/E/O y su cobertura contra pantallas; incluir las operaciones académicas.
- H07: comprobar contradicciones, seguridad, aceptación y que nada indispensable se pospone a implementación por falta de diseño.
- H08: cada caso se implementa y prueba de forma real, aislada, con autorización de release.
