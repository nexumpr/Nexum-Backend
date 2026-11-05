# Tareas planificadas — E072

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E072-T01` Revisar OpenAPI `operationId=nexum_e072` vs `GET /api/v1/health/live` y contrato de dominio.
- [ ] `E072-T02` Implementar request/response DTO estrictos `ninguno` / `Health`; rechazo de campos desconocidos.
- [ ] `E072-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E072-T04` Implementar caso de uso y invariantes: Solo estado del proceso; nunca hacer depender liveness de Dynamo/Redis.
- [ ] `E072-T05` Implementar adaptador DynamoDB y patrón: Readiness consulta dependencias sin exponer tablas; liveness solo proceso (sin escritura).
- [ ] `E072-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E072-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E072-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E072-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E072-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E072-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E072-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E072-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E072-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E072-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E072-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E072-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E072-CT06` Nodo individual: /live no depende de DB, /ready devuelve 503 al fallar dependencia; el ALB saca solo el nodo no listo.
