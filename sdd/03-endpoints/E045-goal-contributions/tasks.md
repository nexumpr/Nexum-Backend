# Tareas planificadas — E045

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E045-T01` Revisar OpenAPI `operationId=nexum_e045` vs `GET /api/v1/goal-contributions` y contrato de dominio.
- [ ] `E045-T02` Implementar request/response DTO estrictos `ninguno` / `PageOfContribution`; rechazo de campos desconocidos.
- [ ] `E045-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E045-T04` Implementar caso de uso y invariantes: Query por usuario con cursor, ordenar newest-first y preservar vínculos.
- [ ] `E045-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / CONTRIBUTION#<id>, GOAL#<id>, MOVEMENT#<UTC>#<id>; localizador y transacción con IDEM#.
- [ ] `E045-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E045-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E045-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E045-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E045-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E045-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E045-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E045-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E045-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E045-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E045-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E045-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E045-CT06` Montos exactos en centavos: 0, mínimo, máximo, decimal inválido, cambio de mes y zona IANA.
- [ ] `E045-CT07` Paginación 25/100, filtro modificado con cursor antiguo -> 400, lectura sin Scan, sin duplicados en páginas.

## Tareas de la enmienda (A8) — PENDIENTES DE EJECUCION

- [ ] Implementar filtro `status` (active|reversed) aplicado **antes** de paginar.
- [ ] Anadir la validacion de entrada correspondiente y su mensaje 422.
- [ ] Cubrir con prueba positiva y negativa.
- [ ] Verificar por HTTP real tras el despliegue autorizado.

## Implementación local H08-E (Oleada 3) — 2026-11-19

> Evidencia de la rama `feat/h08e-e045-list`. No se reescriben las casillas históricas de especificación; esta sección registra sólo lo ejecutado.

| Tarea | Estado | Evidencia |
|---|---|---|
| `E045-T01` | HECHO | Contrato revisado: parámetros `limit`, `cursor`, `goalId`, `from`, `to` (el filtro `status` proviene de la enmienda A8) y respuesta `PageOfContribution` con `items`/`nextCursor`. |
| `E045-T02` | HECHO | Sin cuerpo de petición; respuesta construida desde `WireContribution` (siete campos) y `nextCursor` omitido cuando no hay más páginas. |
| `E045-T03` | HECHO | `Bearer` + estado autoritativo de sesión (`CadenaAportaciones`); `userId` derivado del sujeto; sin Bearer o sesión revocada → 401. |
| `E045-T04` | HECHO | `Service.List`: `Query` por usuario, orden newest-first, filtros antes de paginar, cursor HMAC ligado a usuario y a la firma exacta de filtros. |
| `E045-T05` | HECHO | Reutiliza el adaptador de la fundación (`ContributionsStore`): `Query` sobre `USER#<sub>` con `begins_with(CONTRIBUTION#)` / `BETWEEN`, sin `Scan`. |
| `E045-T06` | HECHO | Redis se usa sólo para la cuota por usuario; ninguna decisión de privilegio o propiedad depende de caché y el fallo del limitador no autoriza de más. |
| `E045-T07` | HECHO | `200/400/401/429/500/503`; `ETag` no aplica a un listado; sin clave de cursor → 503 (nunca paginación sin firma). |
| `E045-T08` | HECHO | `tests/unit/transport/contributions_list_test.go` (5 pruebas): contrato, paginación, filtros, validación, auth y fallo cerrado. |
| `E045-T09` | NOT_RUN | Requiere DynamoDB y HTTP reales: pendiente de Gate de despliegue. |
| `E045-T10` | HECHO (local) | Negativas de auth/ownership, cursores manipulados y de otro usuario, límites 1..100, `from>to`; concurrencia multiinstancia real `NOT_RUN`. |
| `E045-T11` | PENDIENTE_CI | `gofmt`, `go build`, `go vet` y `go test` locales PASS; el resultado de CI del PR se registra en el reporte de cierre. |
| `E045-T12` | NOT_RUN | Sin despliegue ni release; no se declara `AWS_VERIFIED`. |

**Enmienda A8 (`status`):** implementada — `NormalizeListQuery` valida `active|reversed` y el adaptador aplica `#status = :status` antes de paginar; cubierta por prueba positiva y negativa de transporte.

<!-- NOTA_VIGENCIA_AUDITORIA_SEMANTICA -->
### Nota de vigencia - auditoria semantica (2026-11-21)

- Estado: `AUDITADA_SEMANTICA_CON_OBSERVACIONES`.
- Hallazgo: PR #134 cerrado sin merge; codigo por #139; tasks conservaba T11/T12 pendientes.
- Los bloques anteriores de este archivo conservan el estado historico del endpoint y no se reescriben.
- Evidencia: `memoria.md:2238` (13/13 en v0.19.2) y `GATE_H08E_OLEADA_3.md` §12.1; CI de main run 44838845962.
- Norma aplicable: RFC-022 v1.2.1 (APPROVED). Release inicial `v0.19.0+3baa15840a2e`; release vigente `v0.19.2+3098f379d7ab`.
- Artefacto HTTP individual por endpoint: `EVIDENCIA_FUNCIONAL_INDIVIDUAL_NO_LOCALIZADA`; la certificacion consta de forma agregada en memoria y Gate.
- Cualquier contradiccion entre bloques anteriores y esta nota se resuelve a favor de esta nota.
