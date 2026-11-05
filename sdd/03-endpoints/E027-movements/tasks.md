# Tareas planificadas — E027

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E027-T01` Revisar OpenAPI `operationId=nexum_e027` vs `GET /api/v1/movements` y contrato de dominio.
- [ ] `E027-T02` Implementar request/response DTO estrictos `ninguno` / `PageOfMovement`; rechazo de campos desconocidos.
- [ ] `E027-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E027-T04` Implementar caso de uso y invariantes: Query por usuario, fecha, tipo y cursor firmado; orden determinista timestamp+ID; nunca Scan.
- [ ] `E027-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / MOVEMENT#<UTC>#<id>; MOVEMENT_ID#<id> locator; GSI1 para filtros por tipo/fecha.
- [ ] `E027-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E027-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E027-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E027-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E027-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E027-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E027-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E027-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E027-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E027-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E027-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E027-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E027-CT06` Montos exactos en centavos: 0, mínimo, máximo, decimal inválido, cambio de mes y zona IANA.
- [ ] `E027-CT07` Paginación 25/100, filtro modificado con cursor antiguo -> 400, lectura sin Scan, sin duplicados en páginas.
