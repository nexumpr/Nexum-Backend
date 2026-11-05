# Tareas planificadas — E075

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E075-T01` Revisar OpenAPI `operationId=nexum_e075` vs `POST /api/v1/voice/parse` y contrato de dominio.
- [ ] `E075-T02` Implementar request/response DTO estrictos `VoiceParseRequest` / `VoiceParse`; rechazo de campos desconocidos.
- [ ] `E075-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E075-T04` Implementar caso de uso y invariantes: Opcional: reconocimiento de voz es local; endpoint interpreta texto, no audio; nunca ejecutar operación financiera sin confirmación explícita.
- [ ] `E075-T05` Implementar adaptador DynamoDB y patrón: No se persiste audio ni texto de voz por defecto; inferencia aislada y confirmación explícita para efectos financieros.
- [ ] `E075-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E075-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E075-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E075-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E075-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E075-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E075-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E075-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E075-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E075-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E075-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E075-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
