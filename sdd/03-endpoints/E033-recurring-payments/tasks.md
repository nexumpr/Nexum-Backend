# Tareas planificadas — E033

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E033-T01` Revisar OpenAPI `operationId=nexum_e033` vs `POST /api/v1/recurring-payments` y contrato de dominio.
- [ ] `E033-T02` Implementar request/response DTO estrictos `RecurringCreate` / `RecurringPayment`; rechazo de campos desconocidos.
- [ ] `E033-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E033-T04` Implementar caso de uso y invariantes: Monto positivo, frecuencia válida y fecha siguiente consistente; crear regla sin generar pago.
- [ ] `E033-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / RECURRING#<id>; transacción de pago con MOVEMENT#<UTC>#<id> e idempotencia.
- [ ] `E033-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E033-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E033-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E033-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E033-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E033-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E033-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E033-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E033-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E033-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E033-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E033-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E033-CT06` Montos exactos en centavos: 0, mínimo, máximo, decimal inválido, cambio de mes y zona IANA.
- [ ] `E033-CT07` Repetición idéntica de Idempotency-Key -> respuesta estable; payload diferente con misma key -> 409; carrera multiinstancia.

## Tareas de la enmienda (A8) — PENDIENTES DE EJECUCION

- [ ] Implementar `type`, `startDate` y `dueDay` en request y respuesta.
- [ ] Anadir la validacion de entrada correspondiente y su mensaje 422.
- [ ] Cubrir con prueba positiva y negativa.
- [ ] Verificar por HTTP real tras el despliegue autorizado.

## Oleada 2 (2026-11-19) — E033 `POST /api/v1/recurring-payments`

| Campo | Valor |
|---|---|
| Estado | IMPLEMENTED · PR_OPEN · CI_GREEN (12/12) · MERGED=YES · AWS_DEPLOYED=NO · HTTPS_VERIFIED=NO · FUNCTIONAL_VERIFIED=NO |
| PR | #107 · HEAD `ed70d395df456c5af40822b17e43423478d22364` |
| Claves DynamoDB | USER#<id>/RECURRING#<id> + IDEM# |
| Decisiones | RFC-021: acepta 9 frecuencias, persiste y emite 7 canonicas (biweekly->fortnightly, yearly->annual); Idempotency-Key por usuario; no genera pagos (E037 fuera de alcance). |
| Errores y causa raiz | U1000 por helper de prueba sin uso, corregido en d6294b5. |
| Pruebas | `go build`, `go vet` y suites de aplicacion/adaptador/transporte en verde; donde Windows Application Control bloqueo el binario se declara NOT_RUN_LOCAL y la autoridad es el CI Linux. |
| Contrato Android | Ruta y esquema segun `sdd/03-endpoints/openapi/nexum-api-v1.json`; errores con `ErrorEnvelope` y `traceId`. |
| Pendiente | Merge autorizado, release agrupada y verificacion HTTP real. |
