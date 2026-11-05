# Tareas planificadas — E002

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E002-T01` Revisar OpenAPI `operationId=nexum_e002` vs `POST /api/v1/auth/login` y contrato de dominio.
- [ ] `E002-T02` Implementar request/response DTO estrictos `LoginRequest` / `AuthTokens`; rechazo de campos desconocidos.
- [ ] `E002-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E002-T04` Implementar caso de uso y invariantes: Verificar contraseña en tiempo constante; refresh opaco hasheado y sesión persistida; respuesta no revela existencia de correo.
- [ ] `E002-T05` Implementar adaptador DynamoDB y patrón: Email-reserva / USER#id PROFILE y SESSION#id / SESSIONHASH#digest; token hash con TTL y expiración funcional.
- [ ] `E002-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E002-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E002-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E002-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E002-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E002-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E002-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E002-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E002-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E002-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E002-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E002-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
