# Tareas planificadas — E060

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E060-T01` Revisar OpenAPI `operationId=nexum_e060` vs `DELETE /api/v1/devices/push-tokens/{deviceRegistrationId}` y contrato de dominio.
- [ ] `E060-T02` Implementar request/response DTO estrictos `ninguno` / `ninguno`; rechazo de campos desconocidos.
- [ ] `E060-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E060-T04` Implementar caso de uso y invariantes: Revocar solamente dispositivo del usuario; borrar token cifrado y prevenir envíos posteriores.
- [ ] `E060-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / DEVICE#<id>, NOTIFICATION#<UTC>#<id>, REMINDER#<id>, PREFERENCES; GSI2 solo worker due.
- [ ] `E060-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E060-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E060-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E060-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E060-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E060-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E060-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E060-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E060-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E060-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E060-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E060-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E060-CT06` Opt-in/opt-out efectivo; no enviar push con token revocado; no requerir aceptación del equipo Android.
