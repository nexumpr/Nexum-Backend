# Tareas planificadas — E046

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E046-T01` Revisar OpenAPI `operationId=nexum_e046` vs `GET /api/v1/goal-contributions/{contributionId}` y contrato de dominio.
- [ ] `E046-T02` Implementar request/response DTO estrictos `ninguno` / `Contribution`; rechazo de campos desconocidos.
- [ ] `E046-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E046-T04` Implementar caso de uso y invariantes: Localizador por ID bajo PK del usuario, sin Scan, recurso ajeno ->404.
- [ ] `E046-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / CONTRIBUTION#<id>, GOAL#<id>, MOVEMENT#<UTC>#<id>; localizador y transacción con IDEM#.
- [ ] `E046-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E046-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E046-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E046-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E046-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E046-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E046-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E046-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E046-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E046-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E046-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E046-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E046-CT06` Montos exactos en centavos: 0, mínimo, máximo, decimal inválido, cambio de mes y zona IANA.
