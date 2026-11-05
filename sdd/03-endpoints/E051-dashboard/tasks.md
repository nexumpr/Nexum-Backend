# Tareas planificadas — E051

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E051-T01` Revisar OpenAPI `operationId=nexum_e051` vs `GET /api/v1/dashboard` y contrato de dominio.
- [ ] `E051-T02` Implementar request/response DTO estrictos `ninguno` / `Dashboard`; rechazo de campos desconocidos.
- [ ] `E051-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E051-T04` Implementar caso de uso y invariantes: Agregados consistentes y versionados, cache privada Redis invalidada ante mutación financiera.
- [ ] `E051-T05` Implementar adaptador DynamoDB y patrón: Query por usuario y periodo + proyecciones versionadas / Redis privado; sin Scan; ML/IA según contrato de proveedor.
- [ ] `E051-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E051-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E051-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E051-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E051-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E051-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E051-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E051-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E051-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E051-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E051-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E051-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E051-CT06` Montos exactos en centavos: 0, mínimo, máximo, decimal inválido, cambio de mes y zona IANA.
