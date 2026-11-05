# Tareas planificadas — E022

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E022-T01` Revisar OpenAPI `operationId=nexum_e022` vs `GET /api/v1/catalogs/categories` y contrato de dominio.
- [ ] `E022-T02` Implementar request/response DTO estrictos `ninguno` / `PageOfCategory`; rechazo de campos desconocidos.
- [ ] `E022-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E022-T04` Implementar caso de uso y invariantes: Catálogo público versionado; identificadores estables, sin Scan sobre datos privados.
- [ ] `E022-T05` Implementar adaptador DynamoDB y patrón: PUBLIC#CATALOG / CATEGORY#id o FREQUENCY#id; versión publicada y Redis TTL con invalidación.
- [ ] `E022-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E022-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E022-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E022-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E022-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E022-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E022-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E022-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E022-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E022-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E022-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E022-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
