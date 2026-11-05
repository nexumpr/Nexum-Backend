# Tareas planificadas — E008

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E008-T01` Revisar OpenAPI `operationId=nexum_e008` vs `GET /api/v1/auth/sessions` y contrato de dominio.
- [ ] `E008-T02` Implementar request/response DTO estrictos `ninguno` / `PageOfSession`; rechazo de campos desconocidos.
- [ ] `E008-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E008-T04` Implementar caso de uso y invariantes: Solo sesiones titular; IP y agente de usuario minimizados; nunca exponer hashes ni credenciales.
- [ ] `E008-T05` Implementar adaptador DynamoDB y patrón: Email-reserva / USER#id PROFILE y SESSION#id / SESSIONHASH#digest; token hash con TTL y expiración funcional.
- [ ] `E008-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E008-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E008-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E008-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E008-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E008-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E008-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E008-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E008-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E008-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E008-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E008-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E008-CT06` Paginación 25/100, filtro modificado con cursor antiguo -> 400, lectura sin Scan, sin duplicados en páginas.
