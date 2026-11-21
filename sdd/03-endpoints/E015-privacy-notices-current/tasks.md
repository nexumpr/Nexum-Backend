# Tareas planificadas — E015

> Todas las tareas de implementación están **TODO** hasta H08. Este archivo es especificación, no evidencia de código.

- [ ] `E015-T01` Revisar OpenAPI `operationId=nexum_e015` vs `GET /api/v1/privacy/notices/current` y contrato de dominio.
- [ ] `E015-T02` Implementar request/response DTO estrictos `ninguno` / `PrivacyNotice`; rechazo de campos desconocidos.
- [ ] `E015-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
- [ ] `E015-T04` Implementar caso de uso y invariantes: Versión pública firmada y fecha de vigencia; no datos de usuario.
- [ ] `E015-T05` Implementar adaptador DynamoDB y patrón: USER#<sub> / PROFILE, CONSENT#*, ONBOARDING, PREFERENCES; privacidad y revocación transversal.
- [ ] `E015-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
- [ ] `E015-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
- [ ] `E015-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
- [ ] `E015-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
- [ ] `E015-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
- [ ] `E015-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
- [ ] `E015-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.

## Casos concretos

- [ ] `E015-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI; ejemplos sintéticos deben validarse con JSON Schema.
- [ ] `E015-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
- [ ] `E015-CT03` Petición sobre recurso de otro usuario no filtra existencia (404), si aplica; verificar propiedad con dos sujetos.
- [ ] `E015-CT04` Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
- [ ] `E015-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.

## Tareas de la enmienda (A8) — PENDIENTES DE EJECUCION

- [ ] Implementar `content` (string) y `requiresAcceptance` (boolean) en la respuesta.
- [ ] Anadir la validacion de entrada correspondiente y su mensaje 422.
- [ ] Cubrir con prueba positiva y negativa.
- [ ] Verificar por HTTP real tras el despliegue autorizado.

<!-- NOTA_VIGENCIA_AUDITORIA_SEMANTICA -->
### Nota de vigencia - auditoria semantica (2026-11-21)

- Estado: `NO_CONFORME_SEMANTICA`.
- Hallazgo: NO CONFORME SEMANTICA: el aviso servido (2026-11-v2) declara no disponibles funciones ya desplegadas; la v3 sigue sin Gate legal y tambien omite funciones desplegadas. Requiere una version nueva aprobada; no se modifica contenido en este PR.
- Los bloques anteriores de este archivo conservan el estado historico del endpoint y no se reescriben.
- Evidencia: `memoria.md:1956-1958` (release v0.16.0) y `memoria.md:1968` (29/29 post-hotfix); Gate `GATE_H08E_OLEADA_1.md`.
- Releases: `v0.16.0+7ad1f4a9d6a2` y hotfix `v0.16.1+615ac1c734f2`. El codigo de estos 15 endpoints no cambio desde `7ad1f4a9`.
- Cualquier contradiccion entre bloques anteriores y esta nota se resuelve a favor de esta nota.
