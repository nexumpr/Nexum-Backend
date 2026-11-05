# Tareas planificadas — E010

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E010-T01` Revisar OpenAPI `operationId=nexum_e010` vs `POST /api/v1/auth/email-verification/request` y contrato de dominio.
- [ ] `E010-T02` Implementar request/response DTO estrictos `ninguno` / `Accepted`; rechazo de campos desconocidos.
- [ ] `E010-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E010-T04` Implementar caso de uso y invariantes: Reenvío limitado con respuesta uniforme; token un uso, expiración y no exposición en logs.
- [ ] `E010-T05` Implementar adaptador DynamoDB y patrón: Email-reserva / USER#id PROFILE y SESSION#id / SESSIONHASH#digest; token hash con TTL y expiración funcional.
- [ ] `E010-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E010-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E010-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E010-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E010-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E010-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E010-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E010-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E010-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E010-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E010-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E010-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
