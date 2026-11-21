# Tareas planificadas — E037

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E037-T01` Revisar OpenAPI `operationId=nexum_e037` vs `POST /api/v1/recurring-payments/{recurringPaymentId}/payments` y contrato de dominio.
- [ ] `E037-T02` Implementar request/response DTO estrictos `RecurringPaymentCreate` / `Movement`; rechazo de campos desconocidos.
- [ ] `E037-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E037-T04` Implementar caso de uso y invariantes: Idempotency-Key y transacción: movimiento de egreso + vínculo a regla + control de duplicado por fecha.
- [ ] `E037-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / RECURRING#<id>; transacción de pago con MOVEMENT#<UTC>#<id> e idempotencia.
- [ ] `E037-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E037-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E037-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E037-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E037-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E037-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E037-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E037-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E037-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E037-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E037-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E037-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E037-CT06` Montos exactos en centavos: 0, mínimo, máximo, decimal inválido, cambio de mes y zona IANA.
- [ ] `E037-CT07` Repetición idéntica de Idempotency-Key -> respuesta estable; payload diferente con misma key -> 409; carrera multiinstancia.

## Cierre local E037 (2026-11-19)

- **Implementado:** `POST /api/v1/recurring-payments/{recurringPaymentId}/payments` con DTO estricto, `Idempotency-Key`, transacción de 5 acciones (movimiento + puntero + `RECURRINGPAY#` + idempotencia + `ConditionCheck` de regla activa), replay y 409 por fecha local duplicada.
- **Categorías:** `expense→otros`, `income→ingreso`, `saving→ahorro` (catálogo E022; `recurring` no existe y no se inventa). `description` = nombre de la regla y `recurringPaymentId` conserva el vínculo.
- **Wiring:** `cmd/server/registro_oleada3.go` y `cmd/server/e037.go`; `main.go` construye `dependenciasOleada3`.
- **Pruebas reales:** `go build ./...`, `go vet ./...`, `go test ./... -count=1` PASS en local; suites de aplicación, adaptador y transporte; prueba de categorías del catálogo.
- **PR:** #124, HEAD `c4f7ee3dc292c91cac29ad2628c09d7401321f0d` (más la corrección U1000 y las pruebas de registro).
- **Pendiente:** CI Linux, certificación HTTPS y despliegue. No se declara `AWS_VERIFIED`.

<!-- NOTA_VIGENCIA_AUDITORIA_SEMANTICA -->
### Nota de vigencia - auditoria semantica (2026-11-21)

- Estado: `AUDITADA_SEMANTICA_CON_OBSERVACIONES`.
- Hallazgo: Evidencia positiva previa no localizada como artefacto individual; el codigo no cambio en v0.19.2 y la ruta esta montada.
- Los bloques anteriores de este archivo conservan el estado historico del endpoint y no se reescriben.
- Evidencia: `memoria.md:2238` (13/13 en v0.19.2) y `GATE_H08E_OLEADA_3.md` §12.1; CI de main run 44838845962.
- Norma aplicable: RFC-022 v1.2.1 (APPROVED). Release inicial `v0.19.0+3baa15840a2e`; release vigente `v0.19.2+3098f379d7ab`.
- Artefacto HTTP individual por endpoint: `EVIDENCIA_FUNCIONAL_INDIVIDUAL_NO_LOCALIZADA`; la certificacion consta de forma agregada en memoria y Gate.
- Cualquier contradiccion entre bloques anteriores y esta nota se resuelve a favor de esta nota.
