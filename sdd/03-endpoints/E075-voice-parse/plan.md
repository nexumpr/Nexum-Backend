# Plan H06 — E075: Interpretar texto de voz en una intención

**Estado:** diseño listo para revisión humana; implementación futura H08+ con Gate H07 aprobado. **Ruta:** `POST /api/v1/voice/parse`.

## Arquitectura y secuencia de implementación futura

1. Definir caso de uso Go en capa `application` y entidades/errores en `domain`; transportar DTO `VoiceParseRequest` → `VoiceParse` sin exponer SDK AWS a controladores.
2. HTTP Router/middleware valida JWT, cuerpo, límites y parámetros: auth `bearer`, idempotencia no específica, If-Match no obligatorio.
3. Adaptador DynamoDB: No se persiste audio ni texto de voz por defecto; inferencia aislada y confirmación explícita para efectos financieros. Integrar condiciones, `Query`/`GetItem`, cero `Scan`, transacciones cuando aplique.
4. Redis: política por usuario, TTL/invalidation, revocación multiinstancia y fallo seguro. Un cache hit no puede saltarse autorización.
5. Responder HTTP 200 con esquema OpenAPI, errores uniformes y cabeceras; `Location` en 201 si corresponde.
6. Publicar un único endpoint nuevo por release con PR, CI SAST/DAST permitido, contratos y pruebas unitarias/de integración/HTTP de dos nodos, rollback y aprobación humana.

## Decisiones específicas

- Opcional: reconocimiento de voz es local; endpoint interpreta texto, no audio; nunca ejecutar operación financiera sin confirmación explícita.
- Modelo de request: `VoiceParseRequest`. Modelo de response: `VoiceParse`.
- Patrón de persistencia: No se persiste audio ni texto de voz por defecto; inferencia aislada y confirmación explícita para efectos financieros.
- Obligación: `OPTIONAL_DOCUMENTED`. Opcional histórico: implementación no se activa sin decisión expresa del responsable.

## Dependencias y riesgos

- No crear recursos ni ejecutar despliegues en H06. AWS Free/45 días/cero pesos y ciclo IaC cerrado son condiciones de H08, no tareas manuales de Mobile.
- Errores de concurrencia 409/412, reglas de negocio 422, disponibilidad 503 y seguridad de recursos ajenos.
- Prueba real HTTP/HTTPS y DynamoDB/Redis: **NOT_RUN**; solo el contrato/documento es verificable ahora.
