# Tareas planificadas — E047

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E047-T01` Revisar OpenAPI `operationId=nexum_e047` vs `POST /api/v1/goal-contributions/{contributionId}/reverse` y contrato de dominio.
- [ ] `E047-T02` Implementar request/response DTO estrictos `ReverseContributionRequest` / `Contribution`; rechazo de campos desconocidos.
- [ ] `E047-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E047-T04` Implementar caso de uso y invariantes: Reversión exacta una vez, compensación transaccional atómica, nunca borrar historia financiera.
- [ ] `E047-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / CONTRIBUTION#<id>, GOAL#<id>, MOVEMENT#<UTC>#<id>; localizador y transacción con IDEM#.
- [ ] `E047-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E047-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E047-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E047-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E047-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E047-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E047-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E047-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E047-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E047-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E047-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E047-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E047-CT06` Montos exactos en centavos: 0, mínimo, máximo, decimal inválido, cambio de mes y zona IANA.
- [ ] `E047-CT07` Repetición idéntica de Idempotency-Key -> respuesta estable; payload diferente con misma key -> 409; carrera multiinstancia.
- [ ] `E047-CT08` Suma de asignaciones exacta; objetivo ajeno, metas repetidas, >12 metas, doble reversa y transacción cancelada sin efectos.
