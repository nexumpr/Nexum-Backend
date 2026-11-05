# Tareas planificadas — E064

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E064-T01` Revisar OpenAPI `operationId=nexum_e064` vs `GET /api/v1/reminders` y contrato de dominio.
- [ ] `E064-T02` Implementar request/response DTO estrictos `ninguno` / `PageOfReminder`; rechazo de campos desconocidos.
- [ ] `E064-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E064-T04` Implementar caso de uso y invariantes: Solo recordatorios del usuario; por vencimiento y estado.
- [ ] `E064-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / DEVICE#<id>, NOTIFICATION#<UTC>#<id>, REMINDER#<id>, PREFERENCES; GSI2 solo worker due.
- [ ] `E064-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E064-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E064-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E064-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E064-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E064-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E064-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E064-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E064-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E064-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E064-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E064-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E064-CT06` Paginación 25/100, filtro modificado con cursor antiguo -> 400, lectura sin Scan, sin duplicados en páginas.
- [ ] `E064-CT07` Opt-in/opt-out efectivo; no enviar push con token revocado; no requerir aceptación del equipo Android.
