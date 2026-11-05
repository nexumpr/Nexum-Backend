# A-030 — Checklist de arquitectura

**Estado:** DRAFT; pruebas documentales del lote comprobables; pruebas reales de Backend/infra `NOT_RUN`.

## Integridad conceptual

- [ ] `constitution.md` prevalece sobre decisiones de esta spec.
- [ ] No se asume que el prototipo Android realiza HTTP ni que el catálogo anterior describe implementación existente.
- [ ] La arquitectura AWS+DynamoDB+Redis reemplaza exclusivamente las decisiones de plataforma históricas; conserva funcionalidad identificada.
- [ ] Solo Backend; ninguna dependencia de aprobación, integración o pantalla del equipo Mobile.
- [ ] No genera specs de H05/H06 ni código ni operaciones cloud.

## Aceptación específica

- [ ] **AC-A-030-01:** Timeout, 413, 415, 405, HEAD/OPTIONS y 429 reproducibles.
- [ ] **AC-A-030-02:** No hay secreto ni PII en logs HTTP ni respuestas de error.
- [ ] **AC-A-030-03:** Shutdown sin solicitudes perdidas indebidas.

## Revisiones de construcción

- [ ] Se revisó **Routing** (§2.1) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [ ] Se revisó **Tiempo y recursos** (§2.2) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [ ] Se revisó **Cadena middleware** (§2.3) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [ ] Se revisó **Seguridad HTTP** (§2.4) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [ ] Se revisó **CORS** (§2.5) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [ ] Se revisó **Protección de entradas** (§2.6) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [ ] Se revisó **Graceful shutdown** (§2.7) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [ ] Se revisó **Compresión** (§2.8) y su mecanismo de aceptación con casos positivos, negativos y límites.

## Estado de las evidencias

| Evidencia | Estado actual | Regla |
|---|---|---|
| Documentos `spec/plan/tasks/checklist` en paquete | GENERADOS; validación en test de ZIP | Hash/estructura comprobables |
| Revisión humana del contenido de A-030 | PENDIENTE | No declarar APPROVED |
| Pruebas Go / HTTP reales | NOT_RUN | Solo tras implementación H08 |
| AWS/Redis/2 instancias/despliegue real | NOT_RUN | Se concretan H05, se ejecutan H08 |
| Costo cero y permisos de cuenta AWS | PENDIENTE H05 | Bloquea H05 si no viable |
