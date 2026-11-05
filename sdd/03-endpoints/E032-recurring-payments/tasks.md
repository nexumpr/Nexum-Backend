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
