# Plan H06 — E037: Registrar pago de regla recurrente

**Estado:** diseño listo para revisión humana; implementación futura H08+ con Gate H07 aprobado. **Ruta:** `POST /api/v1/recurring-payments/{recurringPaymentId}/payments`.

## Arquitectura y secuencia de implementación futura

1. Definir caso de uso Go en capa `application` y entidades/errores en `domain`; transportar DTO `RecurringPaymentCreate` → `Movement` sin exponer SDK AWS a controladores.
2. HTTP Router/middleware valida JWT, cuerpo, límites y parámetros: auth `bearer`, idempotencia sí, If-Match no obligatorio.
3. Adaptador DynamoDB: USER#<sub> / RECURRING#<id>; transacción de pago con MOVEMENT#<UTC>#<id> e idempotencia. Integrar condiciones, `Query`/`GetItem`, cero `Scan`, transacciones cuando aplique.
4. Redis: política por usuario, TTL/invalidation, revocación multiinstancia y fallo seguro. Un cache hit no puede saltarse autorización.
5. Responder HTTP 200 con esquema OpenAPI, errores uniformes y cabeceras; `Location` en 201 si corresponde.
6. Publicar un único endpoint nuevo por release con PR, CI SAST/DAST permitido, contratos y pruebas unitarias/de integración/HTTP de dos nodos, rollback y aprobación humana.

## Decisiones específicas

- Idempotency-Key y transacción: movimiento de egreso + vínculo a regla + control de duplicado por fecha.
- Modelo de request: `RecurringPaymentCreate`. Modelo de response: `Movement`.
- Patrón de persistencia: USER#<sub> / RECURRING#<id>; transacción de pago con MOVEMENT#<UTC>#<id> e idempotencia.
- Obligación: `REQUIRED_DESIGN`. Requerido por el catálogo o soporte vigente; no rebajarlo a demo.

## Dependencias y riesgos

- No crear recursos ni ejecutar despliegues en H06. AWS Free/45 días/cero pesos y ciclo IaC cerrado son condiciones de H08, no tareas manuales de Mobile.
- Errores de concurrencia 409/412, reglas de negocio 422, disponibilidad 503 y seguridad de recursos ajenos.
- Prueba real HTTP/HTTPS y DynamoDB/Redis: **NOT_RUN**; solo el contrato/documento es verificable ahora.
