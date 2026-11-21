# Tareas planificadas — E023

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E023-T01` Revisar OpenAPI `operationId=nexum_e023` vs `GET /api/v1/catalogs/frequencies` y contrato de dominio.
- [ ] `E023-T02` Implementar request/response DTO estrictos `ninguno` / `PageOfFrequency`; rechazo de campos desconocidos.
- [ ] `E023-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E023-T04` Implementar caso de uso y invariantes: Conjunto controlado, reglas de fecha y periodicidad coherentes con operaciones recurrentes.
- [ ] `E023-T05` Implementar adaptador DynamoDB y patrón: PUBLIC#CATALOG / CATEGORY#id o FREQUENCY#id; versión publicada y Redis TTL con invalidación.
- [ ] `E023-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E023-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E023-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E023-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E023-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E023-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E023-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E023-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E023-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E023-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E023-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E023-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.

<!-- NOTA_VIGENCIA_AUDITORIA_SEMANTICA -->
### Nota de vigencia - auditoria semantica (2026-11-21)

- Estado: `AUDITADA_SEMANTICA_CON_OBSERVACIONES`.
- Hallazgo: `spec.md`/`plan.md` dicen DynamoDB; la implementacion es estatica. El enum de 9 frente a 7 canonicas es intencional (RFC-020).
- Los bloques anteriores de este archivo conservan el estado historico del endpoint y no se reescriben.
- Evidencia: `memoria.md:1956-1958` (release v0.16.0) y `memoria.md:1968` (29/29 post-hotfix); Gate `GATE_H08E_OLEADA_1.md`.
- Releases: `v0.16.0+7ad1f4a9d6a2` y hotfix `v0.16.1+615ac1c734f2`. El codigo de estos 15 endpoints no cambio desde `7ad1f4a9`.
- Cualquier contradiccion entre bloques anteriores y esta nota se resuelve a favor de esta nota.
