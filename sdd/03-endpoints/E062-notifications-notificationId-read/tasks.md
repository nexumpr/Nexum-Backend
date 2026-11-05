# Tareas planificadas — E062

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E062-T01` Revisar OpenAPI `operationId=nexum_e062` vs `PATCH /api/v1/notifications/{notificationId}/read` y contrato de dominio.
- [ ] `E062-T02` Implementar request/response DTO estrictos `ninguno` / `Notification`; rechazo de campos desconocidos.
- [ ] `E062-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E062-T04` Implementar caso de uso y invariantes: Idempotente por usuario; recurso ajeno ->404.
- [ ] `E062-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / DEVICE#<id>, NOTIFICATION#<UTC>#<id>, REMINDER#<id>, PREFERENCES; GSI2 solo worker due.
- [ ] `E062-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E062-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E062-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E062-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E062-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E062-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E062-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E062-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E062-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E062-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E062-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E062-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E062-CT06` Opt-in/opt-out efectivo; no enviar push con token revocado; no requerir aceptación del equipo Android.
