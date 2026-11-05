# Tareas planificadas — E069

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E069-T01` Revisar OpenAPI `operationId=nexum_e069` vs `PATCH /api/v1/users/me/preferences` y contrato de dominio.
- [ ] `E069-T02` Implementar request/response DTO estrictos `UserPreferencesPatch` / `UserPreferences`; rechazo de campos desconocidos.
- [ ] `E069-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E069-T04` Implementar caso de uso y invariantes: Ruta opcional; versión y valores IANA/idioma verificados.
- [ ] `E069-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / PREFERENCES con control de version y unicidad.
- [ ] `E069-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E069-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E069-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E069-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E069-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E069-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E069-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E069-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E069-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E069-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E069-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E069-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E069-CT06` If-Match omitido -> 428; ETag viejo -> 412; carrera de versiones no pierde actualizaciones.
