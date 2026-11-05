# Tareas planificadas — E005

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E005-T01` Revisar OpenAPI `operationId=nexum_e005` vs `POST /api/v1/auth/password/forgot` y contrato de dominio.
- [ ] `E005-T02` Implementar request/response DTO estrictos `EmailRequest` / `Accepted`; rechazo de campos desconocidos.
- [ ] `E005-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E005-T04` Implementar caso de uso y invariantes: Respuesta 202 indistinguible para correos existentes/no existentes; token de un uso y expiración; envío mediante adaptador verificado.
- [ ] `E005-T05` Implementar adaptador DynamoDB y patrón: Email-reserva / USER#id PROFILE y SESSION#id / SESSIONHASH#digest; token hash con TTL y expiración funcional.
- [ ] `E005-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E005-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E005-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E005-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E005-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E005-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E005-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E005-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E005-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E005-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E005-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E005-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
