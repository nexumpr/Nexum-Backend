# Tareas planificadas — E004

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E004-T01` Revisar OpenAPI `operationId=nexum_e004` vs `POST /api/v1/auth/logout` y contrato de dominio.
- [ ] `E004-T02` Implementar request/response DTO estrictos `ninguno` / `ninguno`; rechazo de campos desconocidos.
- [ ] `E004-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E004-T04` Implementar caso de uso y invariantes: Revocación inmediata del refresh y sesión; JWT access corto controlado por sesión/revocación en Redis y DynamoDB.
- [ ] `E004-T05` Implementar adaptador DynamoDB y patrón: Email-reserva / USER#id PROFILE y SESSION#id / SESSIONHASH#digest; token hash con TTL y expiración funcional.
- [ ] `E004-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E004-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E004-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E004-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E004-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E004-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E004-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E004-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E004-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E004-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E004-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E004-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E004-CT06` Revocación/rotación en nodo A persiste y es observable inmediatamente en nodo B; token reutilizado rechazado.
