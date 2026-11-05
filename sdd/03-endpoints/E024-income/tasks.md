# Tareas planificadas — E024

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E024-T01` Revisar OpenAPI `operationId=nexum_e024` vs `GET /api/v1/income` y contrato de dominio.
- [ ] `E024-T02` Implementar request/response DTO estrictos `ninguno` / `Income`; rechazo de campos desconocidos.
- [ ] `E024-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E024-T04` Implementar caso de uso y invariantes: Ingreso base distinto de movimientos realizados; no duplicar en sumarios.
- [ ] `E024-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / INCOME, MOVEMENT#<UTC>#<id>; periodo y proyección financiera versionada.
- [ ] `E024-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E024-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E024-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E024-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E024-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E024-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E024-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E024-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E024-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E024-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E024-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E024-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E024-CT06` Montos exactos en centavos: 0, mínimo, máximo, decimal inválido, cambio de mes y zona IANA.
