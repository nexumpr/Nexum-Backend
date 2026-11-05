# Tareas planificadas — E038

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E038-T01` Revisar OpenAPI `operationId=nexum_e038` vs `GET /api/v1/goals` y contrato de dominio.
- [ ] `E038-T02` Implementar request/response DTO estrictos `ninguno` / `PageOfGoal`; rechazo de campos desconocidos.
- [ ] `E038-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E038-T04` Implementar caso de uso y invariantes: Consultar metas por usuario, estado y cursor; progreso desde contribuciones confirmadas.
- [ ] `E038-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / GOAL#<id>; CAS sobre version, historial de contribuciones separado.
- [ ] `E038-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E038-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E038-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E038-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E038-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E038-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E038-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E038-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E038-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E038-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E038-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E038-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E038-CT06` Montos exactos en centavos: 0, mínimo, máximo, decimal inválido, cambio de mes y zona IANA.
- [ ] `E038-CT07` Paginación 25/100, filtro modificado con cursor antiguo -> 400, lectura sin Scan, sin duplicados en páginas.
