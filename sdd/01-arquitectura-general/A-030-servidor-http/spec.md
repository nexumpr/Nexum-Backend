# A-030 — Servidor HTTP, middleware y políticas de transporte

| Campo | Valor |
|---|---|
| Fase | H04 — Arquitectura general (lote consolidado) |
| Versión | 0.2.0 |
| Estado | **DRAFT — Gate H04 pendiente** |
| Trazabilidad | E001-E075, GOV-05 |
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

### 1. Routing

/api/v1 con rutas explícitas y 404 no enumerable. Resolver PathValue y métodos; rechazar método no permitido con 405+Allow donde corresponda.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 2. Tiempo y recursos

Propuesta inicial (validar carga): readHeader 5s, read 10s, write 15s, idle 60s, respuesta backend 10s, request JSON normal <=1 MiB. Streaming/export se modela aparte. Propagar contexto y deadline hasta AWS SDK.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 3. Cadena middleware

Recovery -> requestId/tracing -> logging saneado -> real-IP desde proxy confiable -> size limit -> CORS/OPTIONS -> auth -> rate limit -> autorización recurso -> handler; no confiar X-Forwarded-* fuera del balanceador.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 4. Seguridad HTTP

TLS termina en LB AWS y opción mTLS interna si H05 lo determina; HTTPS/HSTS en borde, Strict-Transport-Security cuando dominio validado, X-Content-Type-Options nosniff, Cache-Control private/no-store en auth, CSP solo si contenido HTML.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 5. CORS

Allowlist explícita por entorno para clientes web permitidos; Native Android no requiere CORS como control de seguridad. No usar wildcard con credenciales ni aceptar Origin como autenticación.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 6. Protección de entradas

JSON UTF-8 válido, Content-Type application/json, tipo de payload estricto, límites de longitud y campos; antirrecursión y normalización. No registrar el cuerpo de auth ni texto de voz.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 7. Graceful shutdown

SIGTERM cancela aceptación, espera in-flight acotadas, cierra telemetría y clientes; readiness pasa a 503 antes de terminar, liveness solo proceso; verificar drenado del LB de dos réplicas.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 8. Compresión

Negotiation Accept-Encoding gzip en cuerpos elegibles, no secrets comprimidos en respuesta reflejada y no duplicar compresión de LB/CDN; probar tamaño vs latencia.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

## 3. Límites del alcance

- Solamente Backend, datos, seguridad, contratos API e infraestructura gestionada por el equipo Backend; ninguna implementación de Android, landing, Play Store o aprobación del equipo móvil.
- H04 congela **arquitectura lógica**; H05 congela viabilidad y topología física; H06 congela OpenAPI detallado de cada operación; H08 implementa conforme a esas decisiones.
- No convertir opciones propuestas de un catálogo histórico en compromisos silenciosos; no hacer optativas las obligaciones docentes.

## 4. Riesgos específicos y decisiones que deberán sustentarse

- **R-A-030-01:** Tiempos de espera definitivos deben validarse con prueba de carga.
- **R-A-030-02:** LB y rate limit no sustituyen validación de propiedad.

## 5. Condiciones de aceptación demostrables

- **AC-A-030-01:** Timeout, 413, 415, 405, HEAD/OPTIONS y 429 reproducibles.
- **AC-A-030-02:** No hay secreto ni PII en logs HTTP ni respuestas de error.
- **AC-A-030-03:** Shutdown sin solicitudes perdidas indebidas.

## 6. Trazabilidad y desbloqueo

- H04: aprobar este diseño en conjunto con otros once, sin ocultar discrepancias de fuentes.
- H05: seleccionar recursos AWS permitidos, demostrar costo cero y capacidad para DOS instancias simultáneas + LB/HTTPS + Redis sin simulaciones.
- H06: ratificar las 75 rutas con contratos y categorías M/S/E/O y su cobertura contra pantallas; incluir las operaciones académicas.
- H07: comprobar contradicciones, seguridad, aceptación y que nada indispensable se pospone a implementación por falta de diseño.
- H08: cada caso se implementa y prueba de forma real, aislada, con autorización de release.
