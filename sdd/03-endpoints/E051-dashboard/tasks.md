# Tareas planificadas — E051

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E051-T01` Revisar OpenAPI `operationId=nexum_e051` vs `GET /api/v1/dashboard` y contrato de dominio.
- [ ] `E051-T02` Implementar request/response DTO estrictos `ninguno` / `Dashboard`; rechazo de campos desconocidos.
- [ ] `E051-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E051-T04` Implementar caso de uso y invariantes: Agregados consistentes y versionados, cache privada Redis invalidada ante mutación financiera.
- [ ] `E051-T05` Implementar adaptador DynamoDB y patrón: Query por usuario y periodo + proyecciones versionadas / Redis privado; sin Scan; ML/IA según contrato de proveedor.
- [ ] `E051-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E051-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E051-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E051-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E051-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E051-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E051-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E051-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E051-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E051-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E051-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E051-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E051-CT06` Montos exactos en centavos: 0, mínimo, máximo, decimal inválido, cambio de mes y zona IANA.

## Cierre local E051 (2026-11-19)

- **Implementado:** `GET /api/v1/dashboard` con `summary` de E026, metas activas no borradas (máx. 100) y 10 movimientos recientes; `asOf` coherente con el resumen.
- **Sin caché Redis:** consultas directas a DynamoDB (`Query` por partición, sin `Scan`). Sin cantidades ficticias.
- **Wiring:** `cmd/server/e051.go` sobre el registro de Oleada 3; handler con Bearer + sesión.
- **Pruebas reales:** `go build`, `go vet`, `go test ./... -count=1` PASS; aplicación y transporte; DTO `Dashboard` con arrays no nulos.
- **PR:** apilado sobre #125; pendiente CI Linux y certificación HTTPS.
- **Sin `availableEstimated`.** No se declara `AWS_VERIFIED`.


## Hotfix 2026-11-20 — diagnostico E051

- El 503 de E051 provenia del resumen financiero compartido (E026) y de la misma consulta invalida; `DashboardStore` usa `begins_with` y no requiere cambio propio.
- Sin cambio de codigo en E051. Se verificara en la certificacion posterior a la release correctiva.


## Verificación AWS v0.19.2 (2026-11-21)

- Corregido y desplegado en `v0.19.2+3098f379d7ab`; HTTPS real y DynamoDB consistente.
- Evidencia: `/health/live` 200, `/health/ready` 200; ASG 2/2/2; dos targets healthy.

<!-- NOTA_VIGENCIA_AUDITORIA_SEMANTICA -->
### Nota de vigencia - auditoria semantica (2026-11-21)

- Estado: `DISCREPANCIA_DOCUMENTAL_REGISTRADA`.
- Hallazgo: `spec.md`/`plan.md`/`tasks.md` exigen cache Redis; RFC-022 D-W3-07 y el codigo calculan directo desde DynamoDB. Discrepancia normativa registrada.
- Los bloques anteriores de este archivo conservan el estado historico del endpoint y no se reescriben.
- Evidencia: `memoria.md:2238` (13/13 en v0.19.2) y `GATE_H08E_OLEADA_3.md` §12.1; CI de main run 44838845962.
- Norma aplicable: RFC-022 v1.2.1 (APPROVED). Release inicial `v0.19.0+3baa15840a2e`; release vigente `v0.19.2+3098f379d7ab`.
- Artefacto HTTP individual por endpoint: `EVIDENCIA_FUNCIONAL_INDIVIDUAL_NO_LOCALIZADA`; la certificacion consta de forma agregada en memoria y Gate.
- Cualquier contradiccion entre bloques anteriores y esta nota se resuelve a favor de esta nota.

> **RFC-026 (2026-11-25):** la caché Redis de respuesta queda descartada por RFC-022 D-W3-07; Redis se conserva solo como limitador de cuota. Ver `sdd/_control/H08/RFC-026-ENMIENDA_E051_CACHE.md. Esta nota no altera el cuerpo aprobado; el RFC es el instrumento de enmienda.
