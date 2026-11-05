# Plan H06 — E068: Consultar preferencias personales

**Estado:** diseño listo para revisión humana; implementación futura H08+ con Gate H07 aprobado. **Ruta:** `GET /api/v1/users/me/preferences`.

## Arquitectura y secuencia de implementación futura

1. Definir caso de uso Go en capa `application` y entidades/errores en `domain`; transportar DTO `ninguno` → `UserPreferences` sin exponer SDK AWS a controladores.
2. HTTP Router/middleware valida JWT, cuerpo, límites y parámetros: auth `bearer`, idempotencia no específica, If-Match no obligatorio.
3. Adaptador DynamoDB: USER#<sub> / PREFERENCES con control de version y unicidad. Integrar condiciones, `Query`/`GetItem`, cero `Scan`, transacciones cuando aplique.
4. Redis: política por usuario, TTL/invalidation, revocación multiinstancia y fallo seguro. Un cache hit no puede saltarse autorización.
5. Responder HTTP 200 con esquema OpenAPI, errores uniformes y cabeceras; `Location` en 201 si corresponde.
6. Publicar un único endpoint nuevo por release con PR, CI SAST/DAST permitido, contratos y pruebas unitarias/de integración/HTTP de dos nodos, rollback y aprobación humana.

## Decisiones específicas

- Ruta opcional; idioma y zona horaria con validación, sin revelar privacidad.
- Modelo de request: `sin cuerpo`. Modelo de response: `UserPreferences`.
- Patrón de persistencia: USER#<sub> / PREFERENCES con control de version y unicidad.
- Obligación: `OPTIONAL_DOCUMENTED`. Opcional histórico: implementación no se activa sin decisión expresa del responsable.

## Dependencias y riesgos

- No crear recursos ni ejecutar despliegues en H06. AWS Free/45 días/cero pesos y ciclo IaC cerrado son condiciones de H08, no tareas manuales de Mobile.
- Errores de concurrencia 409/412, reglas de negocio 422, disponibilidad 503 y seguridad de recursos ajenos.
- Prueba real HTTP/HTTPS y DynamoDB/Redis: **NOT_RUN**; solo el contrato/documento es verificable ahora.
