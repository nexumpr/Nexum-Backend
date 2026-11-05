# A-030 — Plan técnico de arquitectura

**Estado:** DRAFT H04. **Entrada obligatoria:** `spec.md` y fuentes de A-030. **Salida:** decisión lista para implementar después de H07; este plan NO ejecuta infraestructura ni código.

## 1. Estrategia de construcción

1. **Routing.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. /api/v1 con rutas explícitas y 404 no enumerable. Resolver PathValue y métodos; rechazar método no permitido con 405+Allow donde corresponda.
2. **Tiempo y recursos.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Propuesta inicial (validar carga): readHeader 5s, read 10s, write 15s, idle 60s, respuesta backend 10s, request JSON normal <=1 MiB. Streaming/export se modela aparte. Propagar contexto y deadline hasta AWS SDK.
3. **Cadena middleware.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Recovery -> requestId/tracing -> logging saneado -> real-IP desde proxy confiable -> size limit -> CORS/OPTIONS -> auth -> rate limit -> autorización recurso -> handler; no confiar X-Forwarded-* fuera del balanceador.
4. **Seguridad HTTP.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. TLS termina en LB AWS y opción mTLS interna si H05 lo determina; HTTPS/HSTS en borde, Strict-Transport-Security cuando dominio validado, X-Content-Type-Options nosniff, Cache-Control private/no-store en auth, CSP solo si contenido HTML.
5. **CORS.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Allowlist explícita por entorno para clientes web permitidos; Native Android no requiere CORS como control de seguridad. No usar wildcard con credenciales ni aceptar Origin como autenticación.
6. **Protección de entradas.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. JSON UTF-8 válido, Content-Type application/json, tipo de payload estricto, límites de longitud y campos; antirrecursión y normalización. No registrar el cuerpo de auth ni texto de voz.
7. **Graceful shutdown.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. SIGTERM cancela aceptación, espera in-flight acotadas, cierra telemetría y clientes; readiness pasa a 503 antes de terminar, liveness solo proceso; verificar drenado del LB de dos réplicas.
8. **Compresión.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Negotiation Accept-Encoding gzip en cuerpos elegibles, no secrets comprimidos en respuesta reflejada y no duplicar compresión de LB/CDN; probar tamaño vs latencia.

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

- Tiempos de espera definitivos deben validarse con prueba de carga. **Mitigación:** resolver antes de aprobar su dependencia; si requiere cambio de constitución emitir RFC.
- LB y rate limit no sustituyen validación de propiedad. **Mitigación:** resolver antes de aprobar su dependencia; si requiere cambio de constitución emitir RFC.

## 5. Criterio de cierre

Se considera listo para revisión cuando los 8 ejes técnicos de `spec.md` tienen trazabilidad a pruebas en `checklist.md`, los elementos futuros permanecen `NOT_RUN`, la matriz de alcance está intacta y las decisiones pendientes tienen dueño/fase, sin `APPROVED` automático.
