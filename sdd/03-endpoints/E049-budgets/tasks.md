# Tareas planificadas — E049

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E049-T01` Revisar OpenAPI `operationId=nexum_e049` vs `PUT /api/v1/budgets` y contrato de dominio.
- [ ] `E049-T02` Implementar request/response DTO estrictos `BudgetReplace` / `PageOfBudget`; rechazo de campos desconocidos.
- [ ] `E049-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E049-T04` Implementar caso de uso y invariantes: PUT sustituye conjunto del periodo bajo control de versión; sin valores negativos ni superposición.
- [ ] `E049-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / BUDGET#<YYYY-MM>#<categoryId> + query movimientos por periodo.
- [ ] `E049-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E049-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E049-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E049-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E049-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E049-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E049-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E049-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E049-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E049-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E049-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E049-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E049-CT06` Montos exactos en centavos: 0, mínimo, máximo, decimal inválido, cambio de mes y zona IANA.
- [ ] `E049-CT07` If-Match omitido -> 428; ETag viejo -> 412; carrera de versiones no pierde actualizaciones.
