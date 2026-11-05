# Tareas planificadas — E059

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E059-T01` Revisar OpenAPI `operationId=nexum_e059` vs `POST /api/v1/devices/push-tokens` y contrato de dominio.
- [ ] `E059-T02` Implementar request/response DTO estrictos `PushTokenRegister` / `PushDevice`; rechazo de campos desconocidos.
- [ ] `E059-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E059-T04` Implementar caso de uso y invariantes: Token de mensajería cifrado en reposo, reemplazar rotación de token, separar instalación y usuario.
- [ ] `E059-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / DEVICE#<id>, NOTIFICATION#<UTC>#<id>, REMINDER#<id>, PREFERENCES; GSI2 solo worker due.
- [ ] `E059-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E059-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E059-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E059-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E059-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E059-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E059-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E059-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E059-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E059-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E059-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E059-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E059-CT06` Repetición idéntica de Idempotency-Key -> respuesta estable; payload diferente con misma key -> 409; carrera multiinstancia.
- [ ] `E059-CT07` Opt-in/opt-out efectivo; no enviar push con token revocado; no requerir aceptación del equipo Android.
