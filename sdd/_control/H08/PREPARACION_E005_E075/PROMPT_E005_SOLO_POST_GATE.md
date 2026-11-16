# E005 — Prompt reservado para uso después de READY_FOR_E005 y Gate

**NO EJECUTAR AHORA.** Cuando el responsable autorice el inicio de E005, regenerar el prompt exacto a partir de `spec.md`, `plan.md`, `tasks.md`, `checklist.md`, OpenAPI YAML/JSON y RFC-016 aprobados e integrados. No usar este borrador como autorización.

MODELO: DeepSeek V4.1 Flash (High). Motivo: implementar envío real de recuperación sin enumeración. Consumo estimado: medio-alto. Cambio de modelo: únicamente otro DeepSeek con aprobación.

Objetivo E005: `POST /api/v1/auth/password/forgot`. No asumir contrato HTTP/JSON hasta reconciliar si `Accepted {status,requestId}` sigue vigente o se aprobó propuesta V6.0.2 `{accepted,message,retryAfterSeconds}`. Autentificación pública; no revelar si correo existe; respuesta 202 indistinguible; rate limiting distribuido, token de reset de un solo uso y expiración (hash persistido con TTL real), envío de correo auténtico con proveedor y coste autorizados; nunca retornar o registrar token/link secreto. No hacer mock de envío en AWS. Reutilizar DynamoDB/Redis existentes. Probar request/response exactos, IP/correo, expiración, replay, concurrencia y regresión E001–E004. Un PR técnico, CI verde, Gate merge SHA40, despliegue incremental S3/LT/ASG con Gates por operación y pruebas HTTPS/Postman. Entregar contrato Android verificado y memoria.md al día. No iniciar E006.
