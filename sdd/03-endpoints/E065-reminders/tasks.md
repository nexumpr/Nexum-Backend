# Tareas planificadas — E065

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E065-T01` Revisar OpenAPI `operationId=nexum_e065` vs `POST /api/v1/reminders` y contrato de dominio.
- [ ] `E065-T02` Implementar request/response DTO estrictos `ReminderCreate` / `Reminder`; rechazo de campos desconocidos.
- [ ] `E065-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E065-T04` Implementar caso de uso y invariantes: Zona horaria IANA válida, programación real sujeta a permisos/costo; no programar envío sin opt-in.
- [ ] `E065-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / DEVICE#<id>, NOTIFICATION#<UTC>#<id>, REMINDER#<id>, PREFERENCES; GSI2 solo worker due.
- [ ] `E065-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E065-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E065-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E065-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E065-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E065-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E065-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E065-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E065-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E065-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E065-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E065-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E065-CT06` Repetición idéntica de Idempotency-Key -> respuesta estable; payload diferente con misma key -> 409; carrera multiinstancia.
- [ ] `E065-CT07` Opt-in/opt-out efectivo; no enviar push con token revocado; no requerir aceptación del equipo Android.
