# A-040 — Checklist de arquitectura

**Estado:** DRAFT; pruebas documentales del lote comprobables; pruebas reales de Backend/infra `NOT_RUN`.

## Integridad conceptual

- [ ] `constitution.md` prevalece sobre decisiones de esta spec.
- [ ] No se asume que el prototipo Android realiza HTTP ni que el catálogo anterior describe implementación existente.
- [ ] La arquitectura AWS+DynamoDB+Redis reemplaza exclusivamente las decisiones de plataforma históricas; conserva funcionalidad identificada.
- [ ] Solo Backend; ninguna dependencia de aprobación, integración o pantalla del equipo Mobile.
- [ ] No genera specs de H05/H06 ni código ni operaciones cloud.

## Aceptación específica

- [ ] **AC-A-040-01:** Matrix 75/75 sin IDs duplicados.
- [ ] **AC-A-040-02:** Errores 401/403/404 no revelan recursos ajenos.
- [ ] **AC-A-040-03:** Headers idempotencia, versionado y paginación documentados.

## Revisiones de construcción

- [ ] Se revisó **Contrato formal** (§2.1) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [ ] Se revisó **Campos y convenciones** (§2.2) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [ ] Se revisó **Paginación** (§2.3) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [ ] Se revisó **Errores** (§2.4) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [ ] Se revisó **HTTP methods** (§2.5) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [ ] Se revisó **Seguridad** (§2.6) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [ ] Se revisó **Compatibilidad** (§2.7) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [ ] Se revisó **Contract-tests** (§2.8) y su mecanismo de aceptación con casos positivos, negativos y límites.

## Estado de las evidencias

| Evidencia | Estado actual | Regla |
|---|---|---|
| Documentos `spec/plan/tasks/checklist` en paquete | GENERADOS; validación en test de ZIP | Hash/estructura comprobables |
| Revisión humana del contenido de A-040 | PENDIENTE | No declarar APPROVED |
| Pruebas Go / HTTP reales | NOT_RUN | Solo tras implementación H08 |
| AWS/Redis/2 instancias/despliegue real | NOT_RUN | Se concretan H05, se ejecutan H08 |
| Costo cero y permisos de cuenta AWS | PENDIENTE H05 | Bloquea H05 si no viable |
