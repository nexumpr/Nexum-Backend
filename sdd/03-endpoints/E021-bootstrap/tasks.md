# Tareas planificadas — E021

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E021-T01` Revisar OpenAPI `operationId=nexum_e021` vs `GET /api/v1/bootstrap` y contrato de dominio.
- [ ] `E021-T02` Implementar request/response DTO estrictos `ninguno` / `Bootstrap`; rechazo de campos desconocidos.
- [ ] `E021-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E021-T04` Implementar caso de uso y invariantes: Solo agregados estrictamente necesarios; sin sesión ni credenciales; cache privada por usuario con invalidación.
- [ ] `E021-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / PROFILE, CONSENT#*, ONBOARDING, PREFERENCES; privacidad y revocación transversal.
- [ ] `E021-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E021-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E021-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E021-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E021-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E021-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E021-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E021-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E021-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E021-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E021-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E021-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
