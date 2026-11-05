# A-010 — Checklist de arquitectura

**Estado:** DRAFT; pruebas documentales del lote comprobables; pruebas reales de Backend/infra `NOT_RUN`.

## Integridad conceptual

- [ ] `constitution.md` prevalece sobre decisiones de esta spec.
- [ ] No se asume que el prototipo Android realiza HTTP ni que el catálogo anterior describe implementación existente.
- [ ] La arquitectura AWS+DynamoDB+Redis reemplaza exclusivamente las decisiones de plataforma históricas; conserva funcionalidad identificada.
- [ ] Solo Backend; ninguna dependencia de aprobación, integración o pantalla del equipo Mobile.
- [ ] No genera specs de H05/H06 ni código ni operaciones cloud.

## Aceptación específica

- [ ] **AC-A-010-01:** go version objetivo y matriz Windows/Linux 64 bits definida.
- [ ] **AC-A-010-02:** Toda dependencia de infraestructura detrás de puertos y sometida a SCA.
- [ ] **AC-A-010-03:** Build de implementación debe funcionar sin Docker local.

## Revisiones de construcción

- [ ] Se revisó **Runtime Go** (§2.1) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [ ] Se revisó **HTTP y JSON** (§2.2) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [ ] Se revisó **DynamoDB** (§2.3) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [ ] Se revisó **Redis** (§2.4) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [ ] Se revisó **Autenticación** (§2.5) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [ ] Se revisó **Telemetría** (§2.6) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [ ] Se revisó **Tests/seguridad** (§2.7) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [ ] Se revisó **Construcción sin contenedores locales** (§2.8) y su mecanismo de aceptación con casos positivos, negativos y límites.

## Estado de las evidencias

| Evidencia | Estado actual | Regla |
|---|---|---|
| Documentos `spec/plan/tasks/checklist` en paquete | GENERADOS; validación en test de ZIP | Hash/estructura comprobables |
| Revisión humana del contenido de A-010 | PENDIENTE | No declarar APPROVED |
| Pruebas Go / HTTP reales | NOT_RUN | Solo tras implementación H08 |
| AWS/Redis/2 instancias/despliegue real | NOT_RUN | Se concretan H05, se ejecutan H08 |
| Costo cero y permisos de cuenta AWS | PENDIENTE H05 | Bloquea H05 si no viable |
