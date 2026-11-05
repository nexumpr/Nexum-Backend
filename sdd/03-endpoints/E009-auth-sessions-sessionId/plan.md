# Plan H06 — E009: Revocar sesión identificada

**Estado:** diseño listo para revisión humana; implementación futura H08+ con Gate H07 aprobado. **Ruta:** `DELETE /api/v1/auth/sessions/{sessionId}`.

## Arquitectura y secuencia de implementación futura

1. Definir caso de uso Go en capa `application` y entidades/errores en `domain`; transportar DTO `ninguno` → `sin contenido` sin exponer SDK AWS a controladores.
2. HTTP Router/middleware valida JWT, cuerpo, límites y parámetros: auth `bearer`, idempotencia no específica, If-Match no obligatorio.
3. Adaptador DynamoDB: Email-reserva / USER#id PROFILE y SESSION#id / SESSIONHASH#digest; token hash con TTL y expiración funcional. Integrar condiciones, `Query`/`GetItem`, cero `Scan`, transacciones cuando aplique.
4. Redis: política por usuario, TTL/invalidation, revocación multiinstancia y fallo seguro. Un cache hit no puede saltarse autorización.
5. Responder HTTP 204 con esquema OpenAPI, errores uniformes y cabeceras; `Location` en 201 si corresponde.
6. Publicar un único endpoint nuevo por release con PR, CI SAST/DAST permitido, contratos y pruebas unitarias/de integración/HTTP de dos nodos, rollback y aprobación humana.

## Decisiones específicas

- ID ajeno -> 404 homogéneo; revocación persistente y propagación multiinstancia.
- Modelo de request: `sin cuerpo`. Modelo de response: `sin cuerpo`.
- Patrón de persistencia: Email-reserva / USER#id PROFILE y SESSION#id / SESSIONHASH#digest; token hash con TTL y expiración funcional.
- Obligación: `REQUIRED_DESIGN`. Requerido por el catálogo o soporte vigente; no rebajarlo a demo.

## Dependencias y riesgos

- No crear recursos ni ejecutar despliegues en H06. AWS Free/45 días/cero pesos y ciclo IaC cerrado son condiciones de H08, no tareas manuales de Mobile.
- Errores de concurrencia 409/412, reglas de negocio 422, disponibilidad 503 y seguridad de recursos ajenos.
- Prueba real HTTP/HTTPS y DynamoDB/Redis: **NOT_RUN**; solo el contrato/documento es verificable ahora.
