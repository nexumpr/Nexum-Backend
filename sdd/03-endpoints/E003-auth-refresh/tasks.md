# Tareas planificadas — E003

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E003-T01` Revisar OpenAPI `operationId=nexum_e003` vs `POST /api/v1/auth/refresh` y contrato de dominio.
- [ ] `E003-T02` Implementar request/response DTO estrictos `RefreshRequest` / `AuthTokens`; rechazo de campos desconocidos.
- [ ] `E003-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E003-T04` Implementar caso de uso y invariantes: Rotación de un solo uso por compare-and-swap; reutilización revoca familia de sesión en todos los nodos.
- [ ] `E003-T05` Implementar adaptador DynamoDB y patrón: Email-reserva / USER#id PROFILE y SESSION#id / SESSIONHASH#digest; token hash con TTL y expiración funcional.
- [ ] `E003-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E003-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E003-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E003-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E003-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E003-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E003-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E003-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E003-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E003-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E003-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E003-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E003-CT06` Revocación/rotación en nodo A persiste y es observable inmediatamente en nodo B; token reutilizado rechazado.
