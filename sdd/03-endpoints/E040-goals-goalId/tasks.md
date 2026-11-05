# Tareas planificadas — E040

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E040-T01` Revisar OpenAPI `operationId=nexum_e040` vs `GET /api/v1/goals/{goalId}` y contrato de dominio.
- [ ] `E040-T02` Implementar request/response DTO estrictos `ninguno` / `Goal`; rechazo de campos desconocidos.
- [ ] `E040-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E040-T04` Implementar caso de uso y invariantes: No revelar ID ajeno; mostrar suma guardada consistente.
- [ ] `E040-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / GOAL#<id>; CAS sobre version, historial de contribuciones separado.
- [ ] `E040-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E040-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E040-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E040-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E040-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E040-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E040-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E040-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E040-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E040-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E040-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E040-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E040-CT06` Montos exactos en centavos: 0, mínimo, máximo, decimal inválido, cambio de mes y zona IANA.
