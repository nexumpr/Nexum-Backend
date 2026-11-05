# Tareas planificadas — E016

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E016-T01` Revisar OpenAPI `operationId=nexum_e016` vs `GET /api/v1/users/me/consents` y contrato de dominio.
- [ ] `E016-T02` Implementar request/response DTO estrictos `ninguno` / `PageOfConsent`; rechazo de campos desconocidos.
- [ ] `E016-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E016-T04` Implementar caso de uso y invariantes: Historial inmutable de consentimiento/versionado, ordenado y sin alteraciones retroactivas.
- [ ] `E016-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / PROFILE, CONSENT#*, ONBOARDING, PREFERENCES; privacidad y revocación transversal.
- [ ] `E016-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E016-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E016-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E016-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E016-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E016-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E016-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E016-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E016-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E016-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E016-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E016-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E016-CT06` Paginación 25/100, filtro modificado con cursor antiguo -> 400, lectura sin Scan, sin duplicados en páginas.
