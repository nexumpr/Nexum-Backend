# Tareas planificadas — E066

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E066-T01` Revisar OpenAPI `operationId=nexum_e066` vs `PATCH /api/v1/reminders/{reminderId}` y contrato de dominio.
- [ ] `E066-T02` Implementar request/response DTO estrictos `ReminderPatch` / `Reminder`; rechazo de campos desconocidos.
- [ ] `E066-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E066-T04` Implementar caso de uso y invariantes: If-Match; reprogramar job idempotente y sin duplicar mensajes.
- [ ] `E066-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / DEVICE#<id>, NOTIFICATION#<UTC>#<id>, REMINDER#<id>, PREFERENCES; GSI2 solo worker due.
- [ ] `E066-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E066-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E066-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E066-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E066-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E066-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E066-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E066-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E066-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E066-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E066-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E066-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E066-CT06` If-Match omitido -> 428; ETag viejo -> 412; carrera de versiones no pierde actualizaciones.
- [ ] `E066-CT07` Opt-in/opt-out efectivo; no enviar push con token revocado; no requerir aceptación del equipo Android.
