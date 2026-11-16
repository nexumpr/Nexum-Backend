# Plan H06 — E005: Solicitar recuperación de contraseña

**Estado:** diseño listo para revisión humana; implementación futura H08+ con Gate H07 aprobado. **Ruta:** `POST /api/v1/auth/password/forgot`.

## Arquitectura y secuencia de implementación futura

1. Definir caso de uso Go en capa `application` y entidades/errores en `domain`; transportar DTO `EmailRequest` → `Accepted` sin exponer SDK AWS a controladores.
2. HTTP Router/middleware valida JWT, cuerpo, límites y parámetros: auth `public`, idempotencia no específica, If-Match no obligatorio.
3. Adaptador DynamoDB: Email-reserva / USER#id PROFILE y SESSION#id / REFRESH#<digest>/SESSION; token hash con TTL y expiración funcional. Integrar condiciones, `Query`/`GetItem`, cero `Scan`, transacciones cuando aplique.
4. Redis: política por usuario, TTL/invalidation, revocación multiinstancia y fallo seguro. Un cache hit no puede saltarse autorización.
5. Responder HTTP 202 con esquema OpenAPI, errores uniformes y cabeceras; `Location` en 201 si corresponde.
6. Publicar un único endpoint nuevo por release con PR, CI SAST/DAST permitido, contratos y pruebas unitarias/de integración/HTTP de dos nodos, rollback y aprobación humana.

## Decisiones específicas

- Respuesta 202 indistinguible para correos existentes/no existentes; token de un uso y expiración; envío mediante adaptador verificado.
- Modelo de request: `EmailRequest`. Modelo de response: `Accepted`.
- Patrón de persistencia: Email-reserva / USER#id PROFILE y SESSION#id / REFRESH#<digest>/SESSION; token hash con TTL y expiración funcional.
- Obligación: `REQUIRED_DESIGN`. Requerido por el catálogo o soporte vigente; no rebajarlo a demo.

## Dependencias y riesgos

- No crear recursos ni ejecutar despliegues en H06. AWS Free/45 días/cero pesos y ciclo IaC cerrado son condiciones de H08, no tareas manuales de Mobile.
- Errores de concurrencia 409/412, reglas de negocio 422, disponibilidad 503 y seguridad de recursos ajenos.
- Prueba real HTTP/HTTPS y DynamoDB/Redis: **NOT_RUN**; solo el contrato/documento es verificable ahora.

## Impacto de la enmienda — RFC-016 (A1)

- Persistencia: se almacena en el mismo item del recurso; no requiere tabla ni indice nuevo.
- Concurrencia: se mantiene el control por `version`/`If-Match` ya definido.
- Ida y vuelta: el dato escrito se devuelve en las respuestas de creacion, detalle y listado.
- Dependencias: ninguna nueva de AWS; sin coste adicional.

## Plan de integración SES (A11)

- Secreto **A9** `PASSWORD_RESET_SEAL_KEY_B64` (AES-256-GCM, AAD `userId|outboxId`) en el bundle
  RFC-004; sin KMS ni secreto adicional.
- IAM **A10**: `LeadingKeys` con `QUEUE#PWRESET` y `RESET#<sha256>` además de `USER#*`/`REFRESH#*`/
  `SESSION#*`; acción `ses:SendEmail` acotada a la identidad verificada.
- Red: sin NAT. Se evaluará interface endpoint de SES API **por ventana** (0,40 USD por ciclo);
  si SES no admite endpoint privado, `BLOCKED_E005_SES_CONNECTIVITY`.
- Worker: líder por lease Redis, reclamación atómica con `fence`, backoff truncado a la expiración.
- Errores de SES no alteran la respuesta (evita enumeración); el trabajo queda `PENDIENTE`.
