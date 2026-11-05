# Tareas planificadas — E039

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E039-T01` Revisar OpenAPI `operationId=nexum_e039` vs `POST /api/v1/goals` y contrato de dominio.
- [ ] `E039-T02` Implementar request/response DTO estrictos `GoalCreate` / `Goal`; rechazo de campos desconocidos.
- [ ] `E039-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E039-T04` Implementar caso de uso y invariantes: Monto objetivo positivo y fecha válida; ahorro inicial opcional 0 <= initialSavedAmount <= targetAmount, se registra como asiento de apertura atómico separado de aportación mensual.
- [ ] `E039-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / GOAL#<id>; CAS sobre version, historial de contribuciones separado.
- [ ] `E039-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E039-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E039-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E039-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E039-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E039-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E039-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E039-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E039-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E039-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E039-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E039-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E039-CT06` Montos exactos en centavos: 0, mínimo, máximo, decimal inválido, cambio de mes y zona IANA.
- [ ] `E039-CT07` Repetición idéntica de Idempotency-Key -> respuesta estable; payload diferente con misma key -> 409; carrera multiinstancia.
