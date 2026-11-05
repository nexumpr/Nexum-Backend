# Tareas planificadas — E073

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E073-T01` Revisar OpenAPI `operationId=nexum_e073` vs `GET /api/v1/health/ready` y contrato de dominio.
- [ ] `E073-T02` Implementar request/response DTO estrictos `ninguno` / `Health`; rechazo de campos desconocidos.
- [ ] `E073-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E073-T04` Implementar caso de uso y invariantes: 503 cuando incapaz de servir peticiones; comprobar dependencias según H05 sin filtrar credenciales.
- [ ] `E073-T05` Implementar adaptador DynamoDB y patrón: Readiness consulta dependencias sin exponer tablas; liveness solo proceso (sin escritura).
- [ ] `E073-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E073-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E073-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E073-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E073-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E073-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E073-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E073-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E073-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E073-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E073-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E073-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E073-CT06` Nodo individual: /live no depende de DB, /ready devuelve 503 al fallar dependencia; el ALB saca solo el nodo no listo.
