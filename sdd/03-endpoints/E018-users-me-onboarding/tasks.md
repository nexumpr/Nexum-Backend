# Tareas planificadas — E018

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E018-T01` Revisar OpenAPI `operationId=nexum_e018` vs `GET /api/v1/users/me/onboarding` y contrato de dominio.
- [ ] `E018-T02` Implementar request/response DTO estrictos `ninguno` / `Onboarding`; rechazo de campos desconocidos.
- [ ] `E018-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E018-T04` Implementar caso de uso y invariantes: Estado determinado por datos propios; nunca bloquear operaciones esenciales por paso visual local.
- [ ] `E018-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / PROFILE, CONSENT#*, ONBOARDING, PREFERENCES; privacidad y revocación transversal.
- [ ] `E018-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E018-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E018-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E018-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E018-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E018-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E018-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E018-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E018-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E018-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E018-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E018-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
