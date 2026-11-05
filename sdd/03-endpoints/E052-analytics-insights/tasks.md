# Tareas planificadas — E052

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E052-T01` Revisar OpenAPI `operationId=nexum_e052` vs `GET /api/v1/analytics/insights` y contrato de dominio.
- [ ] `E052-T02` Implementar request/response DTO estrictos `ninguno` / `PageOfInsight`; rechazo de campos desconocidos.
- [ ] `E052-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E052-T04` Implementar caso de uso y invariantes: Insights sustentados por métricas verificables; componente ML real y explicaciones generativas solo cuando se hayan validado costo, privacidad y modelo.
- [ ] `E052-T05` Implementar adaptador DynamoDB y patrón: Query por usuario y periodo + proyecciones versionadas / Redis privado; sin Scan; ML/IA según contrato de proveedor.
- [ ] `E052-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E052-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E052-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E052-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E052-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E052-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E052-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E052-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E052-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E052-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E052-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E052-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E052-CT06` Montos exactos en centavos: 0, mínimo, máximo, decimal inválido, cambio de mes y zona IANA.
- [ ] `E052-CT07` Salida ML/IA real solo si proveedor/costo/autorización verificados; prohibido presentar respuesta fija como IA generativa.
