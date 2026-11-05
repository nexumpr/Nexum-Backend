# Tareas planificadas — E055

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E055-T01` Revisar OpenAPI `operationId=nexum_e055` vs `GET /api/v1/analytics/recommendations` y contrato de dominio.
- [ ] `E055-T02` Implementar request/response DTO estrictos `ninguno` / `PageOfRecommendation`; rechazo de campos desconocidos.
- [ ] `E055-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E055-T04` Implementar caso de uso y invariantes: Sugerencias no equivalen a asesoría financiera profesional; explicación de fuente, límites y sin inventar ahorro garantizado.
- [ ] `E055-T05` Implementar adaptador DynamoDB y patrón: Query por usuario y periodo + proyecciones versionadas / Redis privado; sin Scan; ML/IA según contrato de proveedor.
- [ ] `E055-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E055-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E055-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E055-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E055-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E055-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E055-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E055-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E055-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E055-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E055-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E055-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E055-CT06` Montos exactos en centavos: 0, mínimo, máximo, decimal inválido, cambio de mes y zona IANA.
- [ ] `E055-CT07` Salida ML/IA real solo si proveedor/costo/autorización verificados; prohibido presentar respuesta fija como IA generativa.
