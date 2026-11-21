# Tareas planificadas — E042

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E042-T01` Revisar OpenAPI `operationId=nexum_e042` vs `DELETE /api/v1/goals/{goalId}` y contrato de dominio.
- [ ] `E042-T02` Implementar request/response DTO estrictos `ninguno` / `ninguno`; rechazo de campos desconocidos.
- [ ] `E042-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E042-T04` Implementar caso de uso y invariantes: No eliminar historial económico; archivar y preservar/revertir aportaciones según regla aprobada.
- [ ] `E042-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / GOAL#<id>; CAS sobre version, historial de contribuciones separado.
- [ ] `E042-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E042-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E042-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E042-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E042-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E042-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E042-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E042-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E042-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E042-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E042-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E042-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E042-CT06` Montos exactos en centavos: 0, mínimo, máximo, decimal inválido, cambio de mes y zona IANA.


## Cierre local E042 (2026-11-19)

- **Implementado:** `DELETE /api/v1/goals/{goalId}` con alternativa B de RFC-022 v1.2.1: 409 si `activeContributions > 0`, reversión explícita por E047, tombstone `deleted=true/status=archived/savedAmount=openingSaved/version+1`, 204 sin cuerpo e idempotencia.
- **Integridad:** E040/E041 tratan `deleted=true` como 404; E038 excluye tombstones; E031/E027/E029 sin cambios.
- **Wiring:** `cmd/server/e042.go`; handler DELETE; Bearer + sesión.
- **Pruebas:** `go build`, `go vet`, `go test ./...` PASS; borrado válido, 409 con aportaciones, segundo DELETE idempotente y 404.
- **PR:** pendiente de abrir; sin merge ni AWS.


## Hotfix 2026-11-20 — E042 `:one` en tombstone

- Causa raiz confirmada en AWS: la `UpdateExpression` de `DeleteGoal` usa `ADD #version :one`, pero `ExpressionAttributeValues` no declaraba `:one`; DynamoDB devolvia `ValidationException` y el adaptador lo traducia a 503 tras la reversion de la aportacion.
- Correccion: declarar `:one` como Number. La condicion de version y `activeContributions` se conserva.
- Prueba de regresion: `TestE042UpdateItemDeclaraTodosLosValores` captura el `UpdateItemInput` real y exige que todos los marcadores tengan valor.
- Estado: corregido en rama de hotfix; AWS pendiente de release y certificacion.


## Verificación AWS v0.19.2 (2026-11-21)

- Corregido y desplegado en `v0.19.2+3098f379d7ab`; HTTPS real y DynamoDB consistente.
- Evidencia: `/health/live` 200, `/health/ready` 200; ASG 2/2/2; dos targets healthy.
