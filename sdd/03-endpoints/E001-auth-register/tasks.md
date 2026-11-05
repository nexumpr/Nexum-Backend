# Tareas planificadas — E001

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E001-T01` Revisar OpenAPI `operationId=nexum_e001` vs `POST /api/v1/auth/register` y contrato de dominio.
- [ ] `E001-T02` Implementar request/response DTO estrictos `RegisterRequest` / `UserProfile`; rechazo de campos desconocidos.
- [ ] `E001-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E001-T04` Implementar caso de uso y invariantes: Email normalizado único con reserva transaccional; contraseña Argon2id; estado email no verificado; política vigente aceptada.
- [ ] `E001-T05` Implementar adaptador DynamoDB y patrón: Email-reserva / USER#id PROFILE y SESSION#id / SESSIONHASH#digest; token hash con TTL y expiración funcional.
- [ ] `E001-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E001-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E001-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E001-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E001-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E001-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E001-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E001-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E001-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E001-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E001-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E001-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E001-CT06` Repetición idéntica de Idempotency-Key -> respuesta estable; payload diferente con misma key -> 409; carrera multiinstancia.
