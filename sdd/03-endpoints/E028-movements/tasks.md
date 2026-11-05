# Tareas planificadas — E028

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E028-T01` Revisar OpenAPI `operationId=nexum_e028` vs `POST /api/v1/movements` y contrato de dominio.
- [ ] `E028-T02` Implementar request/response DTO estrictos `MovementCreate` / `Movement`; rechazo de campos desconocidos.
- [ ] `E028-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E028-T04` Implementar caso de uso y invariantes: Una única contabilización por Idempotency-Key; categorías válidas y monto positivo; transacción con proyecciones si aplica.
- [ ] `E028-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / MOVEMENT#<UTC>#<id>; MOVEMENT_ID#<id> locator; GSI1 para filtros por tipo/fecha.
- [ ] `E028-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E028-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E028-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E028-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E028-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E028-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E028-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E028-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E028-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E028-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E028-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E028-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E028-CT06` Montos exactos en centavos: 0, mínimo, máximo, decimal inválido, cambio de mes y zona IANA.
- [ ] `E028-CT07` Repetición idéntica de Idempotency-Key -> respuesta estable; payload diferente con misma key -> 409; carrera multiinstancia.
