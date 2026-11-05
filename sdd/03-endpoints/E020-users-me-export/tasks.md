# Tareas planificadas — E020

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E020-T01` Revisar OpenAPI `operationId=nexum_e020` vs `GET /api/v1/users/me/export` y contrato de dominio.
- [ ] `E020-T02` Implementar request/response DTO estrictos `ninguno` / `UserExport`; rechazo de campos desconocidos.
- [ ] `E020-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E020-T04` Implementar caso de uso y invariantes: Exportación sin datos de otros usuarios, paginación interna y límites; respuesta JSON redacted sin tokens.
- [ ] `E020-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / PROFILE, CONSENT#*, ONBOARDING, PREFERENCES; privacidad y revocación transversal.
- [ ] `E020-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E020-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E020-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E020-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E020-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E020-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E020-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E020-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E020-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E020-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E020-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E020-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
