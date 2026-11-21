# Tareas planificadas — E046

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E046-T01` Revisar OpenAPI `operationId=nexum_e046` vs `GET /api/v1/goal-contributions/{contributionId}` y contrato de dominio.
- [ ] `E046-T02` Implementar request/response DTO estrictos `ninguno` / `Contribution`; rechazo de campos desconocidos.
- [ ] `E046-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E046-T04` Implementar caso de uso y invariantes: Localizador por ID bajo PK del usuario, sin Scan, recurso ajeno ->404.
- [ ] `E046-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / CONTRIBUTION#<id>, GOAL#<id>, MOVEMENT#<UTC>#<id>; localizador y transacción con IDEM#.
- [ ] `E046-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E046-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E046-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E046-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E046-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E046-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E046-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E046-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E046-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E046-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E046-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E046-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E046-CT06` Montos exactos en centavos: 0, mínimo, máximo, decimal inválido, cambio de mes y zona IANA.


## Cierre local E046 (2026-11-19)

- **Implementado:** GET /api/v1/goal-contributions/{contributionId} con ownership, 7 campos exactos y 404 homogeneo.
- **Wiring:** `cmd/server/e046.go`; base compartida #130.
- **Pruebas:** `go build`, `go vet`, `go test ./...` PASS local; pruebas de transporte E046 PASS.
- **PR:** pendiente de abrir; sin merge ni AWS.

<!-- NOTA_VIGENCIA_AUDITORIA_SEMANTICA -->
### Nota de vigencia - auditoria semantica (2026-11-21)

- Estado: `AUDITADA_SEMANTICA_CON_OBSERVACIONES`.
- Hallazgo: PR #135 cerrado sin merge; codigo por #139; tasks decia 'PR pendiente'.
- Los bloques anteriores de este archivo conservan el estado historico del endpoint y no se reescriben.
- Evidencia: `memoria.md:2238` (13/13 en v0.19.2) y `GATE_H08E_OLEADA_3.md` §12.1; CI de main run 44838845962.
- Norma aplicable: RFC-022 v1.2.1 (APPROVED). Release inicial `v0.19.0+3baa15840a2e`; release vigente `v0.19.2+3098f379d7ab`.
- Artefacto HTTP individual por endpoint: `EVIDENCIA_FUNCIONAL_INDIVIDUAL_NO_LOCALIZADA`; la certificacion consta de forma agregada en memoria y Gate.
- Cualquier contradiccion entre bloques anteriores y esta nota se resuelve a favor de esta nota.
