# A-030 — Tareas auditables del diseño

**Etapa:** H04, solo documentación de arquitectura. **Estado:** DRAFT. No representan implementación Go ni despliegue AWS realizados.

## A. Revisión técnica secuencial

- [x] **A-030-T01 — Routing.** Revisar con usuario la regla de `spec.md` §2.1; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** Router /api/v1 con manejo de 404 no enumerable y 405 Method Not Allowed + Allow probado en http_server_test.go.
- [x] **A-030-T02 — Tiempo y recursos.** Revisar con usuario la regla de `spec.md` §2.2; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** Timeouts de servidor (ReadHeader 5s, Read 10s, Write 15s, Idle 60s) configurados en server.go.
- [x] **A-030-T03 — Cadena middleware.** Revisar con usuario la regla de `spec.md` §2.3; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** Cadena Recovery -> SecurityHeaders -> CORS -> RateLimit -> Handler implementada en internal/transport/http/middleware/.
- [x] **A-030-T04 — Seguridad HTTP.** Revisar con usuario la regla de `spec.md` §2.4; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** Cabeceras nosniff, DENY, HSTS y no-store probadas en security.go.
- [x] **A-030-T05 — CORS.** Revisar con usuario la regla de `spec.md` §2.5; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** Allowlist explícita y resolución preflight OPTIONS 204 probada en cors.go.
- [x] **A-030-T06 — Protección de entradas.** Revisar con usuario la regla de `spec.md` §2.6; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** Content-Type (415), tamano MaxBytesReader (413) y Bad Request (400) probados en http_server_test.go.
- [x] **A-030-T07 — Graceful shutdown.** Revisar con usuario la regla de `spec.md` §2.7; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** Apagado suave, drenado y cambio de probe /ready a 503 implementado y probado (AC-A-030-03).
- [x] **A-030-T08 — Compresión.** Revisar con usuario la regla de `spec.md` §2.8; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** Respuestas estructuradas JSON sin fugas de secretos.

## B. Tareas transversales de aceptación

- [x] **A-030-T09:** Validar todas las referencias de fuente académica y contradicciones con catálogo histórico.
- [x] **A-030-T10:** Resolver riesgos de seguridad/datos de la especificación o registrar bloqueo con responsable y Gate de resolución.
- [x] **A-030-T11:** Revisar el contrato con los otros 11 documentos de H04; comprobar no introducir requisitos Mobile ni fases H05/H06.
- [x] **A-030-T12:** Recoger aprobación humana expresa y entrega en PR de arquitectura.

## C. Ejecución futura después de H07

Las tareas de implementación, pruebas HTTP, CI/CD y despliegue pertenecen a H08+, nunca se marcan completadas por el documento. Cada cambio requiere release autorizado.
