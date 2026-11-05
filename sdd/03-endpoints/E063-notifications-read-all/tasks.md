# Tareas planificadas — E063

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E063-T01` Revisar OpenAPI `operationId=nexum_e063` vs `POST /api/v1/notifications/read-all` y contrato de dominio.
- [ ] `E063-T02` Implementar request/response DTO estrictos `ninguno` / `Accepted`; rechazo de campos desconocidos.
- [ ] `E063-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E063-T04` Implementar caso de uso y invariantes: Operación opcional del catálogo; fan-out por lotes con cursor, no Scan global.
- [ ] `E063-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / DEVICE#<id>, NOTIFICATION#<UTC>#<id>, REMINDER#<id>, PREFERENCES; GSI2 solo worker due.
- [ ] `E063-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E063-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E063-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E063-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E063-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E063-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E063-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E063-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E063-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E063-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E063-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E063-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
