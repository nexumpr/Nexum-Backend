# Tareas planificadas — E057

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E057-T01` Revisar OpenAPI `operationId=nexum_e057` vs `GET /api/v1/notification-preferences` y contrato de dominio.
- [ ] `E057-T02` Implementar request/response DTO estrictos `ninguno` / `NotificationPreferences`; rechazo de campos desconocidos.
- [ ] `E057-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E057-T04` Implementar caso de uso y invariantes: Preferencias por canal, opt-in privado y estado real.
- [ ] `E057-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / DEVICE#<id>, NOTIFICATION#<UTC>#<id>, REMINDER#<id>, PREFERENCES; GSI2 solo worker due.
- [ ] `E057-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E057-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E057-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E057-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E057-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E057-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E057-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E057-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E057-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E057-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E057-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E057-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
