# Tareas planificadas — E044

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E044-T01` Revisar OpenAPI `operationId=nexum_e044` vs `POST /api/v1/goal-contributions` y contrato de dominio.
- [ ] `E044-T02` Implementar request/response DTO estrictos `ContributionCreate` / `Contribution`; rechazo de campos desconocidos.
- [ ] `E044-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E044-T04` Implementar caso de uso y invariantes: TransactWriteItems atómico: idempotencia + aporte + movimientos + metas; <=100 acciones y 4 MB, sin duplicar item en transacción.
- [ ] `E044-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / CONTRIBUTION#<id>, GOAL#<id>, MOVEMENT#<UTC>#<id>; localizador y transacción con IDEM#.
- [ ] `E044-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E044-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E044-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E044-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E044-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E044-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E044-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E044-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E044-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E044-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E044-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E044-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E044-CT06` Montos exactos en centavos: 0, mínimo, máximo, decimal inválido, cambio de mes y zona IANA.
- [ ] `E044-CT07` Repetición idéntica de Idempotency-Key -> respuesta estable; payload diferente con misma key -> 409; carrera multiinstancia.
- [ ] `E044-CT08` Suma de asignaciones exacta; objetivo ajeno, metas repetidas, >12 metas, doble reversa y transacción cancelada sin efectos.
