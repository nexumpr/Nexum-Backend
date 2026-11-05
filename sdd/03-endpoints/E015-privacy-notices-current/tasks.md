# Tareas planificadas — E015

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E015-T01` Revisar OpenAPI `operationId=nexum_e015` vs `GET /api/v1/privacy/notices/current` y contrato de dominio.
- [ ] `E015-T02` Implementar request/response DTO estrictos `ninguno` / `PrivacyNotice`; rechazo de campos desconocidos.
- [ ] `E015-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E015-T04` Implementar caso de uso y invariantes: Versión pública firmada y fecha de vigencia; no datos de usuario.
- [ ] `E015-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / PROFILE, CONSENT#*, ONBOARDING, PREFERENCES; privacidad y revocación transversal.
- [ ] `E015-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E015-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E015-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E015-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E015-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E015-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E015-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E015-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E015-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E015-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E015-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E015-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
