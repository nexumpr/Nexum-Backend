# Tareas planificadas — E014

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E014-T01` Revisar OpenAPI `operationId=nexum_e014` vs `DELETE /api/v1/users/me` y contrato de dominio.
- [ ] `E014-T02` Implementar request/response DTO estrictos `DeleteAccountRequest` / `DeletionReceipt`; rechazo de campos desconocidos.
- [ ] `E014-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E014-T04` Implementar caso de uso y invariantes: Marcar tombstone y revocar sesiones de inmediato; trabajo idempotente auditable para purga por lotes; respetar retenciones legales declaradas.
- [ ] `E014-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / PROFILE, CONSENT#*, ONBOARDING, PREFERENCES; privacidad y revocación transversal.
- [ ] `E014-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E014-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E014-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E014-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E014-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E014-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E014-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E014-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E014-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E014-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E014-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E014-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E014-CT06` Revocación/rotación en nodo A persiste y es observable inmediatamente en nodo B; token reutilizado rechazado.
