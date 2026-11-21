# Tareas planificadas — E026

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E026-T01` Revisar OpenAPI `operationId=nexum_e026` vs `GET /api/v1/financial-summary` y contrato de dominio.
- [ ] `E026-T02` Implementar request/response DTO estrictos `ninguno` / `FinancialSummary`; rechazo de campos desconocidos.
- [ ] `E026-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E026-T04` Implementar caso de uso y invariantes: Calcular por período y moneda sin mezclar ingreso base, gasto y aportación como doble salida; coherente con movimientos.
- [ ] `E026-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / INCOME, MOVEMENT#<UTC>#<id>; periodo y proyección financiera versionada.
- [ ] `E026-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E026-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E026-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E026-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E026-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E026-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E026-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E026-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E026-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E026-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E026-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E026-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E026-CT06` Montos exactos en centavos: 0, mínimo, máximo, decimal inválido, cambio de mes y zona IANA.

## Cierre local E026 (2026-11-19)

- **Implementado:** `GET /api/v1/financial-summary` con semántica RFC-022 v1.2.1: `income`/`expenses` realizados, `savings` = movimientos `saving` independientes + aportaciones `active`, `net = income − expenses − savings`, sin `availableEstimated`.
- **Persistencia:** `Query` por rango de SK sobre `MOVEMENT#` y `CONTRIBUTION#`, `GetItem` de `PREFERENCES`; sin `Scan`; topes de volumen que fallan cerrado (503) en lugar de sumar parcialmente.
- **Wiring:** `cmd/server/e026.go` sobre el registro de Oleada 3; handler con Bearer + sesión, periodo por zona IANA y 400/401/503.
- **Pruebas reales:** `go build`, `go vet`, `go test ./... -count=1` PASS; casos de ahorro independiente E028/E037, aportación activa/revertida, no doble conteo, zona horaria, periodo inválido y dos usuarios.
- **PR:** apilado sobre #124; pendiente CI Linux y certificación HTTPS.
- **Sin `availableEstimated`.** No se declara `AWS_VERIFIED`.


## Hotfix 2026-11-20 — consulta DynamoDB

- Causa raiz confirmada en AWS: `KeyConditionExpression` con `#sk >= :from AND #sk < :to`; DynamoDB solo admite una condicion de rango por clave (`ValidationException`), lo que producia 503.
- Correccion: `BETWEEN :from AND :to` con descarte explicito del borde superior para conservar el intervalo `[from, to)` y paginacion por `LastEvaluatedKey`.
- Prueba de regresion: `tests/unit/dynamo/query_condition_regression_test.go` captura la `QueryInput` efectiva, exige `BETWEEN` y comprueba que el borde superior no se contabiliza.
- Estado: corregido en rama de hotfix; AWS pendiente de nueva release y certificacion.


## Verificación AWS v0.19.2 (2026-11-21)

- Corregido y desplegado en `v0.19.2+3098f379d7ab`; HTTPS real y DynamoDB consistente.
- Evidencia: `/health/live` 200, `/health/ready` 200; ASG 2/2/2; dos targets healthy.
