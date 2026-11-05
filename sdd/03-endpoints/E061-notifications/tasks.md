# Tareas planificadas — E061

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E061-T01` Revisar OpenAPI `operationId=nexum_e061` vs `GET /api/v1/notifications` y contrato de dominio.
- [ ] `E061-T02` Implementar request/response DTO estrictos `ninguno` / `PageOfNotification`; rechazo de campos desconocidos.
- [ ] `E061-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E061-T04` Implementar caso de uso y invariantes: Solo destinatario; cursor y filtros isRead; sin PII sensible en textos push.
- [ ] `E061-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / DEVICE#<id>, NOTIFICATION#<UTC>#<id>, REMINDER#<id>, PREFERENCES; GSI2 solo worker due.
- [ ] `E061-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E061-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E061-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E061-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E061-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E061-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E061-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E061-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E061-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E061-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E061-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E061-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E061-CT06` Paginación 25/100, filtro modificado con cursor antiguo -> 400, lectura sin Scan, sin duplicados en páginas.
- [ ] `E061-CT07` Opt-in/opt-out efectivo; no enviar push con token revocado; no requerir aceptación del equipo Android.
