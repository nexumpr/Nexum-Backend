# Tareas planificadas — E074

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E074-T01` Revisar OpenAPI `operationId=nexum_e074` vs `GET /api/v1/demo/bootstrap` y contrato de dominio.
- [ ] `E074-T02` Implementar request/response DTO estrictos `ninguno` / `DemoBootstrap`; rechazo de campos desconocidos.
- [ ] `E074-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E074-T04` Implementar caso de uso y invariantes: Opcional: si se autoriza, aislar por entorno y sujeto; nunca exponer en producción a usuarios reales.
- [ ] `E074-T05` Implementar adaptador DynamoDB y patrón: Demo aislada de producción, sin datos reales y habilitación por environment flag de IaC.
- [ ] `E074-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E074-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E074-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E074-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E074-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E074-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E074-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E074-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E074-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E074-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E074-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E074-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
