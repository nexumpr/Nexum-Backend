# Tareas planificadas — E048

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E048-T01` Revisar OpenAPI `operationId=nexum_e048` vs `GET /api/v1/budgets` y contrato de dominio.
- [ ] `E048-T02` Implementar request/response DTO estrictos `ninguno` / `PageOfBudget`; rechazo de campos desconocidos.
- [ ] `E048-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E048-T04` Implementar caso de uso y invariantes: Query por periodo yyyy-MM; límites por categoría y moneda.
- [ ] `E048-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / BUDGET#<YYYY-MM>#<categoryId> + query movimientos por periodo.
- [ ] `E048-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E048-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E048-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E048-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E048-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E048-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E048-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E048-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E048-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E048-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E048-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E048-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E048-CT06` Montos exactos en centavos: 0, mínimo, máximo, decimal inválido, cambio de mes y zona IANA.
- [ ] `E048-CT07` Paginación 25/100, filtro modificado con cursor antiguo -> 400, lectura sin Scan, sin duplicados en páginas.


## Cierre local E048 (2026-11-19)

- **Implementado:** `GET /api/v1/budgets` sobre `BUDGETSET#<period>` con ETag por periodo, paginacion `limit`/`cursor`, ownership y lectura sin Scan.
- **Wiring:** `cmd/server/e048.go` sobre el registro de Oleada 3; Bearer + sesion; cursor HMAC derivado.
- **Pruebas:** `go build`, `go vet`, `go test ./... -count=1` PASS; 0/1/25/50 categorias, periodo vacio, paginacion y sin periodo.
- **PR:** pendiente de abrir; sin merge ni AWS.
