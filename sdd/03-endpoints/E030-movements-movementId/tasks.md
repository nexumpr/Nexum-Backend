# Tareas planificadas — E030

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E030-T01` Revisar OpenAPI `operationId=nexum_e030` vs `PATCH /api/v1/movements/{movementId}` y contrato de dominio.
- [ ] `E030-T02` Implementar request/response DTO estrictos `MovementPatch` / `Movement`; rechazo de campos desconocidos.
- [ ] `E030-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E030-T04` Implementar caso de uso y invariantes: If-Match requerido; prohibir cambiar vínculos a aportaciones o pagos liquidados; recalcular proyecciones.
- [ ] `E030-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / MOVEMENT#<UTC>#<id>; MOVEMENT_ID#<id> locator; GSI1 para filtros por tipo/fecha.
- [ ] `E030-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E030-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E030-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E030-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E030-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E030-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E030-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E030-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E030-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E030-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E030-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E030-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E030-CT06` Montos exactos en centavos: 0, mínimo, máximo, decimal inválido, cambio de mes y zona IANA.
- [ ] `E030-CT07` If-Match omitido -> 428; ETag viejo -> 412; carrera de versiones no pierde actualizaciones.
