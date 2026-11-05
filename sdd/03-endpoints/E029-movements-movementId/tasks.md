# Tareas planificadas — E029

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E029-T01` Revisar OpenAPI `operationId=nexum_e029` vs `GET /api/v1/movements/{movementId}` y contrato de dominio.
- [ ] `E029-T02` Implementar request/response DTO estrictos `ninguno` / `Movement`; rechazo de campos desconocidos.
- [ ] `E029-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E029-T04` Implementar caso de uso y invariantes: Localizador por ID del mismo usuario -> SK temporal; 404 para inexistente/ajeno.
- [ ] `E029-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / MOVEMENT#<UTC>#<id>; MOVEMENT_ID#<id> locator; GSI1 para filtros por tipo/fecha.
- [ ] `E029-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E029-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E029-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E029-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E029-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E029-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E029-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E029-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E029-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E029-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E029-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E029-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E029-CT06` Montos exactos en centavos: 0, mínimo, máximo, decimal inválido, cambio de mes y zona IANA.
