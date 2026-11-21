# Tareas planificadas — E043

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E043-T01` Revisar OpenAPI `operationId=nexum_e043` vs `POST /api/v1/goal-contributions/preview` y contrato de dominio.
- [ ] `E043-T02` Implementar request/response DTO estrictos `ContributionPreviewRequest` / `ContributionPreview`; rechazo de campos desconocidos.
- [ ] `E043-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E043-T04` Implementar caso de uso y invariantes: Pura validación/calculo sin escrituras, mismas reglas que POST; centavos exactos, sin redondeos que pierdan unidades.
- [ ] `E043-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / CONTRIBUTION#<id>, GOAL#<id>, MOVEMENT#<UTC>#<id>; localizador y transacción con IDEM#.
- [ ] `E043-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E043-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E043-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E043-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E043-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E043-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E043-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E043-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E043-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E043-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E043-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E043-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E043-CT06` Montos exactos en centavos: 0, mínimo, máximo, decimal inválido, cambio de mes y zona IANA.
- [ ] `E043-CT07` Suma de asignaciones exacta; objetivo ajeno, metas repetidas, >12 metas, doble reversa y transacción cancelada sin efectos.

## Oleada 3 (2026-11-19) — E043 `POST /api/v1/goal-contributions/preview`

| Campo | Valor |
|---|---|
| Estado | IMPLEMENTED=YES · PR_OPEN=NO (pendiente de publicar) · CI_GREEN=PENDIENTE · MERGED=NO · AWS_DEPLOYED=NO · HTTPS_VERIFIED=NO · FUNCTIONAL_VERIFIED=NO |
| Rama | `feat/h08e-e043-preview`, base `feat/h08e-contributions-foundation` (`7759eed`) |
| Handler | `internal/transport/http/contributions_preview_handler.go` |
| Wiring | `cmd/server/e043.go` -> `registrarOleada3` (`POST /goal-contributions/preview`) |
| Reutilizacion | Fundacion compartida: `CadenaAportaciones`, `WriteContributionsError`, `MaxBodyAportaciones`, `CuotaAportacionesPorMinuto`. Sin duplicar el caso de uso ni el adaptador. |
| Decisiones | DTO cerrado; Bearer RS256 + estado autoritativo de sesion; cuota por usuario 60/min DENTRO de la cadena autenticada; E043 no escribe (solo `GetItem` de las metas). |
| Regla de suma | El servicio exige `sum allocations == amount` (`NormalizeAllocations(..., exactTotal=true)`); una meta sin capacidad devuelve `canCommit=false` con el remanente. |
| Defecto corregido antes del primer commit | La composicion inicial aplicaba `LimitarPorUsuario` por fuera de `CadenaAportaciones`, donde aun no hay identidad en el contexto: la cuota quedaba desactivada en silencio. Ahora la cuota envuelve al handler ya autenticado. Cubierto por `TestE043HTTPPreviewCuota`. |
| Pruebas | `go build ./...` y `go vet ./...` PASS local; `go test ./tests/unit/transport/... -run TestE043 -count=1` PASS (200 de contrato, 422 de suma/duplicado/divisa, 400 de DTO, 401 de auth, 404, 503, 429 y fail-open de Redis). |
| Pendiente | CI del PR, Gate de merge por SHA40, publicacion y verificacion HTTPS real. |

> **Registro operativo:** las casillas `[ ]` del plan historico H06 no se marcan retroactivamente; la evidencia vigente es este bloque.

<!-- NOTA_VIGENCIA_AUDITORIA_SEMANTICA -->
### Nota de vigencia - auditoria semantica (2026-11-21)

- Estado: `AUDITADA_SEMANTICA_CON_OBSERVACIONES`.
- Hallazgo: `tasks.md` conservaba MERGED=NO y AWS_DEPLOYED=NO; ficha desactualizada.
- Los bloques anteriores de este archivo conservan el estado historico del endpoint y no se reescriben.
- Evidencia: `memoria.md:2238` (13/13 en v0.19.2) y `GATE_H08E_OLEADA_3.md` §12.1; CI de main run 44838845962.
- Norma aplicable: RFC-022 v1.2.1 (APPROVED). Release inicial `v0.19.0+3baa15840a2e`; release vigente `v0.19.2+3098f379d7ab`.
- Artefacto HTTP individual por endpoint: `EVIDENCIA_FUNCIONAL_INDIVIDUAL_NO_LOCALIZADA`; la certificacion consta de forma agregada en memoria y Gate.
- Cualquier contradiccion entre bloques anteriores y esta nota se resuelve a favor de esta nota.
