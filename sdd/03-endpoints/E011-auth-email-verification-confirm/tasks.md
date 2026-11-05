# Tareas planificadas — E011

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E011-T01` Revisar OpenAPI `operationId=nexum_e011` vs `POST /api/v1/auth/email-verification/confirm` y contrato de dominio.
- [ ] `E011-T02` Implementar request/response DTO estrictos `VerifyEmailRequest` / `Accepted`; rechazo de campos desconocidos.
- [ ] `E011-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E011-T04` Implementar caso de uso y invariantes: Token ligado a usuario/correo; uso único transaccional; impedir repetir verificación.
- [ ] `E011-T05` Implementar adaptador DynamoDB y patrón: Email-reserva / USER#id PROFILE y SESSION#id / SESSIONHASH#digest; token hash con TTL y expiración funcional.
- [ ] `E011-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E011-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E011-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E011-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E011-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E011-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E011-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E011-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E011-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E011-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E011-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E011-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
