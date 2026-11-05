# Tareas planificadas — E058

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E058-T01` Revisar OpenAPI `operationId=nexum_e058` vs `PUT /api/v1/notification-preferences` y contrato de dominio.
- [ ] `E058-T02` Implementar request/response DTO estrictos `NotificationPreferencesUpdate` / `NotificationPreferences`; rechazo de campos desconocidos.
- [ ] `E058-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E058-T04` Implementar caso de uso y invariantes: Desactivación debe prevalecer antes de enviar; consentimiento explícito por canal.
- [ ] `E058-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / DEVICE#<id>, NOTIFICATION#<UTC>#<id>, REMINDER#<id>, PREFERENCES; GSI2 solo worker due.
- [ ] `E058-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E058-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E058-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E058-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E058-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E058-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E058-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E058-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E058-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E058-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E058-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E058-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
