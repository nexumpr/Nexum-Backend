# Tareas planificadas — E012

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E012-T01` Revisar OpenAPI `operationId=nexum_e012` vs `GET /api/v1/users/me` y contrato de dominio.
- [ ] `E012-T02` Implementar request/response DTO estrictos `ninguno` / `UserProfile`; rechazo de campos desconocidos.
- [ ] `E012-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E012-T04` Implementar caso de uso y invariantes: Obtención por sujeto JWT, no aceptar userId suministrado por cliente.
- [ ] `E012-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / PROFILE, CONSENT#*, ONBOARDING, PREFERENCES; privacidad y revocación transversal.
- [ ] `E012-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E012-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E012-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E012-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E012-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E012-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E012-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E012-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E012-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E012-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E012-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E012-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
