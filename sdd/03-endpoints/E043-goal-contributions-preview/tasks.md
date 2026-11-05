# Tareas planificadas — E043

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E043-T01` Revisar OpenAPI `operationId=nexum_e043` vs `POST /api/v1/goal-contributions/preview` y contrato de dominio.
- [ ] `E043-T02` Implementar request/response DTO estrictos `ContributionPreviewRequest` / `ContributionPreview`; rechazo de campos desconocidos.
- [ ] `E043-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E043-T04` Implementar caso de uso y invariantes: Pura validación/calculo sin escrituras, mismas reglas que POST; centavos exactos, sin redondeos que pierdan unidades.
- [ ] `E043-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / CONTRIBUTION#<id>, GOAL#<id>, MOVEMENT#<UTC>#<id>; localizador y transacción con IDEM#.
- [ ] `E043-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E043-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E043-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E043-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E043-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E043-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E043-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E043-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E043-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E043-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E043-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E043-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E043-CT06` Montos exactos en centavos: 0, mínimo, máximo, decimal inválido, cambio de mes y zona IANA.
- [ ] `E043-CT07` Suma de asignaciones exacta; objetivo ajeno, metas repetidas, >12 metas, doble reversa y transacción cancelada sin efectos.
