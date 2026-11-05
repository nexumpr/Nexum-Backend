# Tareas planificadas — E025

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E025-T01` Revisar OpenAPI `operationId=nexum_e025` vs `PUT /api/v1/income` y contrato de dominio.
- [ ] `E025-T02` Implementar request/response DTO estrictos `IncomeUpdate` / `Income`; rechazo de campos desconocidos.
- [ ] `E025-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E025-T04` Implementar caso de uso y invariantes: PUT idempotente con versionado; almacenar cents int64, currency MXN y frecuencia explícita.
- [ ] `E025-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / INCOME, MOVEMENT#<UTC>#<id>; periodo y proyección financiera versionada.
- [ ] `E025-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E025-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E025-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E025-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E025-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E025-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E025-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E025-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E025-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E025-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E025-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E025-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E025-CT06` Montos exactos en centavos: 0, mínimo, máximo, decimal inválido, cambio de mes y zona IANA.
- [ ] `E025-CT07` If-Match omitido -> 428; ETag viejo -> 412; carrera de versiones no pierde actualizaciones.
