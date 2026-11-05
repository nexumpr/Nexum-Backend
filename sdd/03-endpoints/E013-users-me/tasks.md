# Tareas planificadas — E013

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E013-T01` Revisar OpenAPI `operationId=nexum_e013` vs `PATCH /api/v1/users/me` y contrato de dominio.
- [ ] `E013-T02` Implementar request/response DTO estrictos `ProfilePatch` / `UserProfile`; rechazo de campos desconocidos.
- [ ] `E013-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E013-T04` Implementar caso de uso y invariantes: PATCH parcial, If-Match versión requerida; campos sensibles requieren verificación independiente.
- [ ] `E013-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / PROFILE, CONSENT#*, ONBOARDING, PREFERENCES; privacidad y revocación transversal.
- [ ] `E013-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E013-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E013-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E013-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E013-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E013-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E013-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E013-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E013-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E013-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E013-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E013-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E013-CT06` If-Match omitido -> 428; ETag viejo -> 412; carrera de versiones no pierde actualizaciones.
