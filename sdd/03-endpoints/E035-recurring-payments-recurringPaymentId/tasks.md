# Tareas planificadas — E035

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E035-T01` Revisar OpenAPI `operationId=nexum_e035` vs `PATCH /api/v1/recurring-payments/{recurringPaymentId}` y contrato de dominio.
- [ ] `E035-T02` Implementar request/response DTO estrictos `RecurringPatch` / `RecurringPayment`; rechazo de campos desconocidos.
- [ ] `E035-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E035-T04` Implementar caso de uso y invariantes: If-Match; cambios no alteran historial de pagos ejecutados.
- [ ] `E035-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / RECURRING#<id>; transacción de pago con MOVEMENT#<UTC>#<id> e idempotencia.
- [ ] `E035-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E035-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E035-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E035-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E035-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E035-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E035-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E035-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E035-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E035-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E035-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E035-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E035-CT06` Montos exactos en centavos: 0, mínimo, máximo, decimal inválido, cambio de mes y zona IANA.
- [ ] `E035-CT07` If-Match omitido -> 428; ETag viejo -> 412; carrera de versiones no pierde actualizaciones.
