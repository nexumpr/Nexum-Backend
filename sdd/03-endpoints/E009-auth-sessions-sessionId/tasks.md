# Tareas planificadas — E009

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E009-T01` Revisar OpenAPI `operationId=nexum_e009` vs `DELETE /api/v1/auth/sessions/{sessionId}` y contrato de dominio.
- [ ] `E009-T02` Implementar request/response DTO estrictos `ninguno` / `ninguno`; rechazo de campos desconocidos.
- [ ] `E009-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E009-T04` Implementar caso de uso y invariantes: ID ajeno -> 404 homogéneo; revocación persistente y propagación multiinstancia.
- [ ] `E009-T05` Implementar adaptador DynamoDB y patrón: Email-reserva / USER#id PROFILE y SESSION#id / REFRESH#<digest>/SESSION; token hash con TTL y expiración funcional.
- [ ] `E009-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E009-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E009-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E009-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E009-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E009-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E009-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E009-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E009-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E009-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E009-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E009-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
- [ ] `E009-CT06` Revocación/rotación en nodo A persiste y es observable inmediatamente en nodo B; token reutilizado rechazado.

## Tareas de la enmienda (A1) — PENDIENTES DE EJECUCION

- [ ] Implementar referencias de persistencia corregidas a `USER#<userId>/SESSION#<sessionId>` y `REFRESH#<sha256(refreshToken)>/SESSION`; sin cambio de tabla ni de codigo.
- [ ] Anadir la validacion de entrada correspondiente y su mensaje 422.
- [ ] Cubrir con prueba positiva y negativa.
- [ ] Verificar por HTTP real tras el despliegue autorizado.

<!-- NOTA_VIGENCIA_AUDITORIA_SEMANTICA -->
### Nota de vigencia - auditoria semantica (2026-11-21)

- Estado: `AUDITADA_SEMANTICA_CON_OBSERVACIONES`.
- Hallazgo: 429 declarado y no emitido; 409/422 individuales en AWS no localizados.
- Los bloques anteriores de este archivo conservan el estado historico del endpoint y no se reescriben.
- Evidencia: `sdd/_control/H08/GATE_H08E_E007_E011_DESPLIEGUE_AWS.md` §4 y los Gates especificos de E001-E006; `memoria.md` cierre E001/E002 y v0.15.0.
- Release inicial: ver la matriz de auditoria del PR #142. Ultima verificacion funcional agregada: `v0.15.0+8f13c86d2ca0`.
- Cualquier contradiccion entre bloques anteriores y esta nota se resuelve a favor de esta nota.
