# Tareas planificadas — E050

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E050-T01` Revisar OpenAPI `operationId=nexum_e050` vs `GET /api/v1/budgets/usage` y contrato de dominio.
- [ ] `E050-T02` Implementar request/response DTO estrictos `ninguno` / `PageOfBudgetUsage`; rechazo de campos desconocidos.
- [ ] `E050-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E050-T04` Implementar caso de uso y invariantes: Egresos confirmados por periodo/categoría, excluye anulados y duplicados; no Scan.
- [ ] `E050-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / BUDGET#<YYYY-MM>#<categoryId> + query movimientos por periodo.
- [ ] `E050-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E050-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E050-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E050-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E050-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E050-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E050-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E050-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E050-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E050-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E050-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E050-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E050-CT06` Montos exactos en centavos: 0, mínimo, máximo, decimal inválido, cambio de mes y zona IANA.
