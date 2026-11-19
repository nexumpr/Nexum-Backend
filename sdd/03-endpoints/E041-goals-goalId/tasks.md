# Tareas planificadas — E041

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E041-T01` Revisar OpenAPI `operationId=nexum_e041` vs `PATCH /api/v1/goals/{goalId}` y contrato de dominio.
- [ ] `E041-T02` Implementar request/response DTO estrictos `GoalPatch` / `Goal`; rechazo de campos desconocidos.
- [ ] `E041-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E041-T04` Implementar caso de uso y invariantes: If-Match; no permitir reducir objetivo por debajo de ahorro acumulado sin regla explícita.
- [ ] `E041-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / GOAL#<id>; CAS sobre version, historial de contribuciones separado.
- [ ] `E041-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E041-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E041-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E041-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E041-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E041-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E041-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E041-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E041-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E041-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E041-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E041-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E041-CT06` Montos exactos en centavos: 0, mínimo, máximo, decimal inválido, cambio de mes y zona IANA.
- [ ] `E041-CT07` If-Match omitido -> 428; ETag viejo -> 412; carrera de versiones no pierde actualizaciones.

## Oleada 2 (2026-11-19) — E041 `PATCH /api/v1/goals/{goalId}`

| Campo | Valor |
|---|---|
| Estado | IMPLEMENTED · PR_OPEN · CI_GREEN (12/12) · MERGED=YES · AWS_DEPLOYED=NO · HTTPS_VERIFIED=NO · FUNCTIONAL_VERIFIED=NO |
| PR | #115 · HEAD `9bf19c942478dbaa468591b0862ac782e9882a2f` |
| Claves DynamoDB | USER#<id>/GOAL#<id> |
| Decisiones | If-Match 428/412; CAS de version; targetAmount no puede quedar por debajo de savedAmount; transiciones de estado validas. |
| Errores y causa raiz | U1000 por helper de prueba, corregido en 51bc79b. |
| Pruebas | `go build`, `go vet` y suites de aplicacion/adaptador/transporte en verde; donde Windows Application Control bloqueo el binario se declara NOT_RUN_LOCAL y la autoridad es el CI Linux. |
| Contrato Android | Ruta y esquema segun `sdd/03-endpoints/openapi/nexum-api-v1.json`; errores con `ErrorEnvelope` y `traceId`. |
| Pendiente | Merge autorizado, release agrupada y verificacion HTTP real. |
