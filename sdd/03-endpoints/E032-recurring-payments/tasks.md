# Tareas planificadas — E032

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E032-T01` Revisar OpenAPI `operationId=nexum_e032` vs `GET /api/v1/recurring-payments` y contrato de dominio.
- [ ] `E032-T02` Implementar request/response DTO estrictos `ninguno` / `PageOfRecurringPayment`; rechazo de campos desconocidos.
- [ ] `E032-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E032-T04` Implementar caso de uso y invariantes: Reglas configuradas, no pagos efectuados; filtrar por estado, no duplicar movimientos.
- [ ] `E032-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / RECURRING#<id>; transacción de pago con MOVEMENT#<UTC>#<id> e idempotencia.
- [ ] `E032-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E032-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E032-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E032-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E032-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E032-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E032-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E032-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E032-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E032-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E032-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E032-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E032-CT06` Montos exactos en centavos: 0, mínimo, máximo, decimal inválido, cambio de mes y zona IANA.
- [ ] `E032-CT07` Paginación 25/100, filtro modificado con cursor antiguo -> 400, lectura sin Scan, sin duplicados en páginas.

## Tareas de la enmienda (A8) — PENDIENTES DE EJECUCION

- [ ] Implementar filtro `type` (income|expense|saving) aplicado **antes** de paginar.
- [ ] Anadir la validacion de entrada correspondiente y su mensaje 422.
- [ ] Cubrir con prueba positiva y negativa.
- [ ] Verificar por HTTP real tras el despliegue autorizado.

## Oleada 2 (2026-11-19) — E032 `GET /api/v1/recurring-payments`

| Campo | Valor |
|---|---|
| Estado | IMPLEMENTED · PR_OPEN · CI_GREEN (12/12) · MERGED=YES · AWS_DEPLOYED=NO · HTTPS_VERIFIED=NO · FUNCTIONAL_VERIFIED=NO |
| PR | #109 · HEAD `b9efc0297b5b14dedadb06c34f7e7a9a1140e3a1` |
| Claves DynamoDB | USER#<id> con begins_with(RECURRING#) |
| Decisiones | Cursor firmado ligado a filtros active/type y usuario; paginacion sin duplicados. |
| Errores y causa raiz | SA4010 por slice de ids no usado (se anadio asercion) y U1000 del helper attrB, corregidos en 701f230. |
| Pruebas | `go build`, `go vet` y suites de aplicacion/adaptador/transporte en verde; donde Windows Application Control bloqueo el binario se declara NOT_RUN_LOCAL y la autoridad es el CI Linux. |
| Contrato Android | Ruta y esquema segun `sdd/03-endpoints/openapi/nexum-api-v1.json`; errores con `ErrorEnvelope` y `traceId`. |
| Pendiente | Merge autorizado, release agrupada y verificacion HTTP real. |
