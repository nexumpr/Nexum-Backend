# Tareas planificadas — E017

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E017-T01` Revisar OpenAPI `operationId=nexum_e017` vs `POST /api/v1/users/me/consents` y contrato de dominio.
- [ ] `E017-T02` Implementar request/response DTO estrictos `ConsentRequest` / `Consent`; rechazo de campos desconocidos.
- [ ] `E017-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E017-T04` Implementar caso de uso y invariantes: No marcar opt-in por omisión; conservar documento/versión, instante UTC, fuente y decisión.
- [ ] `E017-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / PROFILE, CONSENT#*, ONBOARDING, PREFERENCES; privacidad y revocación transversal.
- [ ] `E017-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E017-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E017-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E017-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E017-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E017-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E017-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E017-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E017-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E017-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E017-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E017-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
