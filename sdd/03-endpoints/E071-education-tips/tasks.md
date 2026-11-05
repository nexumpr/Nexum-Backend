# Tareas planificadas — E071

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E071-T01` Revisar OpenAPI `operationId=nexum_e071` vs `GET /api/v1/education/tips` y contrato de dominio.
- [ ] `E071-T02` Implementar request/response DTO estrictos `ninguno` / `PageOfTip`; rechazo de campos desconocidos.
- [ ] `E071-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E071-T04` Implementar caso de uso y invariantes: Información informativa, no recomendar productos por sesgo ni personalizar sin consentimiento.
- [ ] `E071-T05` Implementar adaptador DynamoDB y patrón: PUBLIC#CONTENT / FAQ#id, TIP#id versionados; Redis caché público.
- [ ] `E071-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E071-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E071-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E071-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E071-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E071-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E071-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E071-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E071-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E071-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E071-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E071-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
