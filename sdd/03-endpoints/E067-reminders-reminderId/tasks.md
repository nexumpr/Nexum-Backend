# Tareas planificadas — E067

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E067-T01` Revisar OpenAPI `operationId=nexum_e067` vs `DELETE /api/v1/reminders/{reminderId}` y contrato de dominio.
- [ ] `E067-T02` Implementar request/response DTO estrictos `ninguno` / `ninguno`; rechazo de campos desconocidos.
- [ ] `E067-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E067-T04` Implementar caso de uso y invariantes: Cancelar job pendiente y soft delete idempotente.
- [ ] `E067-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / DEVICE#<id>, NOTIFICATION#<UTC>#<id>, REMINDER#<id>, PREFERENCES; GSI2 solo worker due.
- [ ] `E067-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E067-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E067-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E067-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E067-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E067-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E067-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E067-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E067-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E067-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E067-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E067-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E067-CT06` Opt-in/opt-out efectivo; no enviar push con token revocado; no requerir aceptación del equipo Android.
