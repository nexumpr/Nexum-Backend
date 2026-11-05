# Tareas planificadas — E054

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E054-T01` Revisar OpenAPI `operationId=nexum_e054` vs `GET /api/v1/analytics/spending-patterns` y contrato de dominio.
- [ ] `E054-T02` Implementar request/response DTO estrictos `ninguno` / `PageOfSpendingPattern`; rechazo de campos desconocidos.
- [ ] `E054-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E054-T04` Implementar caso de uso y invariantes: Resultados sustentados por historial suficiente; no afirmar certeza estadística sin tamaño de muestra.
- [ ] `E054-T05` Implementar adaptador DynamoDB y patrón: Query por usuario y periodo + proyecciones versionadas / Redis privado; sin Scan; ML/IA según contrato de proveedor.
- [ ] `E054-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E054-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E054-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E054-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E054-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E054-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E054-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E054-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E054-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E054-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E054-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E054-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E054-CT06` Montos exactos en centavos: 0, mínimo, máximo, decimal inválido, cambio de mes y zona IANA.
