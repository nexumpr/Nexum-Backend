# A-060 — Checklist de arquitectura

**Estado:** DRAFT; pruebas documentales del lote comprobables; pruebas reales de Backend/infra `NOT_RUN`.

## Integridad conceptual

- [ ] `constitution.md` prevalece sobre decisiones de esta spec.
- [ ] No se asume que el prototipo Android realiza HTTP ni que el catálogo anterior describe implementación existente.
- [ ] La arquitectura AWS+DynamoDB+Redis reemplaza exclusivamente las decisiones de plataforma históricas; conserva funcionalidad identificada.
- [ ] Solo Backend; ninguna dependencia de aprobación, integración o pantalla del equipo Mobile.
- [ ] No genera specs de H05/H06 ni código ni operaciones cloud.

## Aceptación específica

- [ ] **AC-A-060-01:** Nunca Scan; solo Query o GetItem/BatchGetItem con índices justificados.
- [ ] **AC-A-060-02:** Transacción con falla de condición no deja goal.saved sin movement.
- [ ] **AC-A-060-03:** Diseño de índices cubre los 75 IDs incluidas rutas O como opciones no obligatorias.

## Revisiones de construcción

- [ ] Se revisó **Tablas y modo** (§2.1) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [ ] Se revisó **Identidad global** (§2.2) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [ ] Se revisó **Partición usuario** (§2.3) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [ ] Se revisó **Índices y acceso** (§2.4) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [ ] Se revisó **Movimientos e integridad** (§2.5) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [ ] Se revisó **Aportación compuesta** (§2.6) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [ ] Se revisó **Límites y concurrencia** (§2.7) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [ ] Se revisó **TTL y eliminación** (§2.8) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [ ] Se revisó **Patrones de acceso obligatorios** (§2.9) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [ ] Se revisó **Validación de diseño** (§2.10) y su mecanismo de aceptación con casos positivos, negativos y límites.

## Estado de las evidencias

| Evidencia | Estado actual | Regla |
|---|---|---|
| Documentos `spec/plan/tasks/checklist` en paquete | GENERADOS; validación en test de ZIP | Hash/estructura comprobables |
| Revisión humana del contenido de A-060 | PENDIENTE | No declarar APPROVED |
| Pruebas Go / HTTP reales | NOT_RUN | Solo tras implementación H08 |
| AWS/Redis/2 instancias/despliegue real | NOT_RUN | Se concretan H05, se ejecutan H08 |
| Costo cero y permisos de cuenta AWS | PENDIENTE H05 | Bloquea H05 si no viable |
