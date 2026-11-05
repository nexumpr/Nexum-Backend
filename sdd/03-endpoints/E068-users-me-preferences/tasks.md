# Tareas planificadas — E068

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E068-T01` Revisar OpenAPI `operationId=nexum_e068` vs `GET /api/v1/users/me/preferences` y contrato de dominio.
- [ ] `E068-T02` Implementar request/response DTO estrictos `ninguno` / `UserPreferences`; rechazo de campos desconocidos.
- [ ] `E068-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E068-T04` Implementar caso de uso y invariantes: Ruta opcional; idioma y zona horaria con validación, sin revelar privacidad.
- [ ] `E068-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / PREFERENCES con control de version y unicidad.
- [ ] `E068-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E068-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E068-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E068-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E068-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E068-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E068-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E068-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E068-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E068-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E068-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E068-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
