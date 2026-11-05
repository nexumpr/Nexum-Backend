# Tareas planificadas — E031

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E031-T01` Revisar OpenAPI `operationId=nexum_e031` vs `DELETE /api/v1/movements/{movementId}` y contrato de dominio.
- [ ] `E031-T02` Implementar request/response DTO estrictos `ninguno` / `ninguno`; rechazo de campos desconocidos.
- [ ] `E031-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E031-T04` Implementar caso de uso y invariantes: Soft delete o reverso auditable; no permitir borrar registro ligado a aportaciones confirmadas sin reversa.
- [ ] `E031-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / MOVEMENT#<UTC>#<id>; MOVEMENT_ID#<id> locator; GSI1 para filtros por tipo/fecha.
- [ ] `E031-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E031-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E031-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E031-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E031-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E031-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E031-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E031-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E031-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E031-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E031-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E031-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E031-CT06` Montos exactos en centavos: 0, mínimo, máximo, decimal inválido, cambio de mes y zona IANA.
